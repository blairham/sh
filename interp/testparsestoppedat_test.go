// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Diagnostics.TestNamesTheWordTheParseStoppedAt and
// Diagnostics.TestTrailingBinaryOperandExpected: one column parses the
// operand list rather than dispatching on its length, so what it names is
// the first word the expression did **not** consume — and a binary operator
// standing in the trailing position is missing its right operand rather than
// being a word in the wrong place.
//
// Asked here by the fields and never by a shell's name; the fields' own
// documentation says where the rows were taken.

// parsesTheWords answers both, with wordings plain enough that an assertion
// is about which word was named rather than about anybody's phrasing.
func parsesTheWords(on bool) func(*Runner) {
	return func(r *Runner) {
		dg := Diagnostics{
			TestUnaryExpected:                 "%[1]s: not an operator",
			TestBinaryExpected:                "%[1]s: not an operator",
			TestOperandExpected:               "argument expected",
			TestTooManyArguments:              "%[1]s: left over",
			TestNamesTheWordTheParseStoppedAt: on,
		}
		if on {
			dg.TestTrailingBinaryOperandExpected = "%[1]s: argument expected"
		}
		r.Diagnostics = &dg
		s := *r.Semantics
		s.TestStringOrder = TestStringOrderBoth
		r.Semantics = &s
	}
}

func TestTheWordNamedIsTheOneTheParseStoppedAt(t *testing.T) {
	for _, tc := range []struct {
		src     string
		parsed  string
		counted string
		why     string
	}{
		// Two words: the expression is the first and the second is left
		// over. The counting reading has the first as the operator instead.
		{`test -Q g.f`, "g.f: not an operator", "-Q: not an operator", "an operator neither shell has"},
		// Three words, and one word further along each time — which is the
		// same rule and not two.
		{`test a b c`, "b: not an operator", "b: not an operator", "the expression is one word"},
		{`test -z a b`, "b: not an operator", "a: not an operator", "`-z a` is two"},
		{`test ! a b`, "b: not an operator", "a: not an operator", "and `! a` is two"},
		// Past the count rules, where the parse says where it stopped: the
		// *first* leftover word rather than the last one taken.
		{`test -z a b c`, "b: left over", "a: left over", "one past `-z a`"},
		{`test a = b = c`, "=: left over", "b: left over", "one past `a = b`, which is the second `=`"},
	} {
		for _, c := range []struct {
			on   bool
			want string
		}{{true, tc.parsed}, {false, tc.counted}} {
			out, st := run(t, tc.src, parsesTheWords(c.on))
			if st != 2 {
				t.Errorf("%s: status %d, want 2", tc.src, st)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("parsing=%v: %s = %q, want it to hold %q — %s",
					c.on, tc.src, out, c.want, tc.why)
			}
		}
	}
}

// A binary operator this shell has, standing in the trailing position, is the
// operator missing its right operand — so the *operator* is named rather than
// the word in front of it. The set is exactly the operators the vector says
// the shell has, which is what makes it a reading of the form and not a list.
func TestATrailingBinaryOperatorIsMissingItsRightOperand(t *testing.T) {
	for _, tc := range []struct {
		src     string
		parsed  string
		counted string
	}{
		{`test a =`, "=: argument expected", "a: not an operator"},
		{`test a !=`, "!=: argument expected", "a: not an operator"},
		{`test a -eq`, "-eq: argument expected", "a: not an operator"},
		{`test a -nt`, "-nt: argument expected", "a: not an operator"},
		{`test a -ge`, "-ge: argument expected", "a: not an operator"},
		// An operator this vector says the shell has, against one it does
		// not: `<` is in the string-order answer above and `==` is not, so
		// the second falls back to the reading for a word in the wrong place
		// even with the sentence set.
		{`test a \<`, "<: argument expected", "a: not an operator"},
		{`test a ==`, "==: not an operator", "a: not an operator"},
		// A connective is not one of them. It is an operator missing its
		// right operand too, and the sentence for that one names nothing.
		{`test a -a`, "argument expected", "a: not an operator"},
		{`test -z a -a`, "argument expected", "a: not an operator"},
	} {
		for _, c := range []struct {
			on   bool
			want string
		}{{true, tc.parsed}, {false, tc.counted}} {
			out, st := run(t, tc.src, parsesTheWords(c.on))
			if st != 2 {
				t.Errorf("%s: status %d, want 2", tc.src, st)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("parsing=%v: %s = %q, want it to hold %q", c.on, tc.src, out, c.want)
			}
		}
	}
}
