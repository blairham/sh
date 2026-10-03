// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"unicode"

	"github.com/blairham/sh/syntax"
)

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
	if isPositional(name) {
		// A positional parameter is always there to zip with: one past `$#`
		// is a single empty element and not an absent array, so `p q` zipped
		// with an unset `$1` is `p` and an empty word, measured. `0` is the
		// shell's name, as everywhere else a digit run worth nothing is.
		other, named = []string{r.positionalForZip(name)}, true
	}
	if !named && r.nounset && (e.Op == syntax.ParamZip || e.Op == syntax.ParamZipCycle) {
		// The zips read the operand as a parameter, and NO_UNSET refuses
		// one that is not set — the empty name included, `${x:^}` being
		// `: parameter not set`. The set operators do not: `${x:|nope}`
		// under the same option is `p q`. Measured on zsh 5.9.2, 2026-10-03.
		r.fatalUnsetParameter("%s\n", Wording(r.diag().UnboundVariable, "%s: parameter not set", name))
		return nil
	}
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

// namesTheOtherArray reports whether a zip or set operator's operand can be the
// name it has to be, and refuses the expansion when it cannot.
//
// The operand is read as a name and never expanded, so the test is on what was
// written: anything but identifier characters is refused, quoted or not, and
// whatever the parameter on the left holds — unset, a scalar, a list.
// Measured on zsh 5.9.2, 2026-10-03, under `-c`:
//
//	${x:^^^y}  ${x:^-y}  ${x:|^y}  ${x:*^y}   not an identifier: ^y (-y, ^y, ^y)
//	${x:^$y}   ${x:^"b"}  ${x:^\b}             not an identifier: $y ("b", \b)
//	${x:^@}    ${x:^y[1]}  ${x:^ y}             not an identifier: @ (y[1],  y)
//	${x:^}     ${x:^^}  ${x:|}                  no operand, no zip: `p q`
//	${x:^12a}  ${x:^_a}  ${x:^é}                accepted
//
// It ends the script at status 1, prints nothing of the command it was in, and
// is an expansion's failure rather than a parse's: `false && print ${x:^-y}`
// runs on.
func (r *Runner) namesTheOtherArray(e *syntax.ParamExpr) bool {
	switch e.Op {
	case syntax.ParamZip, syntax.ParamZipCycle, syntax.ParamSetDifference, syntax.ParamSetIntersection:
	default:
		return true
	}
	written := e.ArgText
	if written == "" && e.Arg != nil {
		// A node the parser did not fill; the word is all there is.
		written = syntax.PrintWord(e.Arg)
	}
	if identifierCharacters(written) {
		return true
	}
	r.diagf("not an identifier: %s\n", written)
	r.expandErr = true
	return false
}

// identifierCharacters reports whether every character of s may be in a name,
// a leading digit included.
func identifierCharacters(s string) bool {
	for _, c := range s {
		if c != '_' && !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			return false
		}
	}
	return true
}

// positionalForZip is the value of a positional parameter named by digits,
// empty past the end of the list.
func (r *Runner) positionalForZip(name string) string {
	n, _ := atoi(name)
	if n == 0 {
		v, _ := r.dollarZero()
		return v
	}
	if p := r.params(); n <= len(p) {
		return p[n-1]
	}
	return ""
}
