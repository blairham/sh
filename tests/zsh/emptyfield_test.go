// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"fmt"
	"testing"
)

// An empty field is a field, in the three places this shell can be asked.
//
// One file because it is one question asked three ways, and the three answers
// were not in step: an array holding one empty string, the field a quoted
// `(s)` or `(f)` split leaves at each edge, and the count `${#name}` gives
// for either. Every case here answers with a *number*, which is the reason
// the file exists — a wrong one is a plausible answer at status 0, and there
// is no diagnostic anywhere to notice (#1097).
//
// Measured 2026-09-07 against zsh 5.9.2 with a scratch HOME and `-f`.

// `${#name}` of an array whose single element is the empty string.
//
// Counted, not measured: this dialect answers `${#a}` with the number of
// elements, so the answer does not depend on how long the element is. It read
// 0 for a while, because whether a name held an array was decided by looking
// at the scalar a one-element array is mirrored into, and the mirror of one
// empty element is indistinguishable from no array at all.
func TestAnArrayHoldingOneEmptyStringIsOneElement(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`x=(""); print -r -- "n=${#x}"`, "n=1\n"},
		{`x=("" ""); print -r -- "n=${#x}"`, "n=2\n"},
		{`x=(); print -r -- "n=${#x}"`, "n=0\n"},
		{`x=(""); print -r -- "n=${#x[@]}"`, "n=1\n"},
		{`x=(""); print -r -- "n=$#x"`, "n=1\n"},
		// The element is still empty, which is the half a count cannot show.
		{`x=(""); print -r -- "e=[${x[1]}]"`, "e=[]\n"},
		{`x=(""); typeset -p x`, "typeset -a x=( '' )\n"},
		// The shape that found it: a tied scalar assigned the empty string
		// splits into one empty field.
		{`typeset -T E e; E=""; print -r -- "n=${#e}"`, "n=1\n"},
		{`typeset -T E e; E=""; typeset -p e`, "typeset -aT E e=( '' )\n"},
	} {
		out, st := runZsh(t, dir, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// Which empty field a quoted `(s)` or `(f)` keeps: the one at each edge, and
// not the ones in between.
//
// The whole table is here rather than a representative row, because dropping
// every empty field passes `a::b` and fails the other ten, and keeping every
// one of them passes `":"` and fails `a::b`. Only the pair of rules is
// distinguishable, so only the pair is asserted.
func TestAQuotedSplitFlagKeepsTheFieldAtEachEdge(t *testing.T) {
	dir := t.TempDir()
	const count = `x=%s; set -- "${(s.:.)x}"; print -r -- "n=$# [${(j:|:)@}]"`
	for _, tc := range []struct{ value, want string }{
		{`""`, "n=1 []\n"},
		{`":"`, "n=2 [|]\n"},
		{`"::"`, "n=2 [|]\n"},
		{`":::"`, "n=2 [|]\n"},
		{`"a"`, "n=1 [a]\n"},
		{`"a:"`, "n=2 [a|]\n"},
		{`":a"`, "n=2 [|a]\n"},
		{`":a:"`, "n=3 [|a|]\n"},
		{`"a::b"`, "n=2 [a|b]\n"},
		{`"a:::b"`, "n=2 [a|b]\n"},
		{`"a::"`, "n=2 [a|]\n"},
		{`"::a"`, "n=2 [|a]\n"},
		{`"a:b"`, "n=2 [a|b]\n"},
	} {
		src := fmt.Sprintf(count, tc.value)
		out, st := runZsh(t, dir, src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", src, out, st, tc.want)
		}
	}
}

// Unquoted keeps none of them and `(@)` keeps all of them, which is what says
// the rule above belongs to the quoting rather than to the flag.
func TestTheEdgeFieldsBelongToTheQuoting(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`x=":"; set -- ${(s.:.)x}; print -r -- "n=$#"`, "n=0\n"},
		{`x="a:"; set -- ${(s.:.)x}; print -r -- "n=$#"`, "n=1\n"},
		{`x=""; set -- ${(s.:.)x}; print -r -- "n=$#"`, "n=0\n"},
		{`x=":"; set -- "${(@s.:.)x}"; print -r -- "n=$#"`, "n=2\n"},
		{`x="a::b"; set -- "${(@s.:.)x}"; print -r -- "n=$#"`, "n=3\n"},
		{`x=""; set -- "${(@s.:.)x}"; print -r -- "n=$#"`, "n=1\n"},
	} {
		out, st := runZsh(t, dir, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// An empty value split into *characters* is one empty field in quotes, the
// same as a split at any other separator.
//
// A character loop over an empty string naturally produces nothing, so this
// is the one separator where the two answers had to be made to agree rather
// than agreeing already — and once they do, whether the field survives is the
// edge question above and not a second rule.
func TestAnEmptyValueSplitIntoCharacters(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`x=""; set -- "${(s::)x}"; print -r -- "n=$#"`, "n=1\n"},
		{`x=""; set -- ${(s::)x}; print -r -- "n=$#"`, "n=0\n"},
		{`x=""; set -- "${(@s::)x}"; print -r -- "n=$#"`, "n=1\n"},
		{`x=""; set -- "${(f)x}"; print -r -- "n=$#"`, "n=1\n"},
		{`x=""; set -- ${(f)x}; print -r -- "n=$#"`, "n=0\n"},
		{`x="ab"; set -- "${(s::)x}"; print -r -- "n=$# [${(j:|:)@}]"`, "n=2 [a|b]\n"},
		{`x="a"; set -- "${(s::)x}"; print -r -- "n=$#"`, "n=1\n"},
		// And through an array literal, which is where the count is what a
		// script reads.
		{`x=("${(s::)}"); print -r -- "n=${#x}"`, "n=1\n"},
	} {
		out, st := runZsh(t, dir, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}
