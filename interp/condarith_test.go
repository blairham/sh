// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The operands of the word-spelled comparisons are expressions, not literals.
//
// The pair of rows is what makes this a measurement rather than a coincidence:
// asserting only `n -eq 5` would pass for a shell that read every non-number
// as zero and compared nothing, and asserting only `n -eq 0` would pass for
// one that read the name and got it wrong. Together they say the name was
// looked up and its value is what was compared.
func TestAComparisonOperandIsAnExpression(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"a bare name is its value", `n=5; [[ n -eq 5 ]]; echo st=$?`, "st=0\n"},
		{"and is not read as zero", `n=5; [[ n -eq 0 ]]; echo st=$?`, "st=1\n"},
		{"on the right as well", `n=5; [[ 5 -eq n ]]; echo st=$?`, "st=0\n"},
		{"an expression, not a number", `[[ 1+1 -eq 2 ]]; echo st=$?`, "st=0\n"},
		{"a name inside one", `k=3; [[ k*2 -eq 6 ]]; echo st=$?`, "st=0\n"},
		{"an unset name is zero", `[[ zz -eq 0 ]]; echo st=$?`, "st=0\n"},
		{"an empty value is zero", `e=''; [[ e -eq 0 ]]; echo st=$?`, "st=0\n"},
		{"surrounding space is not an operand", `sp=' 4 '; [[ sp -eq 4 ]]; echo st=$?`, "st=0\n"},
		{"quoting does not make it a literal", `n=5; [[ "n" -eq 5 ]]; echo st=$?`, "st=0\n"},
		{"a value that is an expression", `q='2+3'; [[ $q -eq 5 ]]; echo st=$?`, "st=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, nil)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The word is expanded once, and what reaches the arithmetic is text.
//
// The operand is written `$q`, not `q`, and the difference is the whole test.
// A bare name puts no `$` in the operand text at all — the `$x` only appears
// later, when the arithmetic looks the name up — so a second expansion placed
// here would have nothing to act on and the run would pass either way. `$q`
// expands to the two characters `$x` *before* the arithmetic sees them, which
// is the only arrangement that can tell one expansion from two.
//
// Measured: no shell in the panel answers 7. All three blame `$x` as an
// operand the arithmetic cannot use, so the comparison fails rather than
// succeeding — and the failing direction is the point, because "wrong" and
// "right" are both quiet here.
func TestAComparisonOperandIsNotExpandedTwice(t *testing.T) {
	sem := PosixSemantics()
	out, _ := run(t, `x=7; q='$x'; [[ $q -eq 7 ]]; echo st=$?`, func(r *Runner) { r.Semantics = &sem })
	if strings.Contains(out, "st=0") {
		t.Errorf("got %q: the operand was expanded a second time", out)
	}
}

// ConditionArithmeticErrorIsFatal decides what an unreadable operand does to
// the rest of the input, and nothing else: the complaint is written and the
// status is 1 under either answer.
func TestAnUnreadableComparisonOperandStopsTheInputOnlyWhenItIsFatal(t *testing.T) {
	const src = `echo one; [[ 1+ -eq 0 ]]; echo two st=$?`

	sem := PosixSemantics()
	sem.ConditionArithmeticErrorIsFatal = No
	out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
	if !strings.Contains(out, "one") || !strings.Contains(out, "two st=1") {
		t.Errorf("not fatal: got %q, want the input to carry on with the condition false", out)
	}

	fatal := PosixSemantics()
	fatal.ConditionArithmeticErrorIsFatal = Yes
	out, st := run(t, src, func(r *Runner) { r.Semantics = &fatal })
	if !strings.Contains(out, "one") {
		t.Errorf("fatal: got %q, want the input before the condition to have run", out)
	}
	if strings.Contains(out, "two") {
		t.Errorf("fatal: got %q, want nothing after the condition to have run", out)
	}
	if st != 1 {
		t.Errorf("fatal: status %d, want 1", st)
	}
}
