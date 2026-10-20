// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `-T` takes a scalar, an array and at most a separator, and a **fourth**
// operand is refused (#5100).
//
// It was taken silently at 0, with everything past the separator ignored, so
// `typeset -T THIS will not work` declared a tie of `THIS` and `will` and said
// nothing about the rest of the line.
//
// Measured 2026-09-28 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*, so the reference is that shell and not
// another build of this one), script files under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME and standard input on the null device.
func TestAFourthOperandToTieIsRefused(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			"four operands", "f(){ typeset -T A a b c }\nf\n",
			"f:typeset: too many arguments for -T\n", 1,
		},
		{
			"and the sentence the issue was filed with",
			"f(){ typeset -T THIS will not work }\nf\n",
			"f:typeset: too many arguments for -T\n", 1,
		},
		{"seven", "f(){ typeset -T A a b c d e f }\nf\n", "f:typeset: too many arguments for -T\n", 1},
		// Three is the most there is room for, and two is the tie without a
		// separator: the controls that say this is a count and not a refusal
		// of the letter.
		{"three is the separator", "f(){ typeset -T A a : }\nf\n", "", 0},
		{"two is the tie", "f(){ typeset -T A a }\nf\n", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != tc.status {
				t.Errorf("out %q status %d, want %q at %d", out, st, tc.want, tc.status)
			}
		})
	}
	// Every word that reaches the letter says it, in its own name.
	for _, word := range []string{"typeset", "declare", "local", "export", "readonly"} {
		t.Run(word+" says it in its own name", func(t *testing.T) {
			src := "f(){ " + word + " -T A a b c }\nf\n"
			want := "f:" + word + ": too many arguments for -T\n"
			if out, st := runZsh(t, dir, src); out != want || st != 1 {
				t.Errorf("out %q status %d, want %q at 1", out, st, want)
			}
		})
	}
}

// The refusal is **first**, which is the opposite of where the frozen scalar's
// stands and is the part that had to be measured a row at a time: at four
// operands every other refusal this builtin has loses to it, and each of those
// is the winner at three, where there is no count to refuse.
func TestTheCountRefusalWinsOverTheOtherOperandRefusals(t *testing.T) {
	dir := t.TempDir()
	const many = "f:typeset: too many arguments for -T\n"
	for _, tc := range []struct{ name, tail, atThree, atFour string }{
		{"a name tied to itself", "A A", "f:typeset: can't tie a variable to itself: A\n", many},
		{"a scalar half that is not a name", "':' a", "f:typeset: not valid in this context: :\n", many},
		{"an array half that is not a name", "A ':'", "f:typeset: not valid in this context: :\n", many},
		{"a value on the array half", "A a=v", "f:typeset: second argument of tie must be array: a\n", many},
	} {
		t.Run(tc.name, func(t *testing.T) {
			three := "f(){ typeset -T " + tc.tail + " x }\nf\n"
			if out, st := runZsh(t, dir, three); out != tc.atThree || st != 1 {
				t.Errorf("three: out %q status %d, want %q at 1", out, st, tc.atThree)
			}
			four := "f(){ typeset -T " + tc.tail + " x y }\nf\n"
			if out, st := runZsh(t, dir, four); out != tc.atFour || st != 1 {
				t.Errorf("four: out %q status %d, want %q at 1", out, st, tc.atFour)
			}
		})
	}
	// The frozen scalar is the fifth, and it is the one whose own note says it
	// is *last* of the refusals — so it loses here too, which is the row that
	// says "first" was measured rather than inherited.
	t.Run("and a frozen scalar", func(t *testing.T) {
		three := "f(){ typeset -r RO=v; typeset -T RO ro x }\nf\n"
		if out, st := runZsh(t, dir, three); out != "f: read-only variable: RO\n" || st != 1 {
			t.Errorf("three: out %q status %d, want the readonly refusal at 1", out, st)
		}
		four := "f(){ typeset -r RO=v; typeset -T RO ro x y }\nf\n"
		if out, st := runZsh(t, dir, four); out != many || st != 1 {
			t.Errorf("four: out %q status %d, want %q at 1", out, st, many)
		}
	})
}

// Reported and run on, and the pair is not made.
func TestTheCountRefusalLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"neither name exists afterwards",
			"f(){ typeset -T A a b c; typeset -p A a }\nf\n",
			"f:typeset: too many arguments for -T\nf:typeset: no such variable: A\nf:typeset: no such variable: a\n",
		},
		{
			"and nothing is tied",
			"f(){ typeset -T A a b c; A=1:2; print -r -- \"[$a] [$A]\" }\nf\n",
			"f:typeset: too many arguments for -T\n[] [1:2]\n",
		},
		{
			"the next command runs",
			"f(){ typeset -T A a b c; print -r -- \"then $?\" }\nf\n",
			"f:typeset: too many arguments for -T\nthen 1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, dir, tc.src); out != tc.want {
				t.Errorf("out %q, want %q", out, tc.want)
			}
		})
	}
	// **The count is not disturbed by the operand reordering** this engine
	// does to an array literal, because a permutation does not change a
	// length — so the row that carries one is right here without waiting on
	// #5096, which is the separate defect that the *order* is lost.
	t.Run("a literal among the operands counts as one", func(t *testing.T) {
		src := "f(){ typeset -T A a=(1 2) b c }\nf\n"
		if out, st := runZsh(t, dir, src); out != "f:typeset: too many arguments for -T\n" || st != 1 {
			t.Errorf("out %q status %d, want the count refusal at 1", out, st)
		}
	})
}
