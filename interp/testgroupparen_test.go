// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a refused `test` expression is *called* when the reading gave up
// inside a group it had not closed — see
// Semantics.TestFailureInsideAnUnclosedGroupIsTheParen. Named for the axis
// rather than for the column that answers it yes; the preset's pick is
// asserted in dialect/ash.
//
// The controls are the half that makes this an axis rather than "any
// expression with a parenthesis in it": once a group has closed, the leftover
// word is named exactly as it is under the other answer.
func TestAFailureInsideAnUnclosedGroupCanBeTheParen(t *testing.T) {
	run := func(src string, on Answer) string {
		t.Helper()
		out, st := optRun(t, func(s *Semantics) {
			s.TestFailureInsideAnUnclosedGroupIsTheParen = on
		}, Diagnostics{
			TestClosingParenExpected: "PAREN",
			// The leftover-word sentence, so the controls can say which
			// word was named rather than only that something was refused.
			TestTooManyArguments: "LEFTOVER %[1]s",
			// Named the way the column that answers the axis yes names it,
			// so a control asserts on the word rather than on whichever
			// word the substrate's own reading happens to blame.
			TestNamesTheWordTheParseStoppedAt: true,
			TestUnaryExpected:                 "WORD %[1]s",
			TestOperandExpected:               "OPERAND",
			TestBinaryExpected:                "WORD %[1]s",
		}, src)
		if st == 0 {
			t.Fatalf("%s: status 0, want a refusal", src)
		}
		return strings.TrimSpace(out)
	}
	for _, c := range []struct{ src, off string }{
		// Every one of these gives up with the group still open, so the
		// answer replaces whatever the reading would have said.
		{`[ \( x y \) ]`, "WORD y"},
		{`[ \( -Q x \) ]`, "WORD x"},
		{`[ \( \) ]`, "WORD )"},
		{`[ \( \( x \) ]`, "WORD x"},
	} {
		if got := run(c.src, No); !strings.Contains(got, c.off) {
			t.Errorf("%s at No: said %q, want %q", c.src, got, c.off)
		}
		if got := run(c.src, Yes); !strings.Contains(got, "PAREN") {
			t.Errorf("%s at Yes: said %q, want PAREN", c.src, got)
		}
	}
	// A group whose **own** closing parenthesis is what the reading ran out
	// of is not this axis at all: the sentence is the same one, and it is
	// written under either answer wherever the dialect has it. Three rows
	// that used to sit in the table above and did not belong there — the
	// reading gave up wanting a `)` rather than inside the group, so a
	// dialect with a sentence for the missing parenthesis writes it either
	// way. Measured 2026-09-22: dash, which answers this axis No and has the
	// sentence, writes `closing paren expected` for all three.
	for _, src := range []string{
		`[ \( -n x y \) ]`,
		`[ \( x y z \) ]`,
		`[ x -a \( y z \) ]`,
	} {
		for _, on := range []Answer{No, Yes} {
			if got := run(src, on); !strings.Contains(got, "PAREN") {
				t.Errorf("%s at %v: said %q, want PAREN", src, on, got)
			}
		}
	}
	// And the controls: a group that closed leaves the leftover word named
	// under either answer, and an expression with no `(` in it is untouched.
	for _, c := range []struct{ src, want string }{
		{`[ \( x \) junk ]`, "LEFTOVER junk"},
		{`[ \( x \) junk more ]`, "LEFTOVER junk"},
		{`[ x \) ]`, "WORD )"},
	} {
		for _, on := range []Answer{No, Yes} {
			if got := run(c.src, on); !strings.Contains(got, c.want) {
				t.Errorf("%s at %v: said %q, want %q", c.src, on, got, c.want)
			}
		}
	}
}
