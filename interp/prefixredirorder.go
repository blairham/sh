// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// When a command's assignment prefix is expanded, against when its
// redirections are opened.
//
// Measured 2026-09-18 from a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C` with a scratch HOME, with `f() { :; }`:
//
//	w=$(echo S >&2) f > /nope/x
//
//	bash 5.3.20          S, then the file complaint, 1
//	ksh93u+ 2012-08-01   S, then the file complaint, 1
//	zsh 5.9.2            the file complaint alone, 1
//	dash 0.5.12          the file complaint alone, 2
//	BusyBox ash 1.37.0   the file complaint alone, 1
//
// So two columns work through the prefix before they open anything, and the
// side effect of a substitution in a value is what makes the difference
// visible: where the redirection fails, the substitution runs in the first two
// and never runs in the other three.
//
// **And the walk is in written order**, which the same two columns show with a
// word one of them refuses:
//
//	w=$(echo S1 >&2) a[1]=v f
//
// bash writes `S1` and *then* `` `a[1]': not a valid identifier ``. This
// engine reported every refused subscripted word in a pass of its own, ahead
// of every value — the same set of lines in a different order. So the refusals
// and the values are one walk here rather than two passes, and the order is
// the order the script wrote them in.
//
// The refused word's **own** value is still never expanded, which is the
// measurement that keeps the walk from being "expand everything then refuse":
// `a[1]=$(echo side) f >/nope/x` writes the identifier complaint, then the
// file's, and never runs the substitution.

// prefixExpandedBeforeTheRedirections asks the axis, and only where there is a
// prefix for it to be about.
//
// The second result is whether the dialect answered. A command with no prefix
// asks nothing, which is most commands and every command in a dialect that has
// no answer to give.
func (r *Runner) prefixExpandedBeforeTheRedirections(assigns []*syntax.Assign, argv []string) (bool, bool) {
	if !aPrefixIsWritten(assigns) {
		return false, true
	}
	switch r.sem().PrefixExpandedBeforeTheRedirections {
	case PrefixExpandedBeforeRedirectionsAlways:
		return true, true
	case PrefixExpandedBeforeRedirectionsNever:
		return false, true
	case PrefixExpandedBeforeRedirectionsWhereItPersists:
		// The kind the prefix stands in front of, which is the same lookup
		// the persistence itself is decided by — so the two cannot come to
		// disagree about which commands they are talking about.
		kind := r.prefixCommandOf(argv)
		return kind.kind == prefixBeforeFunction ||
			kind.kind == prefixBeforeSpecialBuiltin, true
	}
	r.errf("%s\n", r.diag().Report(r.name(), r.line,
		r.unanswered("a command's assignment prefix against its redirections")))
	r.status, r.unspecified = 2, true
	return false, false
}

// PrefixRedirectionOrder is when a command's assignment prefix is worked
// through against when its redirections are opened — see the note at the top
// of this file for the panel.
type PrefixRedirectionOrder uint8

const (
	// PrefixExpandedBeforeRedirectionsUnspecified is no answer, and is
	// refused like any other.
	PrefixExpandedBeforeRedirectionsUnspecified PrefixRedirectionOrder = iota
	// PrefixExpandedBeforeRedirectionsAlways works through the prefix first
	// whatever the command is: bash, where `w=$(echo S >&2) /usr/bin/true >
	// /nope/x` writes `S` as readily as the same line in front of a function
	// does.
	PrefixExpandedBeforeRedirectionsAlways
	// PrefixExpandedBeforeRedirectionsWhereItPersists works through it first
	// only where the assignment is a real store — a function or a special
	// builtin — and opens the redirections first for a regular builtin and an
	// external: ksh93. Measured 2026-09-18, `w=$(echo S >&2) f > /nope/x` and
	// `w=$(echo E >&2) eval : > /nope/x` write their letter there and
	// `/usr/bin/true`, `true` and `print` write none, which is exactly the
	// set that shell keeps a prefix for.
	PrefixExpandedBeforeRedirectionsWhereItPersists
	// PrefixExpandedBeforeRedirectionsNever opens the redirections first, so
	// a substitution in a value never runs when one fails: zsh, dash and
	// BusyBox ash.
	PrefixExpandedBeforeRedirectionsNever
)

