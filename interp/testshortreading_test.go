// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The two readings a long `test` expression can take where the grammar runs
// out of words, named by their axes rather than by the column that answers
// them yes — the preset's pick is asserted in dialect/bash.
//
// Both sides of each are run, which is what makes them axes rather than
// rules: the same words answer differently, and the No side is the refusal
// this shell already gave. Every row ends in `echo $?`, so what is asserted
// is the **expression's** own answer rather than the script's.
func testReading(t *testing.T, set func(*Semantics), src string) string {
	t.Helper()
	out, _ := optRun(t, set, Diagnostics{TestOperandExpected: "OPERAND"}, src)
	return out
}

func TestATrailingOperatorIsAWordWhereTheAxisSaysSo(t *testing.T) {
	for _, c := range []struct{ src, yes, no string }{
		// The operator is the last word, so there is nothing for it to be an
		// operator over: a word under one answer, a refusal under the other.
		{`test -n xx -a -f; echo $?`, "0\n", "OPERAND\n2\n"},
		{`test -n xx -o -z; echo $?`, "0\n", "OPERAND\n2\n"},
		// The control: a word behind it and the reading is unchanged under
		// either answer, because the operator has its operand.
		{`test -n xx -a -n yy; echo $?`, "0\n", "0\n"},
	} {
		for _, on := range []Answer{Yes, No} {
			want := c.yes
			if on == No {
				want = c.no
			}
			got := testReading(t, func(s *Semantics) {
				s.TestTrailingUnaryOperatorIsAWord = on
			}, c.src)
			if !strings.HasSuffix(got, want) {
				t.Errorf("%s at %v: output %q, want it to end %q", c.src, on, got, want)
			}
		}
	}
}

// `-R` asks whether a name is a reference, and a shell without the operator
// refuses it by name — which is the answer the axis's other side gives.
//
// A plain name is the operand here rather than a reference: making one needs
// a declaration letter no preset in this package has, and what the axis
// decides is whether the operator **exists**. That the reference itself
// answers true is dialect/bash's row.
func TestANameReferenceTestAsksAboutTheBinding(t *testing.T) {
	const src = "v=1\ntest -R v; echo $?"
	if yes := testReading(t, func(s *Semantics) {
		s.TestHasTheNameReferenceOperator = Yes
	}, src); yes != "1\n" {
		t.Errorf("at yes: output %q, want %q — the operator answering false", yes, "1\n")
	}
	if no := testReading(t, func(s *Semantics) {
		s.TestHasTheNameReferenceOperator = No
	}, src); no == "1\n" {
		t.Errorf("at no: output %q, want the operator refused by name", no)
	}
}

// And a group short enough for the counts to read is read by them.
func TestAShortGroupIsReadByTheCountsWhereTheAxisSaysSo(t *testing.T) {
	for _, c := range []struct{ src, yes, no string }{
		// One word inside: the operator is the word it is spelled with.
		{`test -n x -a \( -f \); echo $?`, "0\n", "OPERAND\n2\n"},
		// Two: the `!` negates that word.
		{`test -n x -a \( ! -f \); echo $?`, "1\n", "OPERAND\n2\n"},
		// Four: the grammar under either answer, in which the operator takes
		// the parenthesis and the group is then never closed.
		{`test \( -n xx -a -n \); echo $?`, "OPERAND\n2\n", "OPERAND\n2\n"},
	} {
		for _, on := range []Answer{Yes, No} {
			want := c.yes
			if on == No {
				want = c.no
			}
			got := testReading(t, func(s *Semantics) {
				s.TestShortGroupIsReadByTheCounts = on
			}, c.src)
			if !strings.HasSuffix(got, want) {
				t.Errorf("%s at %v: output %q, want it to end %q", c.src, on, got, want)
			}
		}
	}
}
