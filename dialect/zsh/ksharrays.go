// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// `ksharrays` is one name over six axes, and that is measured rather than
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
// The four that did not move are as much of the measurement as the six that
// did. `${a[1,2]}` is still a range and not the arithmetic comma operator;
// `${a[@]}` and `${a[*]}` are still every element; `${a[-1]}` still counts
// from the end; and a subscript on a scalar still reaches into its characters,
// `s=hello; ${s[1]}` being `e` under the option and `h` without it, which is
// the base moving rather than the construct going away.
//
// One function rather than six lines in three places: `setopt ksharrays`,
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
}
