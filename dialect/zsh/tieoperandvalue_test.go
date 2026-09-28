// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// What each of `-T`'s operands may **hold** (#5103).
//
// The three positions take three different kinds of thing — a scalar, an array
// and a join character — and three refusals for an operand holding the wrong
// kind were missing. Each was silence at 0.
//
// Measured 2026-09-28 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*, so the reference is that shell and not
// another build of this one), script files under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME and standard input on the null device.
func TestWhatEachTieOperandMayHold(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			"an array literal on the scalar half",
			"f(){ typeset -T A=(1 2) a }\nf\n",
			"f:typeset: first argument of tie must be scalar: A\n", 1,
		},
		{
			"a value on both halves at once",
			"f(){ typeset -T A=v a=(x y) }\nf\n",
			"f:typeset: only one tied parameter can have value: A\n", 1,
		},
		{
			"a value on the separator",
			"f(){ typeset -T A a c=v }\nf\n",
			"f:typeset: third argument of tie must be join character\n", 1,
		},
		{
			"and an array literal there",
			"f(){ typeset -T A a b=(2) }\nf\n",
			"f:typeset: third argument of tie must be join character\n", 1,
		},
		// The controls. One value on its own is what a tie is for, and the
		// separator's *shape* is not being refused.
		{"a value on the scalar alone", "typeset -T A=v a\n" + `print -r -- "[$A]"` + "\n", "[v]\n", 0},
		{"a literal on the array alone", "typeset -T A a=(x y)\n" + `print -r -- "[$A]"` + "\n", "[x:y]\n", 0},
		{"a separator of two characters", "f(){ typeset -T A a '::' }\nf\n", "", 0},
		{"a separator that is a word", "f(){ typeset -T A a ab }\nf\n", "", 0},
		{"a separator with nothing in it", "f(){ typeset -T A a '' }\nf\n", "", 0},
		{"three plain names", "f(){ typeset -T A a b }\nf\n", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != tc.status {
				t.Errorf("out %q status %d, want %q at %d", out, st, tc.want, tc.status)
			}
		})
	}
	// An empty value is a value, on either side.
	for _, tc := range []struct{ name, src string }{
		{"an empty scalar value", "f(){ typeset -T A= a=(x y) }\nf\n"},
		{"an empty array literal", "f(){ typeset -T A=v a=() }\nf\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := "f:typeset: only one tied parameter can have value: A\n"
			if out, st := runZsh(t, dir, tc.src); out != want || st != 1 {
				t.Errorf("out %q status %d, want %q at 1", out, st, want)
			}
		})
	}
	// Every word that reaches the letter writes its own name in the location.
	for _, word := range []string{"typeset", "declare", "local", "export", "readonly"} {
		t.Run(word+" says it in its own name", func(t *testing.T) {
			src := "f(){ " + word + " -T A=(1) a }\nf\n"
			want := "f:" + word + ": first argument of tie must be scalar: A\n"
			if out, st := runZsh(t, dir, src); out != want || st != 1 {
				t.Errorf("out %q status %d, want %q at 1", out, st, want)
			}
		})
	}
}

// **The position decides, not the name.** The two lines below hold the same
// three names and differ only in which position the array literal was written
// at, and the reference answers them differently — so a rule that asked "is
// there a literal named `a`" would be right about one of them by luck.
//
// This is why the check reads Runner.arrayOperands, which records each
// literal's index in the command's word list (#5096), rather than
// Runner.literalOperands, which is a set of names.
func TestTheSeparatorRuleAsksThePositionAndNotTheName(t *testing.T) {
	dir := t.TempDir()
	const refused = "f:typeset: third argument of tie must be join character\n"
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{"the literal is the third operand", "f(){ typeset -T A a a=(1) }\nf\n", refused, 1},
		{"the literal is the array half", "f(){ typeset -T A a=(1) a }\nf\n", "", 0},
		// The same pair with a name that is not shared, which is the control
		// saying the first row is not about the repetition.
		{"and with distinct names", "f(){ typeset -T A b a=(1) }\nf\n", refused, 1},
		{"the other way round", "f(){ typeset -T A a=(1) b }\nf\n", "", 0},
		{"the array half repeats itself", "f(){ typeset -T A b=(1) b }\nf\n", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != tc.status {
				t.Errorf("out %q status %d, want %q at %d", out, st, tc.want, tc.status)
			}
		})
	}
}

