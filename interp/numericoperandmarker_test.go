// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// The end-of-options marker in front of a numeric operand, which `shift` took
// from its first day and `break`, `continue`, `return` and `exit` read as the
// operand itself — so `break -- 1` ended the script at 2 where five of the
// seven columns end the loop at 0.
//
// One reader for the four, so the rows below are the same question asked
// through each of them: a second reader beside the first is what drifts the
// next time the marker is measured, and this defect is what one looked like.
func TestADoubleDashEndsTheOptionsOfANumericOperand(t *testing.T) {
	base := func(marker Answer) Semantics {
		s := CoreSemantics()
		s.NumericOperandDoubleDashEndsOptions = marker
		s.LoopControlOutsideALoopIsFatal = No
		s.ReturnOutsideAFunctionIsRefused = Yes
		s.BadOptionToSpecialBuiltinFatal = No
		return s
	}
	dg := Diagnostics{
		LoopControlCount:           "%[1]s: %[2]s: unreadable count",
		LoopControlCountOutOfRange: "%[1]s: %[2]s: out of range",
		NumericArgument:            "%[1]s: %[2]s: not a number",
	}

	for _, tc := range []struct {
		name   string
		marker Answer
		src    string
		want   string
		why    string
	}{
		{
			"a count behind the marker", Yes,
			`for i in 1 2; do break -- 1; echo body; done; echo "B=$?"`,
			"B=0\n",
			"the marker is taken and the 1 is the count, so the loop ends once and nothing is said",
		},
		{
			"the other loop builtin reads it too", Yes,
			`for i in 1 2; do continue -- 1; echo body; done; echo "B=$?"`,
			"B=0\n",
			"one reader for both, so a fix written into `break` alone would leave this row refusing",
		},
		{
			"the marker alone still counts one", Yes,
			`for i in 1 2; do break --; echo body; done; echo "B=$?"`,
			"B=0\n",
			"the marker is consumed rather than read, so the count falls back to its default",
		},
		{
			"past the marker a dash word is the count", Yes,
			`for i in 1 2; do break -- -1; echo body; done; echo "B=$?"`,
			"sh: break: -1: out of range\nB=1\n",
			"the complaint names the number behind the marker, not the marker, and the loop ends at 1 — bash 5.3.20 answers `B=1` here",
		},
		{
			"only the first marker is one", Yes,
			`for i in 1 2; do break -- --; echo body; done; echo "B=$?"`,
			"sh: break: --: unreadable count\n",
			"the second `--` is the count, and it is not one — the script ends where an unreadable count ends it",
		},
		{
			"a return status behind the marker", Yes,
			`f() { return -- 3; }; f; echo "D=$?"`,
			"D=3\n",
			"the same reader through `return`, whose operand is a status rather than a count",
		},
		{
			"an exit status behind the marker", Yes,
			`echo A; exit -- 3`,
			"A\n",
			"and through `exit`, where the operand is the shell's own status",
		},
		{
			"without the marker the word is the operand", No,
			`for i in 1 2; do break -- 1; echo body; done; echo "B=$?"`,
			"sh: break: --: unreadable count\n",
			"the two columns with no marker here read `--` as the count and refuse it, which is dash's and ash's answer",
		},
		{
			"unanswered, and only for a `--`", Unspecified,
			`for i in 1 2; do break 1; done; echo "B=$?"`,
			"B=0\n",
			"a dialect is never asked about an ordinary count",
		},
		{
			"unanswered with a `--` is refused", Unspecified,
			`for i in 1 2; do break -- 1; done; echo "B=$?"`,
			"sh: `--` read as the end of a numeric operand's options: the shells disagree here and no dialect was chosen\n" +
				"sh: `--` read as the end of a numeric operand's options: the shells disagree here and no dialect was chosen\nB=2\n",
			"the strict core reports the axis rather than guessing which word is the count — once per pass, because a `break` that was refused did not leave the loop",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := base(tc.marker)
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
			if out != tc.want {
				t.Errorf("got %q, want %q — %s", out, tc.want, tc.why)
			}
		})
	}
}

// The exit status of a script that ended through a marker it took, which the
// output above cannot show: `exit -- 3` leaves 3 and not the 2 a refused
// operand leaves.
func TestAnExitStatusBehindTheMarkerIsTheShellsOwn(t *testing.T) {
	sem := CoreSemantics()
	sem.NumericOperandDoubleDashEndsOptions = Yes
	_, st := run(t, `echo A; exit -- 3`, func(r *Runner) { r.Semantics = &sem })
	if st != 3 {
		t.Errorf("status %d, want 3 — the operand behind the marker is the status", st)
	}
}
