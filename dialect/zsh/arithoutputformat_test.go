// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// `[#base]` inside an arithmetic expression says what base the *result* is
// written in, end to end through this dialect.
//
// Measured 2026-09-12 on zsh 5.9.2, which is the whole panel for this: bash
// 5.3.15, bash 3.2.57, bash as `sh`, ksh93u+ and dash all read the `[` as an
// operand they cannot have and call it an arithmetic syntax error.
//
// The two spellings are one `#` apart and that is the whole of the difference
// between them: one writes the `base#` in front of the digits and two write
// the digits alone. It was found on a real interactive start, where a prompt
// theme uses it to render a color as hex — and the failure was a *parse*
// refusal rather than a wrong number, so the expression produced nothing at
// all (#2095).
func TestTheArithmeticOutputBase(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a base writes its mark", `echo $(( [#16] 255 ))`, "16#FF\n"},
		{"two hashes drop the mark", `echo $(( [##16] 255 ))`, "FF\n"},
		{"the smallest base", `echo $(( [#2] 5 )) $(( [##2] 5 ))`, "2#101 101\n"},
		{"the largest base", `echo $(( [#36] 1295 )) $(( [##36] 1295 ))`, "36#ZZ ZZ\n"},
		{"an octal base", `echo $(( [#8] 8 ))`, "8#10\n"},
		{"base ten marks nothing either way", `echo $(( [#10] 255 )) $(( [##10] 255 ))`, "255 255\n"},
		{"zero", `echo $(( [#16] 0 )) $(( [##16] 0 ))`, "16#0 0\n"},
		{"a negative keeps its sign outside the mark", `echo $(( [#16] -255 )) $(( [##16] -255 ))`, "-16#FF -FF\n"},
		{"the whole expression is formatted", `echo $(( [#16] 255 + 1 ))`, "16#100\n"},
		{"a specifier with nothing after it", `echo $(( [#16] ))`, "16#0\n"},
		{"a value read in one base and written in another", `echo $(( [#16] 0x1f ))`, "16#1F\n"},
		{"a float under a base is truncated", `echo $(( [#16] 3.5 ))`, "16#3\n"},
		{"the alternative expansion", `echo $[ [#16] 255 ]`, "16#FF\n"},
		{"through let", `let "x = [#16] 255"; echo $x`, "16#FF\n"},
		{"through the arithmetic command", `x=5; (( x = [#16] 255 )); echo $x`, "16#FF\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// An `_` in the specifier groups the digits, which is the rest of the
// construct and is measured with it: a bare `_` is decimal in threes, a number
// after it is the group size, and `_0` turns grouping off again.
func TestTheArithmeticOutputGrouping(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"decimal in threes", `echo $(( [#_] 1234567 ))`, "1_234_567\n"},
		{"a group size", `echo $(( [#_5] 1234567 ))`, "12_34567\n"},
		{"grouping under a base", `echo $(( [#16_4] 1048575 ))`, "16#F_FFFF\n"},
		{"grouping with no mark", `echo $(( [##16_4] 1048575 ))`, "F_FFFF\n"},
		{"a bare underscore after a base is three", `echo $(( [#16_] 1048575 ))`, "16#FF_FFF\n"},
		{"grouping turned off", `echo $(( [#16_0] 1048575 )) $(( [#_0] 1234567 ))`, "16#FFFFF 1234567\n"},
		{"base ten groups either way", `echo $(( [#10_3] 1234567 )) $(( [##10_3] 1234567 ))`, "1_234_567 1_234_567\n"},
		{"a grouped negative", `echo $(( [#_] -1234567 ))`, "-1_234_567\n"},
		{"no base leaves a float alone", `echo $(( [#_3] 1234567.5 ))`, "1_234_567.5\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The specifier is lexical: it stands where a token may rather than where an
// operand may, it takes effect from a branch that is never evaluated, and the
// textually last one decides. Each row is a reading that a prefix operator
// over the expression beside it would get wrong.
func TestTheArithmeticOutputBaseIsLexical(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"in the middle of an expression", `echo $(( 255 + [#16] 1 ))`, "16#100\n"},
		{"after a value", `echo $(( 2[#8] ))`, "8#2\n"},
		{"after a name", `echo $(( a [#8] ))`, "8#0\n"},
		{"in a branch never taken", `echo $(( 0 ? [#16] 1 : 2 ))`, "16#2\n"},
		{"past a short circuit", `echo $(( 0 && [#16] 1 ))`, "16#0\n"},
		{"before an assignment", `echo $(( [#16] x = 255 ))`, "16#FF\n"},
		{"the last of several wins", `echo $(( [#16] 255 + [#8] 1 ))`, "8#400\n"},
		{"two touching", `echo $(( [#16][#8] 255 ))`, "8#377\n"},
		{"inside parentheses", `echo $(( ([#16] 255) ))`, "16#FF\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// It formats the answer and never changes it, and it belongs to the one
// evaluation that carried it. The second half is what a leaked format would
// fail: the next expansion is plain decimal again.
func TestTheArithmeticOutputBaseChangesNoValueAndDoesNotLeak(t *testing.T) {
	out, st := answersRun(t, `echo $(( [#16] 255 )) $(( 255 )) $(( 16#FF ))`)
	if out != "16#FF 255 255\n" || st != 0 {
		t.Errorf("gave %q at %d, want %q at 0", out, st, "16#FF 255 255\n")
	}
}

// An assignment inside the expression stores the *formatted* text, which is
// why the value reads back as the base it was written in. The subscript of the
// same assignment is not formatted, which is what tells the value apart from
// every other text the write touches.
func TestTheArithmeticOutputBaseReachesTheValueAndNotTheKey(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a scalar", `x=5; (( x = [#16] 255 )); typeset -p x`, "typeset x='16#FF'\n"},
		{"read back", `x=5; (( x = [#16] 255 )); echo $(( x ))`, "255\n"},
		{"an element", `a=(1 2 3); (( [#16] a[2] = 9 )); typeset -p a`, "typeset -a a=( 1 '16#9' 3 )\n"},
		{"an associative key", `typeset -A m; (( [#16] m[255] = 1 )); typeset -p m`, "typeset -A m=( [255]='16#1' )\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The boundary with the integer attribute, which is the other construct that
// spells a base the same way. A name that already has the attribute does not
// take its base from an expression's format — the format renders an answer, it
// does not write a literal — where the same characters arriving as the text of
// an ordinary assignment do teach one.
func TestTheArithmeticOutputBaseTeachesAnIntegerNameNothing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an arithmetic assignment teaches nothing", `typeset -i i; (( i = [#16] 255 )); typeset -p i; echo $i`, "typeset -i i=255\n255\n"},
		{"an ordinary assignment still does", `typeset -i i; i=$(( [#16] 255 )); typeset -p i`, "typeset -i16 i=255\n"},
		{"a base the name already has stands", `typeset -i16 i; (( i = [#8] 255 )); typeset -p i; echo $i`, "typeset -i16 i=255\n16#FF\n"},
		{"and a loop counter stays plain", `typeset -i i; for (( i=[#16] 0; i<2; i++ )); do echo $i; done`, "0\n1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The three refusals, which are three different sentences: a base outside the
// range, a bracketed group that is not a specifier at all, and one holding
// nothing but digits — the last two being one character apart.
func TestTheArithmeticOutputBaseRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{"a base past the end", `echo $(( [#37] 5 ))`, "zsh:1: invalid base (must be 2 to 36 inclusive): 37\n", 1},
		{"a base below the start", `echo $(( [#1] 5 ))`, "zsh:1: invalid base (must be 2 to 36 inclusive): 1\n", 1},
		{"zero is a base, not the absence of one", `echo $(( [#0] 5 ))`, "zsh:1: invalid base (must be 2 to 36 inclusive): 0\n", 1},
		{"and with grouping beside it", `echo $(( [#0_3] 5 ))`, "zsh:1: invalid base (must be 2 to 36 inclusive): 0\n", 1},
		{"a blank in the specifier", `echo $(( [# 16] 5 ))`, "zsh:1: bad output format specification\n", 1},
		{"a blank at the end of it", `echo $(( [#16 ] 5 ))`, "zsh:1: bad output format specification\n", 1},
		{"no base and no grouping", `echo $(( [#] 5 ))`, "zsh:1: bad output format specification\n", 1},
		{"nothing in the brackets", `echo $(( [] 5 ))`, "zsh:1: bad output format specification\n", 1},
		{"a name in the brackets", `echo $(( [foo] 5 ))`, "zsh:1: bad output format specification\n", 1},
		{"three hashes", `echo $(( [###16] 5 ))`, "zsh:1: bad output format specification\n", 1},
		{"digits alone are a different sentence", `echo $(( [16] 255 ))`, "zsh:1: bad base syntax\n", 1},
		{"and anywhere in the expression", `echo $(( 1 + [16] 2 ))`, "zsh:1: bad base syntax\n", 1},
		{"the command form carries this shell's status", `(( [#37] 5 ))`, "zsh:1: invalid base (must be 2 to 36 inclusive): 37\n", 2},
		{"and for an unreadable specifier too", `(( [# 16] 5 ))`, "zsh:1: bad output format specification\n", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s gave %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}

// A refusal is raised when the expression runs and not when the file is read,
// which is what keeps a branch nobody takes quiet — measured, the same script
// reaches the echo after it.
func TestAnUnreadableOutputFormatIsARunTimeFailure(t *testing.T) {
	out, st := answersRun(t, `false && echo $(( [#37] 1 )); echo reached`)
	if out != "reached\n" || st != 0 {
		t.Errorf("gave %q at %d, want %q at 0", out, st, "reached\n")
	}
}

// A subscript is untouched: its bracket touches the name in front of it and is
// read by the name, where a specifier stands where a token begins.
func TestASubscriptIsNotAnOutputFormat(t *testing.T) {
	out, st := answersRun(t, `a=(1 2 3); echo $(( a[#8] )) $(( a[2] ))`)
	if out != "0 2\n" || st != 0 {
		t.Errorf("gave %q at %d, want %q at 0", out, st, "0 2\n")
	}
}
