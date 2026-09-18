// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.TestTrailingConnectiveTakesAMissingOperand: a `-a` or a `-o` that
// ends the operand list is the connective it is, with a right operand that is
// missing and therefore false.
//
// The rows are a measurement of two columns of the panel, asked here by the
// axis and never by a shell's name — the axis's own documentation says where
// they were taken and under what.
//
// The status is the whole of the answer, which is why every row asserts one:
// the columns that hold this reading are **silent**, so a row graded on the
// diagnostic alone could not tell an answer from a refusal. `-a` against `-o`
// over the same left side is what says the missing operand is false rather
// than merely absent.

// connectiveTakesAMissingOperand answers the axis, with plain wordings so a
// refusal is recognizable without being any dialect's phrasing.
func connectiveTakesAMissingOperand(on Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.TestTrailingConnectiveTakesAMissingOperand = on
		r.Semantics = &s
		dg := Diagnostics{
			TestUnaryExpected:    "%[1]s: unknown operator",
			TestBinaryExpected:   "%[1]s: unknown operator",
			TestOperandExpected:  "argument expected",
			TestTooManyArguments: "too many arguments",
		}
		r.Diagnostics = &dg
	}
}

func TestATrailingConnectiveTakesAMissingOperandWhereTheDialectSaysSo(t *testing.T) {
	for _, tc := range []struct {
		src     string
		takes   int
		refuses int
		why     string
	}{
		// The two-word form, and the pair that says the missing operand is
		// false: the same left side answers 1 under `and` and 0 under `or`.
		{`test x -a`, 1, 2, "a true left side and a missing right one"},
		{`test x -o`, 0, 2, "the same left side under the other connective"},
		{`test "" -a`, 1, 2, "a false left side"},
		{`test "" -o`, 1, 2, "which leaves `or` false as well"},
		// It reaches every length rather than only the form POSIX gives its
		// own rule, which is what makes it an axis about the connective.
		{`test -z a -a`, 1, 2, "three words, the first an operator"},
		{`test a = b -a`, 1, 2, "three words that are a comparison"},
		{`test 1 -eq 1 -a`, 1, 2, "four, past the count rules"},
		{`test 1 -eq 1 -o`, 0, 2, "and the other connective there"},
		{`test x -a x -a`, 1, 2, "two connectives, the last one bare"},
		{`test \( x \) -a`, 1, 2, "behind a group"},
	} {
		for _, c := range []struct {
			on   Answer
			want int
		}{{Yes, tc.takes}, {No, tc.refuses}} {
			out, st := run(t, tc.src, connectiveTakesAMissingOperand(c.on))
			if st != c.want {
				t.Errorf("%v: %s = %d (%q), want %d — %s", c.on, tc.src, st, out, c.want, tc.why)
			}
			if c.on == Yes && strings.TrimSpace(out) != "" {
				t.Errorf("taking the question: %s said %q, and the columns that take it are silent", tc.src, out)
			}
		}
	}
}

// The count rules win first, and that is measured rather than an ordering
// this reader chose: three words are the both-set guard over two strings in
// every column, so a rule that read the last word as a connective before
// counting would make `[ a -a -a ]` false where the whole panel answers true.
func TestTheCountRulesStillWinOverATrailingConnective(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want int
	}{
		{`test a -a -a`, 0},
		{`test a -o -o`, 0},
		{`test a -a -o`, 0},
		{`test -a -a -a`, 0},
		// One word is a string and nothing else, so a lone connective is
		// true for being non-empty.
		{`test -a`, 0},
		{`test -o`, 0},
	} {
		for _, on := range []Answer{Yes, No} {
			if _, st := run(t, tc.src, connectiveTakesAMissingOperand(on)); st != tc.want {
				t.Errorf("%v: %s = %d, want %d", on, tc.src, st, tc.want)
			}
		}
	}
}

// And a grouping token in front of the connective is not a left side: a
// parenthesis nothing closed is still a parenthesis nothing closed, so the
// axis never gets that far and the refusal stands under either reading.
func TestAGroupingTokenIsNotTheConnectivesLeftSide(t *testing.T) {
	for _, on := range []Answer{Yes, No} {
		if _, st := run(t, `test \( -a`, connectiveTakesAMissingOperand(on)); st != 2 {
			t.Errorf("%v: `test ( -a` = %d, want 2", on, st)
		}
	}
}