func (p PrefixRedirectionOrder) String() string {
	switch p {
	case PrefixExpandedBeforeRedirectionsAlways:
		return "the prefix is expanded before the redirections"
	case PrefixExpandedBeforeRedirectionsWhereItPersists:
		return "the prefix is expanded first where it persists"
	case PrefixExpandedBeforeRedirectionsNever:
		return "the redirections are opened first"
	}
	return "unspecified"
}

// aPrefixIsWritten reports whether any of these assignments is a prefix rather
// than an operand of a declaration builtin.
func aPrefixIsWritten(assigns []*syntax.Assign) bool {
	for _, a := range assigns {
		if !a.Operand {
			return true
		}
	}
	return false
}

// walkThePrefixBeforeTheRedirections is the single ordered pass the two
// columns above make: each entry is refused or expanded where it stands, and
// the values it leaves are the ones the store reads back.
//
// traceEach says the shell is writing a line per prefix entry ahead of the
// command, in which case that line is written here too — otherwise the
// refusal would come out ahead of every one of them, which is the same set of
// lines in the wrong order again. Measured on bash 5.3.20, `set -x; w=1
// a[1]=v q=1 f`: `+ w=1`, the identifier complaint, `+ q=1`, `+ f`.
//
// Reports whether it wrote those lines, so the caller writes the command alone.
func (r *Runner) walkThePrefixBeforeTheRedirections(assigns []*syntax.Assign, walk prefixWalk, traceEach bool) bool {
	// Whether a subscripted word in this prefix is refused at all, settled
	// once for the command the way the pass it replaces settled it. A `false`
	// here is a prefix with no subscripted word in it as often as it is a
	// dialect that stores the element, so it gates the refusal below and
	// never the walk — which is what row C of the panel above needs, since
	// `w=$(…) f > /nope/x` has nothing subscripted in it at all.
	refuses := r.subscriptedPrefixRefusalAnswered(assigns)
	if r.unspecified {
		return false
	}
	d := r.diag()
	// What this walk has expanded so far, visible to the entries behind it
	// and put back before the route below makes the stores for real. The
	// walk is the one place the two columns that take it expand *every*
	// value before any of them has been applied, which is what made
	// `K=v1 A=${K#v} f` hand the body nothing. See interp/prefixsees.go.
	var held heldPrefix
	defer held.release(r)
	wrote := false
	for _, a := range assigns {
		if r.prefixWalkFailed(walk) {
			// The walk stops at the first value that would not expand, and
			// the entries behind it are not expanded at all — unanimous in
			// the panel, and the caller gives the command up. See
			// interp/prefixexpansionfailed.go.
			break
		}
		if a.Operand {
			continue
		}
		if r.subscriptedPrefixDropped(a) {
			// The refusal, said where the word stands rather than in a pass
			// of its own, and the word's own value is not expanded.
			if refuses {
				r.refuseOneSubscriptedPrefix(a)
			}
			continue
		}
		if !r.prefixEntryHasATraceableValue(a) {
			continue
		}
		value := r.prefixExpansion(a)
		r.prefixTraceAssigns = append(r.prefixTraceAssigns, a)
		r.prefixTraceValues = append(r.prefixTraceValues, value)
		if name, ok := r.prefixHoldableName(a); ok {
			held.hold(r, name, r.prefixJoined(a, value))
		}
		if !traceEach {
			continue
		}
		for _, w := range r.prefixTraceWords([]*syntax.Assign{a}, *d) {
			r.awaitTraceTurn()
			r.traceLine(w, *d)
			r.releaseTraceTurn()
			wrote = true
		}
	}
	return wrote
}
