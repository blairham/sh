// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// The dialect that reads `${!name}` as a **subscript flag** rather than as an
// indirection or as the name.
//
// One shell reaches this and it does not reach it in its own mode: zsh's
// grammar has no `${!…}` at all, and `emulate ksh` gives it one that means
// zsh's own `(k)` — "the subscript", not "the thing the value names". See
// Semantics.IndirectionIsTheSubscriptFlag, where the twenty rows are.
//
// # A rewrite rather than a reading
//
// The flag is already implemented and already agrees with the reference, so
// nothing here answers the construct: the node is handed on as the `(k)`
// spelling of itself and the existing machinery answers it. That is what
// keeps the sigil and the flag from drifting — four switches keyed on the
// shapes the grid varies would have been four places for them to part, and
// the shapes are exactly where they would have parted, since `${!a[@]}` is
// the array's *values* and `${!m[@]}` its keys.
//
// # Where it is asked
//
// The same three seams Runner.unreadBareSubscript uses, and for the same
// reason its own file gives: every span that expands goes through expandSpan
// or expandAt, and every span that becomes a pattern goes through
// patternSpan. The two rewrites are made in one expression at each of those
// three sites so that a fourth seam cannot pick up one and miss the other.

// indirectionReadAsTheSubscriptFlag is the span with its `!` rewritten as a
// `(k)` flag, where the dialect reads it that way — and the span it was
// handed everywhere else.
//
// The node is copied rather than written through: the tree is the program,
// and a function body holding `${!v}` is parsed once and run under whatever
// answer is in force at the call.
func (r *Runner) indirectionReadAsTheSubscriptFlag(s syntax.Span) syntax.Span {
	e := s.Param
	if s.Kind != syntax.ParamExp || e == nil || !e.Indirect {
		return s
	}
	if e.Prefix != 0 {
		// `${!name@}` and `${!name*}` — the spelling that yields the *names*
		// beginning with `name`. The shell this axis is about has no such
		// form at all (syntax.Dialect.ParamIndirectionPrefixListing is off
		// there, so this never arrives from it), and the two that do are the
		// ones answering No. Declined here rather than left to the answer, so
		// the axis is asked where it decides and not on a road no dialect
		// disagrees about.
		return s
	}
	if !r.ask(r.sem().IndirectionIsTheSubscriptFlag,
		"`${!name}` being the subscript rather than an indirection") {
		return s
	}
	flagged := *e
	// Dropped, although it is **measured equivalent**: with the flag on the
	// node, every shape the reference's `emulate ksh` can be asked answers the
	// same either way, and that mode has no name references for an
	// indirection path to reach at all (`typeset -n` is `bad option: -n`
	// there). Written because a node carrying both readings claims to be two
	// things at once, and asserted where it operates rather than left to a
	// row that cannot see it — see
	// TestTheRewrittenNodeIsTheFlagAndNotAnIndirection.
	flagged.Indirect = false
	flagged.HasFlags = true
	// In front of whatever group was written, because the `!` is the outer
	// reading: `${(P)!v}` is not a spelling any measured shell has, and a
	// letter already there keeps its meaning under it rather than instead of
	// it.
	flagged.Flags = "k" + flagged.Flags
	s.Param = &flagged
	return s
}
