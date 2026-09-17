// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.TestReadsOneExpressionOffTheOperands: `test` reads one expression
// off the *front* of its operand list and never looks at the words behind it.
//
// The rows are a measurement of one column of the panel, and they are asked
// here by the axis rather than by the shell's name — see the axis's own
// documentation for where they were taken and under what.
//
// Two things are asserted about every row. The status, because half of these
// lists have no diagnostic at all and the status is the whole of the answer;
// and the text, because the three complaints this reading has are about a
// *word* or about the grammar, and a row graded on the status alone would
// read `argument expected` and `b: unknown operator` as the same refusal.

// frontReader answers the axis yes and spells the sentences the reading
// needs plainly, so an assertion is about which complaint was raised rather
// than about one dialect's phrasing of it.
func frontReader(r *Runner) {
	s := *r.Semantics
	s.TestReadsOneExpressionOffTheOperands = Yes
	s.TestHasTheFileExistsLetter = Yes
	s.TestHasTheShellOptionOperator = Yes
	r.Semantics = &s
	dg := Diagnostics{
		TestUnaryExpected:    "%[1]s: unknown operator",
		TestBinaryExpected:   "%[1]s: unknown operator",
		TestOperandExpected:  "argument expected",
		TestIncorrectSyntax:  "incorrect syntax",
		TestTooManyArguments: "too many arguments",
	}
	r.Diagnostics = &dg
}

// wholeList is the same wordings with the axis answered the other way, so a
// row can be shown to move rather than merely to pass.
func wholeList(r *Runner) {
	frontReader(r)
	s := *r.Semantics
	s.TestReadsOneExpressionOffTheOperands = No
	r.Semantics = &s
}

// frontRows is every row the reading is built from. want is the status and
// text is a phrase the complaint must contain, empty where the row is silent.
var frontRows = []struct {
	src  string
	want int
	text string
}{
	// The headline: an expression with words behind it, in each of the
	// readings that can have them.
	{`test x = x y`, 0, ""},
	{`test x = x junk1 junk2 junk3`, 0, ""},
	{`test -n x y`, 0, ""},
	{`test -n x y z w v`, 0, ""},
	{`test 1 -eq 1 2`, 0, ""},
	{`test ! x = x junk`, 1, ""},
	{`test ! ! x = x junk`, 0, ""},
	{`test \( x = x \) junk`, 0, ""},
	{`test a = b = c`, 1, ""},
	{`test x = x \)`, 0, ""},

	// The seam. The same words, and only the connective has moved.
	{`test x = x junk -a junk`, 0, ""},
	{`test x = x -a x = x junk`, 2, "incorrect syntax"},
	{`test -n x -a -n y junk`, 2, "incorrect syntax"},
	{`test -n x y -o z`, 0, ""},
	// A connective inside a group is not the seam: the words after the
	// group are still dropped.
	{`test \( x = x -a y = z \) junk`, 1, ""},

	// Three operands are their own reading, and the third word is never
	// looked at. Four is where the grammar starts, which is why the same
	// trailing connective is taken there and not here.
	{`test -n x -a`, 0, ""},
	{`test -z "" -a`, 0, ""},
	{`test x = x -a`, 2, "argument expected"},

	// The order of the three-word questions.
	{`test ! -a /`, 1, ""},
	{`test ! = x`, 1, ""},
	{`test -n = x`, 1, ""},
	{`test -z -a y`, 1, ""},
	{`test -n -a y`, 0, ""},
	{`test x -a y`, 0, ""},
	{`test "" -a x`, 1, ""},
	{`test \( x \)`, 0, ""},
	{`test \( = \)`, 1, ""},

	// And the same order inside the grammar, where a connective in the
	// operator's place ends the primary instead of joining two strings.
	{`test -n = x y`, 1, ""},
	{`test -n -a y z`, 0, ""},
	{`test -z -a y z`, 1, ""},
	{`test x -a y z`, 2, "argument expected"},
	{`test x y -a z`, 2, "y: unknown operator"},

	// The complaints, and which word each names.
	{`test a b c`, 2, "b: unknown operator"},
	{`test a b c d`, 2, "b: unknown operator"},
	{`test a b c -a x = x`, 2, "b: unknown operator"},
	{`test -Q x y`, 2, "-Q: unknown operator"},
	{`test -Q x y z`, 2, "x: unknown operator"},
	{`test -QQ x y`, 2, "x: unknown operator"},
	{`test - x y`, 2, "-: unknown operator"},
	{`test x junk`, 2, "argument expected"},
	{`test -Q x`, 2, "-Q: unknown operator"},
	{`test -QQ x`, 2, "argument expected"},
	{`test \( x`, 2, "argument expected"},

	// A form that ran out, and the one word left over that is a string
	// rather than a form that ran out.
	{`test -n x -a -n`, 2, "argument expected"},
	{`test -n x -a -Q`, 0, ""},
	{`test -n x -a junk`, 0, ""},
	{`test -n x -a junk junk2`, 2, "argument expected"},

	// Groups: the reading inside one counts the words to the end of the
	// whole list, which is why a word after the group is blamed on the `)`.
	{`test \( x \) y`, 2, "): unknown operator"},
	{`test \( \( x \) \) y`, 2, "): unknown operator"},
	{`test \( x y \)`, 2, "argument expected"},
	{`test \( -Q x \)`, 2, "-Q: unknown operator"},
	{`test \( x = x z \)`, 2, "incorrect syntax"},
	{`test -n x -a \( y = y \) junk`, 2, "incorrect syntax"},

	// `!` has no shortcut of its own: the grammar reads it, so this is
	// `(not x) and ""` and not the negation of `x -a ""`.
	{`test ! x -a ""`, 1, ""},
	{`test ! -Q x y`, 2, "x: unknown operator"},
}

func TestOneExpressionIsReadOffTheFrontOfTheOperands(t *testing.T) {
	for _, tc := range frontRows {
		t.Run(tc.src, func(t *testing.T) {
			out, st := run(t, tc.src, frontReader)
			if st != tc.want {
				t.Errorf("%s = %d, want %d (said %q)", tc.src, st, tc.want, out)
			}
			if tc.text == "" {
				if strings.TrimSpace(out) != "" {
					t.Errorf("%s said %q, want nothing", tc.src, out)
				}
				return
			}
			if !strings.Contains(out, tc.text) {
				t.Errorf("%s said %q, want it to contain %q", tc.src, out, tc.text)
			}
		})
	}
}

// TestTheWholeListIsReadWithTheAxisTheOtherWay is the control, and it is what
// makes the table above a measurement rather than a description of the code:
// with the axis answered the other way the reader is the one every other
// column uses, and most of these rows move.
//
// Written as a count rather than row by row, because the point is not what
// the other reader says — its own tests cover that — but that the axis is
// what decides. A change to this reader that left the other one alone would
// take this number to zero.
func TestTheWholeListIsReadWithTheAxisTheOtherWay(t *testing.T) {
	moved := 0
	for _, tc := range frontRows {
		out, st := run(t, tc.src, wholeList)
		if st != tc.want || (tc.text != "" && !strings.Contains(out, tc.text)) {
			moved++
		}
	}
	// Measured against the whole-list reader on 2026-09-16. A floor rather
	// than the figure itself — 39 of the 55 moved when it was taken. A row the
	// two readers happen to agree on is still worth keeping in the table
	// above, and one more of them agreeing tomorrow is not a regression.
	if moved < 35 {
		t.Errorf("%d of %d rows moved with the axis, want at least 35 — the table is not measuring the axis", moved, len(frontRows))
	}
}
