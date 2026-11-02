// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

	"github.com/blairham/sh/syntax"
)

// A prefix that persists — one in front of a special builtin, or a POSIX-form
// function in the shell that keeps that one too — is **stored** ahead of the
// command's redirections in one shell, and behind them in the other that
// expands it there.
//
// Measured 2026-11-02 from script files under `env -i PATH=/usr/bin:/bin` with
// a scratch HOME, with `f() { :; }` first; the `:` rows read the value from an
// EXIT trap, because a failed redirection on a special builtin ends the script
// where the POSIX rule is kept:
//
//	                                 ksh93u+  bash 5.3.20  bash --posix  zsh 5.9.2  dash
//	x=0; x=5 f >/nope/x; echo $x     5        0            0             0          0
//	x=0; x=5 : >/nope/x              5        0            0             0          0
//	x=0; x=5 f >out_$x               out_5    out_0        out_0         out_0      out_0
//	x=0; x=5 : >out_$x               out_5    out_0        out_0         out_0      out_0
//	x=0; x=5 /usr/bin/true >out_$x   out_0    out_0        out_0         out_0      out_0
//
// And the elements, in ksh93u+, where bash refuses a subscripted prefix and
// zsh keeps the old element: `typeset -A a; a[k]=old; a[k]=new f >/nope/x`
// leaves `new`, and `b=(x y z); i=1; b[i++]=1 f >/nope/x` leaves `2 x 1 z`.
//
// So ksh93 performs a persisting prefix as the plain assignment it is, before
// it opens anything: the value is in place when a redirection fails, and the
// redirection's own word already reads it. The same holds for the front pass
// of a traced `function`-form call (see interp/tracedcallprefix.go) — `x=1 g
// >/nope/x` leaves `1` — and through `command`, whose `:` keeps its prefix
// there. It is a real store and not a value held for the entries behind it:
// `export x=o; x=n : >/nope/x` takes the export off, `typeset -i v=1; v=2+2
// f >/nope/x` leaves a plain `v=2+2`, and `y=o; y+=a f >/nope/x` leaves `oa`.
//
// bash under `--posix` is the column that tells this apart from
// Semantics.PrefixExpandedBeforeTheRedirections: it keeps `:`'s prefix too,
// and expands it ahead of the redirections — `w=$(echo S >&2) : >/nope/x`
// writes `S` — and still stores it behind them, so the same lines leave `0`
// and write `out_0`. Expanding first and storing first are two answers.

// storedAheadPrefix is what the store made ahead of the redirections handed
// back, kept for the route behind them. made says there was one.
type storedAheadPrefix struct {
	made bool
	undo []savedVar
	held []string
}

// storeThePrefixAheadOfTheRedirections makes the store of a prefix that
// persists, where the dialect makes it before the command's redirections are
// opened, and reports whether the command is over.
//
// The store is the route's own — applyTheFunctionPrefix's persisting form for
// a function, applyTheBuiltinPrefix for a special builtin — so it is one store
// made earlier and not a second one: each of those hands back what it made
// here when the route reaches it. Asked behind the walk that expanded the
// values, which is the only place a dialect stores ahead of anything.
func (r *Runner) storeThePrefixAheadOfTheRedirections(
	ctx context.Context,
	c *syntax.SimpleCmd,
	argv []string,
	walk prefixWalk,
	local string,
) bool {
	kind := r.prefixCommandOf(argv)
	if !r.persistingPrefixStoredAhead(c.Assigns, argv, kind) {
		return r.unspecified
	}
	// Not the builtin's redirection being opened: a complaint the store makes
	// is located as an assignment's, as it was behind the redirections.
	forBuiltin, forWord := r.redirectForBuiltin, r.redirForCommandWord
	r.redirectForBuiltin, r.redirForCommandWord = "", ""
	// A frozen name is never stored: it has been refused already, ahead of
	// the values, where this dialect refuses a persisting prefix's — see
	// Semantics.PrefixToAFrozenNameIsCheckedFirst — or is left for the route
	// to refuse.
	var undo []savedVar
	var held []string
	if kind.kind == prefixBeforeFunction {
		undo, held = r.applyTheFunctionPrefix(ctx, c.Assigns, walk, true, false)
	} else {
		undo, held = r.applyTheBuiltinPrefix(ctx, c, argv, kind, walk, true, local)
	}
	r.redirectForBuiltin, r.redirForCommandWord = forBuiltin, forWord
	r.prefixStoredAhead = storedAheadPrefix{made: true, undo: undo, held: held}
	return r.ctl == controlExit || r.unspecified
}

// persistingPrefixStoredAhead asks Semantics.PersistingPrefixStoredBeforeTheRedirections,
// and only of a command whose prefix persists: a function's where the dialect
// keeps one and it is not the call's own cell, and a special builtin's where
// it keeps that.
func (r *Runner) persistingPrefixStoredAhead(assigns []*syntax.Assign, argv []string, kind prefixCommand) bool {
	switch kind.kind {
	case prefixBeforeFunction:
		// Read rather than asked, as applyTheFunctionPrefix reads it: the
		// take-back after the call is where that question is put.
		if r.sem().AssignmentPrefixPersistsAfterAFunction != Yes || r.prefixIsTheCallsOwn(assigns, argv) {
			return false
		}
	case prefixBeforeSpecialBuiltin:
		if !r.prefixPersistsAtThisBuiltin(argv[0], kind) {
			return false
		}
	default:
		return false
	}
	return r.ask(r.sem().PersistingPrefixStoredBeforeTheRedirections,
		"a persisting assignment prefix stored before the command's redirections")
}
