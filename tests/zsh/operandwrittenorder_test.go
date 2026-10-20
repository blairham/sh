// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A declaration's **array-literal operand** keeps the place it was written
// (#5096).
//
// The parser hands the two halves of `typeset a=(x y) b` over separately — the
// word `b` in `c.Args`, the literal in `c.Assigns` — and this engine rebuilt
// the argument list by walking the words and appending each operand's name at
// the end. The list a utility was handed was therefore a permutation of the
// one that was written.
//
// Measured 2026-09-28 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*, so the reference is that shell and not
// another build of this one), script files under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME and standard input on the null device.
//
// **Two surfaces, one record.** The trace here and the positional operands of
// `-T` below are the same defect seen from two sides, and a fix keyed on
// either one alone leaves the other wrong — which is why the order is restored
// in argv itself. The two test functions are deliberately kept apart so that a
// change which fixes one and not the other cannot pass.
func TestAnArrayOperandIsTracedWhereItWasWritten(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`typeset a=(x y) b`, "> typeset a=( x y ) b\n"},
		{`typeset -a q=(1) r=(2) s`, "> typeset -a q=( 1 ) r=( 2 ) s\n"},
		{`export v=(1 2) w`, "> export v=( 1 2 ) w\n"},
		{`typeset s=plain a=(x y) t`, "> typeset s=plain a=( x y ) t\n"},
		{`local a=(1) b`, "> local a=( 1 ) b\n"},
		{`readonly a=(1) b`, "> readonly a=( 1 ) b\n"},
		{`declare a=(1) b`, "> declare a=( 1 ) b\n"},
		{`typeset -A h=(k v) g`, "> typeset -A h=( k v ) g\n"},
		{`typeset a=(1) b c`, "> typeset a=( 1 ) b c\n"},
		{`typeset b c a=(1)`, "> typeset b c a=( 1 )\n"},
		{`typeset a=() b`, "> typeset a=( ) b\n"},
		{`typeset a=($(echo p)) b`, "> typeset a=( p ) b\n"},
		// The literal in the middle of two plain words, which is the shape a
		// rule that merely sorts the operands to the front would also get
		// wrong.
		{`typeset b a=(1) c`, "> typeset b a=( 1 ) c\n"},
		// Two literals keep their order with respect to each other as well as
		// to the words.
		{`typeset a=(1) b=(2)`, "> typeset a=( 1 ) b=( 2 )\n"},
		{`typeset b=(2) a=(1)`, "> typeset b=( 2 ) a=( 1 )\n"},
		{`typeset x a=(1) y b=(2) z`, "> typeset x a=( 1 ) y b=( 2 ) z\n"},
		// Already right before this change, and they stay right: no operand
		// to move, or the operand written last anyway.
		{`typeset a=(x y)`, "> typeset a=( x y )\n"},
		{`typeset b a=(x y)`, "> typeset b a=( x y )\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, _ := runZsh(t, dir, "set -x\n"+tc.src+"\n")
			if !strings.Contains(out, tc.want) {
				t.Errorf("%q\n does not carry %q", out, tc.want)
			}
		})
	}
}

// The other surface: `-T`'s operands are **positional**, so the permutation put
// the separator in the array half's place and it was refused as a name.
func TestATieReadsItsOperandsAsWritten(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			"a separator after an array literal",
			"typeset -T A a=(x y) +\n" + `print -r -- "[$A]"` + "\n", "[x+y]\n", 0,
		},
		{
			"and under local, inside a function",
			"f(){ local -T A a=(x y) +; print -r -- \"[$A]\" }\nf\n", "[x+y]\n", 0,
		},
		{
			"the listing writes the pair back",
			"typeset -T A a=(x y) +\ntypeset -p A a\n",
			"typeset -T A a=( x y ) +\ntypeset -aT A a=( x y ) +\n", 0,
		},
		// The controls. The same line without the literal was always right,
		// which is what says this is the operand's *place* and not the
		// separator itself; and the same line without the separator was too.
		{
			"a separator with no literal in the line",
			"typeset -T A a +\nA=p+q\n" + `print -r -- "[${a[@]}]"` + "\n", "[p q]\n", 0,
		},
		{
			"a literal with no separator",
			"typeset -T A a=(x y)\n" + `print -r -- "[$A]"` + "\n", "[x:y]\n", 0,
		},
		// And the count refusal still counts the same operands (#5100), which
		// is the row where the two changes meet.
		{
			"a fourth operand is still too many",
			"f(){ typeset -T A a=(x y) + extra }\nf\n",
			"f:typeset: too many arguments for -T\n", 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != tc.status {
				t.Errorf("out %q status %d, want %q at %d", out, st, tc.want, tc.status)
			}
		})
	}
}

// What the operands *mean* does not move with them, which is the half of this
// that had to stay still: the names are declared with the same attributes and
// the same values, and a refusal still names the same operand.
func TestMovingTheOperandDoesNotChangeWhatItDeclares(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"the types", "typeset a=(1) b\n" + `print -r -- "${(t)a} ${(t)b}"` + "\n", "array scalar\n"},
		{"written the other way round", "typeset b a=(1)\n" + `print -r -- "${(t)a} ${(t)b}"` + "\n", "array scalar\n"},
		{
			"a letter reaches both", "typeset -U u=(1 1 2) t\n" + `print -r -- "[${u[@]}] ${(t)t}"` + "\n",
			"[1 2] scalar-unique\n",
		},
		{
			"a scope reaches both", "f(){ local v=(1) w; print -r -- \"${(t)v} ${(t)w}\" }\nf\n",
			"array-local scalar-local\n",
		},
		{
			"and a refusal still names its own operand",
			"typeset -i n=3 a=(1) m\n", "zsh:typeset:1: a: inconsistent type for assignment\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, dir, tc.src); out != tc.want {
				t.Errorf("out %q, want %q", out, tc.want)
			}
		})
	}
}