// The three refusals in the order this shell puts them, each pair of neighbors
// pinned by one line: the line below is refused by the rule above it.
func TestTheOrderOfTheTiesOperandRefusals(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"the count wins over a value on both halves",
			"f(){ typeset -T A=v a=(1) b=(2) c=(3) }\nf\n",
			"f:typeset: too many arguments for -T\n",
		},
		{
			"the scalar half's literal wins over the array half's value",
			"f(){ typeset -T A=(1) a=v }\nf\n",
			"f:typeset: first argument of tie must be scalar: A\n",
		},
		{
			"the array half's value wins over a name tied to itself",
			"f(){ typeset -T A A=v a=(1) }\nf\n",
			"f:typeset: second argument of tie must be array: A\n",
		},
		{
			"a name tied to itself wins over a value on both halves",
			"f(){ typeset -T A=v A=(1) }\nf\n",
			"f:typeset: can't tie a variable to itself: A\n",
		},
		{
			"a value on both halves wins over a value on the separator",
			"f(){ typeset -T A=v a=(1) b=(2) }\nf\n",
			"f:typeset: only one tied parameter can have value: A\n",
		},
		{
			"a value on the separator wins over a name that is not one",
			"f(){ typeset -T ':' a=(1) b=(2) }\nf\n",
			"f:typeset: third argument of tie must be join character\n",
		},
		{
			"and a name that is not one wins over a frozen scalar",
			"f(){ typeset -r RO=v; typeset -T RO ro b=(2) }\nf\n",
			"f:typeset: third argument of tie must be join character\n",
		},
		// The name checks moved behind a name tied to itself as part of this,
		// which is measured the same way: the pair `':' ':'` is both, and the
		// reference says which one it is.
		{
			"a name tied to itself wins over a name that is not one",
			"f(){ typeset -T ':' ':' }\nf\n",
			"f:typeset: can't tie a variable to itself: :\n",
		},
		{
			"and the same with a different non-name",
			"f(){ typeset -T ']x' ']x' }\nf\n",
			"f:typeset: can't tie a variable to itself: ]x\n",
		},
		// What each of those still answers on its own, which is the half that
		// had to stay still.
		{"a non-name in the scalar half alone", "f(){ typeset -T ':' a }\nf\n", "f:typeset: not valid in this context: :\n"},
		{"a non-name in the array half alone", "f(){ typeset -T A ':' }\nf\n", "f:typeset: not valid in this context: :\n"},
		{"two plain names that are the same", "f(){ typeset -T A A }\nf\n", "f:typeset: can't tie a variable to itself: A\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 1 {
				t.Errorf("out %q status %d, want %q at 1", out, st, tc.want)
			}
		})
	}
}

// **One of the three is fatal and the other two are not**, which is measured
// rather than assumed — the neighbors on either side of it in the order above
// both run the next command.
func TestOnlyTheValueOnBothHalvesIsFatal(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"a value on both halves ends the script",
			"typeset -T A=v a=(x y)\nprint -r -- next\n",
			"zsh:typeset:1: only one tied parameter can have value: A\n",
		},
		{
			"the scalar half's literal does not",
			"typeset -T A=(1) a\nprint -r -- next\n",
			"zsh:typeset:1: first argument of tie must be scalar: A\nnext\n",
		},
		{
			"nor does the separator's value",
			"typeset -T A a=(1) b=(2)\nprint -r -- next\n",
			"zsh:typeset:1: third argument of tie must be join character\nnext\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, dir, tc.src); out != tc.want {
				t.Errorf("out %q, want %q", out, tc.want)
			}
		})
	}
	// And none of the three makes the pair: the names do not exist afterwards
	// and a write to one does not reach the other.
	t.Run("the refused tie is not made", func(t *testing.T) {
		src := "f(){ typeset -T A a=(1) b=(2); A=1:2; print -r -- \"[${a[@]}] [$A]\" }\nf\n"
		want := "f:typeset: third argument of tie must be join character\n[] [1:2]\n"
		if out, _ := runZsh(t, dir, src); out != want {
			t.Errorf("out %q, want %q", out, want)
		}
	})
}
