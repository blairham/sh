// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// When a command's assignment prefix is worked through against when a
// declaration utility's operand is expanded —
// Semantics.PrefixExpandedBeforeADeclarationsOperand. Two substitutions
// writing to standard error are the instrument: the order of the two lines is
// the order they were expanded in. Tests name the axis and never a shell.

func prefixOperandOrderRun(t *testing.T, src string, prefixFirst Answer) string {
	t.Helper()
	sem := testSemantics()
	sem.PrefixExpandedBeforeADeclarationsOperand = prefixFirst
	_, errs := builtinPrefixRun(t, src, sem)
	return errs
}

// order reads the two side effects back as the word that came first.
func prefixOperandFirst(t *testing.T, errs string) string {
	t.Helper()
	pre, op := strings.Index(errs, "PRE"), strings.Index(errs, "OP")
	if pre < 0 || op < 0 {
		t.Fatalf("stderr = %q, want both side effects", errs)
	}
	if pre < op {
		return "PRE"
	}
	return "OP"
}

const prefixOperandProbe = "PRE=$(echo PRE >&2) export s=$(echo OP >&2)\n"

func TestAPrefixCanBeExpandedBeforeADeclarationsOperand(t *testing.T) {
	t.Parallel()
	if got := prefixOperandFirst(t, prefixOperandOrderRun(t, prefixOperandProbe, Yes)); got != "PRE" {
		t.Errorf("first side effect = %s, want the prefix's", got)
	}
}

func TestAPrefixCanBeExpandedAfterADeclarationsOperand(t *testing.T) {
	t.Parallel()
	if got := prefixOperandFirst(t, prefixOperandOrderRun(t, prefixOperandProbe, No)); got != "OP" {
		t.Errorf("first side effect = %s, want the operand's", got)
	}
}

// The control that says the axis is about a **declaration's** operand and not
// about words in general: an ordinary command's argument is expanded before
// its prefix on both sides of the axis, which is what every column does.
func TestAnOrdinaryArgumentIsExpandedBeforeThePrefixOnBothSides(t *testing.T) {
	t.Parallel()
	const src = "PRE=$(echo PRE >&2) : s=$(echo OP >&2)\n"
	for _, answer := range []Answer{Yes, No} {
		if got := prefixOperandFirst(t, prefixOperandOrderRun(t, src, answer)); got != "OP" {
			t.Errorf("%v: first side effect = %s, want the argument's", answer, got)
		}
	}
}

// And the other body the operand's parentheses can hold moves with it: an
// array literal answers the same way, so the axis is about the operand rather
// than about which of the two spellings was written.
func TestAnArrayLiteralOperandFollowsTheSameOrder(t *testing.T) {
	t.Parallel()
	sem := testSemantics()
	sem.PrefixExpandedBeforeADeclarationsOperand = Yes
	_, errs := builtinPrefixRun(t, "PRE=$(echo PRE >&2) export a=($(echo OP >&2))\n", sem)
	if got := prefixOperandFirst(t, errs); got != "PRE" {
		t.Errorf("first side effect = %s, want the prefix's", got)
	}
	sem.PrefixExpandedBeforeADeclarationsOperand = No
	_, errs = builtinPrefixRun(t, "PRE=$(echo PRE >&2) export a=($(echo OP >&2))\n", sem)
	if got := prefixOperandFirst(t, errs); got != "OP" {
		t.Errorf("first side effect = %s, want the operand's", got)
	}
}

// A prefix of two entries stays in written order on either side of the split,
// which is what says the answer moves the whole walk rather than one value.
func TestTheWholePrefixMovesInWrittenOrder(t *testing.T) {
	t.Parallel()
	const src = "P1=$(echo P1 >&2) P2=$(echo P2 >&2) export s=$(echo OP >&2)\n"
	for _, row := range []struct {
		prefixFirst Answer
		want        []string
	}{
		{Yes, []string{"P1", "P2", "OP"}},
		{No, []string{"OP", "P1", "P2"}},
	} {
		errs := prefixOperandOrderRun(t, src, row.prefixFirst)
		if got := strings.Fields(errs); !prefixOperandFieldsEqual(got, row.want) {
			t.Errorf("%v: stderr = %q, want %v", row.prefixFirst, errs, row.want)
		}
	}
}

func prefixOperandFieldsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// And a dialect that has not chosen refuses by name rather than picking one.
func TestThePrefixOperandOrderIsRefusedWhereTheDialectHasNotChosen(t *testing.T) {
	t.Parallel()
	errs := prefixOperandOrderRun(t, prefixOperandProbe, Unspecified)
	if !strings.Contains(errs, "assignment prefix against a declaration utility's operand") {
		t.Errorf("stderr = %q, want the axis named", errs)
	}
	// And the command with no operand at all asks nothing, so an unanswered
	// axis does not reach `x=1 cmd`.
	sem := testSemantics()
	sem.PrefixExpandedBeforeADeclarationsOperand = Unspecified
	out, errs := builtinPrefixRun(t, "PRE=1 echo ran\n", sem)
	if out != "ran\n" || strings.Contains(errs, "declaration utility's operand") {
		t.Errorf("out = %q, stderr = %q; want the command to run unasked", out, errs)
	}
}
