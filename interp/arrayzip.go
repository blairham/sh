// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// `${a:^b}` and `${a:^^b}`: interleave a parameter with the array the operand
// names, one element from each in turn.
//
// Measured on zsh 5.9.2, 2026-09-11, and every row below is a test:
//
//	a=(1 2)   b=(x y)       ${a:^b}    1 x 2 y
//	a=(1 2)   b=(x y z w)   ${a:^b}    1 x 2 y          — stops at the shorter
//	a=(1 2)   b=(x y z w)   ${a:^^b}   1 x 2 y 1 z 2 w  — cycles the shorter
//	a=(1)     b=(x y z)     ${a:^^b}   1 x 1 y 1 z
//	a=(1 2 3) b=()          ${a:^b}    (empty)
//	a=(1 2 3) b=()          ${a:^^b}   1 2 3
//
// # The operand is a name, not a pattern
//
// The same shape ParamSetDifference and ParamSetIntersection have, and the
// same resolution: the word is the *name* of an array and its elements are
// the operand. A name nothing is stored under contributes no elements — but
// which of the two answers that produces differs by operator, which is the
// one asymmetry here and is measured rather than derived:
//
//	a=(1 2); ${a:^nosuch}   1 2          — an unset right-hand name is no zip
//	b=(x y); "${a:^b}"      (empty)       — an unset left-hand one is empty
//
// # Quoting is not a special case
//
// Quoted, the left operand is already one word by the time this is reached,
// so the zip stops after a single pair and `"${a:^b}"` on `(1 2 3)` and
// `(x y z)` is the two fields `1 2 3` and `x`. That falls out of the rule
// rather than needing one of its own, which is why nothing here looks at
// quoting.
//
// # Why it is not an element selector
//
// selectsElements and its three operators *choose* which elements survive, so
// the count only ever falls. This one produces a list longer than either
// input, so it is its own path — folding it in would have made `keep` a
// function that has to return more than a bool.

// zipsElements reports whether the operator interleaves two arrays.
func zipsElements(op syntax.ParamOp) bool {
	return op == syntax.ParamZip || op == syntax.ParamZipCycle
}

// zipElements interleaves elems with the array the operand names.
func (r *Runner) zipElements(e *syntax.ParamExpr, elems []string) []string {
	name := r.joinWord(e.Arg)
	other, named := r.arrayElems(name)
	if !named {
		// A name nothing answers to is no second array at all, and the
		// parameter comes back as it was — measured, and the opposite of
		// what a *set* operator does with the same absence.
		return elems
	}
	if len(elems) == 0 || len(other) == 0 {
		if e.Op == syntax.ParamZipCycle {
			// Cycling an empty array contributes nothing, so what is left is
			// the other side whole: `(1 2 3):^^()` is `1 2 3`.
			if len(other) == 0 {
				return elems
			}
			return other
		}
		return nil
	}
	pairs := len(elems)
	if e.Op == syntax.ParamZip {
		pairs = min(len(elems), len(other))
	} else if len(other) > pairs {
		pairs = len(other)
	}
	out := make([]string, 0, pairs*2)
	for i := range pairs {
		out = append(out, elems[i%len(elems)], other[i%len(other)])
	}
	return out
}
