// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"github.com/blairham/sh/syntax"
)

// The three operators that choose which *elements* of a value survive, from
// docs/spec/grammar/parameter-expansion.md. One grammar in the panel has them
// (syntax.Dialect.ParamElementSelection), so what they mean here is that
// shell's answer, measured rather than inferred from the manual's wording.
//
//	${a:#pattern}   drop the elements the pattern matches
//	${a:|other}     drop the elements the array `other` holds
//	${a:*other}     keep only those
//
// The pattern is matched against a **whole** element and never a part of one,
// which is what separates `:#` from `#`: `${v:#hel*}` empties `hello` where
// `${v#hel*}` leaves `lo`. Measured on zsh 5.9.2, and it is also why an empty
// pattern removes only the empty string — `${(@)a:#}` on `("" one)` is `one`
// and on `(one two)` is both.
//
// The two set operators compare for **equality**, not by pattern, and name
// another array rather than taking a word: `${(@)a:|b}` reads `b`. An array
// that is not set holds nothing, so a difference against it keeps everything
// and an intersection with it keeps nothing — measured, and quietly, with no
// complaint about the name.

// selectsElements reports whether the operator is one of the three.
//
// Its own predicate rather than a case in elementOp, because the two families
// are opposites: an elementOp maps every element through a function and the
// count comes out the same, where these three change which elements there
// are. Folding them together made `${a[@]:#p}` map each element to itself.
func selectsElements(op syntax.ParamOp) bool {
	switch op {
	case syntax.ParamExclude, syntax.ParamSetDifference, syntax.ParamSetIntersection:
		return true
	}
	return false
}

// selectElements applies one of the three to a list, returning what survives.
//
// The operand is expanded once for the whole list rather than once per
// element, which is the rule every other operator inside `${ }` follows and
// is observable: a command substitution in the pattern runs a single time.
func (r *Runner) selectElements(e *syntax.ParamExpr, elems []string) []string {
	keep := r.elementKeeper(e)
	out := make([]string, 0, len(elems))
	for _, el := range elems {
		if keep(el) {
			out = append(out, el)
		}
	}
	return out
}

// elementKeeper expands the operand once and returns the test one element has
// to pass to survive.
func (r *Runner) elementKeeper(e *syntax.ParamExpr) func(string) bool {
	if e.Op == syntax.ParamExclude {
		// A pattern, so the *word* is what the matcher needs rather than its
		// text: quoting decides whether a metacharacter is one, and the same
		// axis that keeps `p="t*"; echo $p` from globbing keeps `${a:#$p}`
		// from matching anything but the two literal characters. Measured —
		// `p=t*; a=(one two); ${(@)a:#$p}` removes nothing in the shell that
		// has the operator.
		pattern := r.patternOf(e.Arg)
		return func(el string) bool { return !r.matchPatternR(pattern, el, false) }
	}
	// The set operators name an array. Its *elements* are the operand, and a
	// name nothing is stored under contributes none of them.
	other, _ := r.arrayElems(r.joinWord(e.Arg))
	held := make(map[string]bool, len(other))
	for _, v := range other {
		held[v] = true
	}
	if e.Op == syntax.ParamSetIntersection {
		return func(el string) bool { return held[el] }
	}
	return func(el string) bool { return !held[el] }
}

// selectScalar is the same three operators against a value that is one string
// rather than a list.
//
// A scalar is treated as the one-element list it is, so the answer is the
// value or nothing at all: `${v:#hel*}` on `hello` is empty and `${v:#xyz}`
// leaves it. That is measured and not merely consistent — it is the shape the
// plugin loader on this machine depends on, where `${${(M)path:#/*}:-$PWD/…}`
// asks "is this absolute" by keeping the value only when it matches.
func (r *Runner) selectScalar(e *syntax.ParamExpr, value string) string {
	if r.elementKeeper(e)(value) {
		return value
	}
	return ""
}
