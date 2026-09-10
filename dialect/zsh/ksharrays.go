// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// `ksharrays` is one name over five axes, and that is measured rather than
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
// The last is the one this file was written for (#1726). Under the option the
// brackets after an *unbraced* name stop being a subscript at all: `$a[1]` is
// `${a[0]}` — which the third and fourth rows are what make `xx` — followed by
// the three characters `[1]`. `${a[1]}` is unaffected, measured, because the
// braces settle where the expansion ends without having to ask.
//
// The four that did not move are as much of the measurement as the five that
// did. `${a[1,2]}` is still a range and not the arithmetic comma operator;
// `${a[@]}` and `${a[*]}` are still every element; `${a[-1]}` still counts
// from the end; and a subscript on a scalar still reaches into its characters,
// `s=hello; ${s[1]}` being `e` under the option and `h` without it, which is
// the base moving rather than the construct going away.
//
// One function rather than five lines in three places: `setopt ksharrays`,
// `unsetopt ksharrays` and `emulate` all place the same option, and a group
// spelled out at each of them is a group that drifts apart at one of them.
func setKshArrays(s *interp.Semantics, on bool) {
	s.ArrayBaseIsZero = answer(on)
	s.ArrayScalarIsTheWholeArray = answer(!on)
	s.ArrayLengthWithoutSubscriptIsCount = answer(!on)
	s.ArrayNameWithoutSubscriptIsTheList = answer(!on)
	s.BareSubscriptIsASubscript = answer(!on)
}
