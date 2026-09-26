// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// `ksharrays` is one name over eight axes, and that is measured rather than
// tidy. The option is described as moving the base, and what it actually does
// is make an array read the way the ksh family reads one, which is a
// different answer to every question this shell asks about a name that holds
// several values.
//
// Measured 2026-09-10 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`) and confirmed on
// zsh 5.9 (`/bin/zsh`), which agrees on every row, with `a=(xx yy zz)`:
//
//	                   ksharrays off   ksharrays on
//	${a[1]}            xx              yy            ArrayBaseIsZero
//	"$a"               xx yy zz        xx            ArrayScalarIsTheWholeArray
//	${#a}              3               2             ArrayLengthWithoutSubscriptIsCount
//	printf '<%s>' $a   <xx><yy><zz>    <xx>          ArrayNameWithoutSubscriptIsTheList
//	"$a[1]"            xx              xx[1]         BareSubscriptIsASubscript
//
// The fifth is the one this file was written for (#1726). Under the option the
// brackets after an *unbraced* name stop being a subscript at all: `$a[1]` is
// `${a[0]}` — which the third and fourth rows are what make `xx` — followed by
// the three characters `[1]`. `${a[1]}` is unaffected, measured, because the
// braces settle where the expansion ends without having to ask.
//
// **The sixth is a write rather than a read**, which is why it was missed for
// as long as it was: the five above are all about what a *word* comes to, and
// this one is about where a value lands. Measured 2026-09-26 on the same
// binary, with `a=(first second)`:
//
//	                   ksharrays off     ksharrays on
//	a+=last            first second last firstlast second
//	                                     ScalarAppendedToAnArrayBecomesANewElement
//
// So a scalar `+=` over a name holding an array joins the element at the base
// instead of growing one past the end, which is what the whole ksh family does
// without an option — measured the same day, bash 5.3.20, bash 3.2.57 and
// ksh93u+ 2012-08-01 all answer `firstlast second` with nothing set (#4609).
//
// **The noun is the scalar append over an array**, and three pairs hold one
// part of that fixed while another moves. The *written form*: with the option
// on, `a+=last` joins and the parenthesised `a+=(last)` still adds a third
// element, so it is not the option alone. The *name's kind*: `s=abc; s+=last`
// under the option is the ordinary string append, so the name has to be
// holding an array. And the *subscript*: `a[1]+=last` under the option reaches
// the element it names — `first secondlast` — so it is the **bare** name that
// moved and not every append.
//
// **It is answered when the assignment runs**, like every axis here and unlike
// a grammar flag. One body parsed once and called twice with the option moved
// between the calls gives `1 2 x` and then `1x 2`; an array built before the
// option is set joins all the same; and one built under the option and
// appended to after an `unsetopt` grows an element. A function's
// `setopt localoptions ksharrays` reaches an append inside it and not one
// after it returns.
//
// **The seventh is a refusal**, and it is the one that says the sixth's noun
// was still a shade too wide. Measured 2026-09-26 on the same binary:
//
//	                         ksharrays off     ksharrays on
//	typeset -A h=(one 1)
//	h=string                 typeset h=string  h: attempt to set associative
//	h+=string                typeset h=string    array to scalar, status 1,
//	                                             and the shell leaves
//	                                     ScalarStoredOverATableIsRefused
//
// A scalar store over a name holding a **table** is refused under the option,
// where the same line over an ordinary array is taken at the base — which is
// the sixth row above, and the pair that keeps the two apart. Both spellings
// ask it, an empty `typeset -A h` asks it, and a subscripted `h[one]=Q` does
// not: it is the bare name's kind and not the operator. `unset h` first and
// the store is an ordinary scalar again.
//
// It is **not** what the whole ksh family does, which is where it parts from
// the sixth: bash 5.3.20 and ksh93u+ 2012-08-01 both write the key `0` and
// keep the table. So the option imitates ksh on where an append lands and
// refuses outright where ksh stores, and one sentence about "reading an array
// the way the ksh family reads one" gets this row wrong (#4617).

// **The eighth is the sixth's other spelling**, and it could not be moved
// until the seventh existed. Measured 2026-09-26 on the same binary, with
// `a=(first second)`:
//
//	                   ksharrays off     ksharrays on
//	a=word             typeset a=word    typeset -a a=( word second )
//	                                     ScalarAssignedOverACompoundReplacesTheName
//
// A plain scalar store over a name holding an **ordinary array** writes the
// base element and leaves the rest standing, where without the option the
// value becomes the whole of the name and the array is gone. `a+=word` is the
// sixth axis and this is the same row's `=`; a change that moved one and not
// the other would be invisible to a probe that ran only the append, which is
// why the test asserts the two beside each other (#4618).
//
// **The noun is the bare name holding an ordinary array, taking a plain
// scalar store**, and four pairs hold it fixed while something else moves.
// The *shape of the store* is the sharpest, because both of its rows are a
// bare name on the left under the option and neither moves: `a=()` replaces
// the array with an empty one and `a+=(last)` grows a third element, as they
// both do without the option — so it is a **scalar** store and not any store.
// The *subscript*: `a[0]=word` already reaches the base and needs no axis to
// do it, so it is the **bare** name. The *name's kind*: a scalar and an unset
// name are the plain store in both columns, and a table is refused by the
// seventh axis ahead of this question entirely. And the *spelling*: `typeset
// a=word` over an array is `inconsistent type for assignment` in both
// columns, so a **declaration** is a different question — see
// interp.Semantics.ScalarUnderAnArrayDeclaration.
//
// It is what the whole ksh family does with nothing set, like the sixth and
// unlike the seventh: bash 5.3.20, bash 3.2.57 and ksh93u+ 2012-08-01 all
// answer `word second` there. And it reaches **every scalar store** rather
// than only an assignment statement, which is the field's own reach: under
// the option `for a in x y`, `read a`, `printf -v a x`, `${a::=x}` and
// `getopts x a` each set the base of the array the name is holding.
//
// The four that did not move are as much of the measurement as the seven that
// did. `${a[1,2]}` is still a range and not the arithmetic comma operator;
// `${a[@]}` and `${a[*]}` are still every element; `${a[-1]}` still counts
// from the end; and a subscript on a scalar still reaches into its characters,
// `s=hello; ${s[1]}` being `e` under the option and `h` without it, which is
// the base moving rather than the construct going away.
//
// One function rather than seven lines in three places: `setopt ksharrays`,
// `unsetopt ksharrays` and `emulate` all place the same option, and a group
// spelled out at each of them is a group that drifts apart at one of them.
// `emulate sh` and `emulate ksh` are how scripts reach it, and both were
// measured carrying the append row too.
func setKshArrays(s *interp.Semantics, on bool) {
	s.ArrayBaseIsZero = answer(on)
	s.ArrayScalarIsTheWholeArray = answer(!on)
	s.ArrayLengthWithoutSubscriptIsCount = answer(!on)
	s.ArrayNameWithoutSubscriptIsTheList = answer(!on)
	s.BareSubscriptIsASubscript = answer(!on)
	s.ScalarAppendedToAnArrayBecomesANewElement = answer(!on)
	s.ScalarStoredOverATableIsRefused = answer(on)
	s.ScalarAssignedOverACompoundReplacesTheName = answer(!on)
}
