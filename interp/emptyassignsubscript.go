// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// refusesEmptyAssignSubscript is `a[]=6` — a plain assignment whose brackets
// were written with nothing at all between them — and reports whether the
// assignment was refused here.
//
// Asked of the **node**, before the name is looked up and before the freeze
// is consulted, because that is what was measured: `readonly r; r[]=6` is the
// subscript complaint in bash 5.3.20 and zsh 5.9.2 alike and not `r: readonly
// variable`. And asked of what was *written*: a subscript that arrived empty
// from an expansion is a different construct with a different answer in every
// column — see Semantics.EmptySubscriptToAnAssignment, and
// syntax.Assign.EmptySubscript for the parser's half.
//
// The value is expanded first and thrown away, which is measured too:
// `a[]=$(echo SUBRAN >&2; echo v)` writes `SUBRAN` before the complaint in
// both columns, so the right-hand side runs and only the store is refused.
//
// Nothing is written — not the element the brackets would have named, and not
// the bare name either. That is the whole of the bug this answers: the
// subscript was dropped while the word was parsed, so the assignment arrived
// as `a=6` and stored element **zero** in one dialect and replaced the array
// with a scalar in another, at status 0 with nothing said (#3949).
func (r *Runner) refusesEmptyAssignSubscript(a *syntax.Assign) bool {
	if !a.EmptySubscript || a.Operand {
		// An operand is a declaration utility's own `typeset a[]=(x y)`,
		// which every column refuses as a bad **name** — sentence, status
		// and speaker all the declaration's — rather than with the sentence
		// below. A question one construct out, and not this one.
		return false
	}
	if a.Value != nil && !a.IsArray {
		// Expanded for its side effects and dropped. Behind the check above
		// rather than in front of it, so that the shapes this does not
		// answer keep the ordering they had.
		r.assignValue(a)
	}
	r.badSubscriptGivesUp(r.sem().EmptySubscriptToAnAssignment,
		"how much a plain assignment with an empty subscript gives up",
		Wording(r.diag().AssignEmptySubscript,
			"%[1]s[]: bad array subscript", a.Name))
	return true
}
