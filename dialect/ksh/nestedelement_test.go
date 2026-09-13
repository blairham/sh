// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// An array literal standing where one element's value goes makes that element
// an array of its own here — the third answer on a panel where bash refuses
// the line and zsh splices the words in (#2620, the second half).
//
// Every row measured on ksh93u+ 2012-08-01, 2026-09-13, `env -i` with a
// scratch HOME and `-c`. Before this the whole construct was
// `the shells disagree here and no dialect was chosen` at status 2, which is
// what 17 rows of the ksh corpus column were answering.
func TestAnArrayLiteralThroughASubscriptNests(t *testing.T) {
	for _, c := range []struct {
		src, want string
	}{
		// The length does not change and the element is not the words: `x p`
		// is element 0 and element 1's own first element, not `p q` spliced
		// in. A splice would have said `x p y` at three.
		{`a=(x y); a[1]=(p q); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`, "[x][p] n=2\n"},
		// And the words really are all still there, which is the row a store
		// keeping only the scalar view would pass the one above and fail.
		{`a=(x y); a[1]=(p q); typeset -p a`, "typeset -a a=(x (p q) )\n"},
		// An empty literal is an element holding an empty array, not a
		// removal and not the empty string: the count holds at three and the
		// listing keeps the parens.
		{`a=(x y z); a[1]=(); echo "n=${#a[@]}"; typeset -p a`, "n=3\ntypeset -a a=(x () z)\n"},
		// `+=` replaces an element holding a string and appends to one
		// already holding an array. The two halves do not follow from each
		// other — the `y` in the first row is gone rather than nested.
		{`a=(x y); a[1]+=(p); typeset -p a`, "typeset -a a=(x (p) )\n"},
		{`a=(x y); a[1]=(p q); a[1]+=(r); typeset -p a`, "typeset -a a=(x (p q r) )\n"},
		// A plain assignment through the subscript reaches the nested array's
		// own first element rather than flattening it.
		{`a=(x y); a[1]=(p q); a[1]=z; typeset -p a`, "typeset -a a=(x (z q) )\n"},
		{`a=(x y); a[1]=(p q); a[1]+=z; typeset -p a`, "typeset -a a=(x (pz q) )\n"},
		// Except over an empty one, which has no first element for the write
		// to reach: the element goes back to being a string.
		{`a=(x y); a[1]=(); a[1]=z; typeset -p a`, "typeset -a a=(x z)\n"},
		// A name holding a string is promoted rather than refused — the
		// splicing dialect refuses exactly this — and the string becomes
		// element zero.
		{`s=abc; s[1]=(p q); echo "after st=$?"; typeset -p s`, "after st=0\ntypeset -a s=(abc (p q) )\n"},
		// A subscript past the last element leaves a gap rather than padding
		// to it, because this dialect's arrays are sparse.
		{`a=(x y); a[5]=(p q); typeset -p a`, "typeset -a a=([0]=x [1]=y [5]=(p q) )\n"},
		{`unset a; a[3]=(x y); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`, "[x] n=1\n"},
		// The comma in a subscript is the arithmetic operator here and not
		// the separator of a range, so the literal lands at the right operand
		// — the same reading `a[2,3]=x` already takes. A range reading would
		// have replaced elements 2 and 3 and left three.
		{`a=(1 2 3); a[2,3]=(x y); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`, "[1][2][3][x] n=4\n"},
		// And the subscript still has to evaluate: a bad end ends the script
		// with the arithmetic's own complaint rather than anything about the
		// array.
		{`a=(1 2 3); a[2,3/0]=(x y); echo "st=$?"`, "ksh: 2,3/0: divide by zero\n"},
		// A keyed table takes the same value under a key, and is not refused
		// as a slice the way the splicing dialect refuses it: nothing here
		// replaces a span.
		{`typeset -A h; h[k]=v; h[a,b]=(x y); echo "st=$?"; typeset -p h`, "st=0\ntypeset -A h=([a,b]=(x y) [k]=v)\n"},
		// A literal placing its own elements nests an array with the hole in
		// it, which is what says the nested value is built by the same
		// placement the whole-array spelling uses.
		{`a=(x y); a[1]=([2]=z); typeset -p a`, "typeset -a a=(x ([2]=z) )\n"},
	} {
		out, _ := kshOut(t, c.src)
		if out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}

// And the answer is ksh's alone. bash refuses `a[1]=(p q)` outright and zsh
// splices the words in; a dialect that started nesting would be storing a
// value its own shell has no spelling for.
func TestNestingIsKshsAlone(t *testing.T) {
	for _, d := range []struct {
		name string
		want interp.SubscriptedArrayLiteralPolicy
		got  interp.SubscriptedArrayLiteralPolicy
	}{
		{"ksh", interp.SubscriptedArrayLiteralNests, ksh.Semantics().SubscriptedArrayLiteral},
		{"bash", interp.SubscriptedArrayLiteralRefused, bash.Semantics().SubscriptedArrayLiteral},
		{"zsh", interp.SubscriptedArrayLiteralSplices, zsh.Semantics().SubscriptedArrayLiteral},
	} {
		if d.got != d.want {
			t.Errorf("%s: SubscriptedArrayLiteral is %v, want %v", d.name, d.got, d.want)
		}
	}
}
