// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

	"github.com/blairham/sh/syntax"
)

// A prefix in front of a traced `function`-form call, which one shell works
// through twice: once ahead of the call as the shell's own, and once more as
// the call's own cells. See Semantics.PrefixToATracedKeywordFunctionIsWorkedTwice
// for the panel. Measured 2026-11-01 on ksh93u+ 2012-08-01 from script files
// under `env -i PATH=/usr/bin:/bin`, with `function g { echo "in x=$x"; };
// typeset -ft g`:
//
//	set -x; x=1 g                  `+ x=1`, `+ g`, `+ x=1`, `+ echo 'in x=1'`
//	set -x; x=1 g 2>/dev/null      `+ x=1`, `+ g`, `+ 2> /dev/null`
//	x=$(echo s >&2) g              `s`, `s`, then the body
//	x=$(echo s >&2) g 2>/dev/null  `s` once: the second is behind the redirection
//	x=1 g; echo $x                 `1`: the front pass persists
//	x=o; x+=a g; echo $x           the body sees `a`, then `oa`
//	export x=o; x=n g; typeset -p x   `x=n`: the front pass takes the export off
//	x=1 y=$(printenv x) g; echo $y the body sees `1`, then nothing
//
// So the front pass is exactly a POSIX-form call's prefix, and this engine
// already made it — written ahead of the command and expanded ahead of the
// redirections, because Runner.prefixIsTheCallsOwn says the prefix is not the
// call's own. What was missing is that it is then **stored** as one: on the
// shell's cells, with the persisting route's export attribute, and left there.
// The second pass is the untraced call's — expanded once the redirections are
// open, written behind the command's line, stored on fresh cells the return
// takes away — and it reads what the first one stored: `x=o; y=$x x=n g` shows
// the body `y=n` and leaves `y=o`.

// tracedCallWorksItsPrefixTwice asks the axis, for a call whose prefix is
// scoped to the call. Only where the function is traced, so a dialect that
// marks no function traced is never asked.
func (r *Runner) tracedCallWorksItsPrefixTwice(fn *syntax.FuncDecl, name string) bool {
	if !r.tracesFunction(name, fn.Keyword) {
		return false
	}
	return r.ask(r.sem().PrefixToATracedKeywordFunctionIsWorkedTwice,
		"an assignment before a traced `function`-form function worked through twice")
}

// tracedCallPersistsItsPrefixFirst is the same question read rather than
// asked, for a walk that only needs to know which route it is holding the
// prefix for. See Runner.heldExportRouteOf.
func (r *Runner) tracedCallPersistsItsPrefixFirst(name string) bool {
	return r.sem().PrefixToATracedKeywordFunctionIsWorkedTwice == Yes && r.tracesFunction(name, true)
}

// persistThePrefixAheadOfATracedCall makes the front pass's stores, as a
// POSIX-form call's, from the values the walk ahead of the redirections
// recorded, and then works the prefix through a second time: the records are
// forgotten, so the call's own stores expand every value again, and the walk
// behind the command's line writes them where the dialect traces one.
//
// It hands back what the front pass's take-back puts back, which in the one
// column with this shape is nothing, since a prefix to a function persists
// there — see Semantics.AssignmentPrefixPersistsAfterAFunction — and reports
// whether the command is over.
func (r *Runner) persistThePrefixAheadOfATracedCall(
	ctx context.Context,
	c *syntax.SimpleCmd,
	argv []string,
	walk prefixWalk,
	refused bool,
) ([]savedVar, bool) {
	persisted, _ := r.applyTheFunctionPrefix(ctx, c.Assigns, walk, refused, false)
	if r.ctl == controlExit || r.unspecified || r.prefixWalkFailed(walk) {
		return persisted, true
	}
	r.forgetThePrefixExpansions()
	if !r.tracesItsPrefix(c.Assigns, argv) || r.diag().TracePrefixAssignment != TracePrefixOwnLineAfter {
		// Untraced, the call's own stores expand the values themselves.
		return persisted, false
	}
	route := r.heldExportRouteOf(argv)
	route.scoped = true
	return persisted, r.walkThePrefixBehindTheCommand(c, argv, route)
}

// forgetThePrefixExpansions drops what this command's walks recorded of its
// prefix, so that the next walk or store expands every value again rather than
// reading back the one already made.
func (r *Runner) forgetThePrefixExpansions() {
	r.prefixTraceAssigns, r.prefixTraceValues, r.prefixTraceJoins = nil, nil, nil
	r.prefixTracedEarly = nil
	r.prefixGlobMatches = nil
	r.prefixSubscripts = nil
}
