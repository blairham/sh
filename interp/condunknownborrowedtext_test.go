// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// condUnknownGrammar lets `[[ -word x ]]` parse with an operator the grammar
// does not know, which is what leaves the refusal to the runner.
func condUnknownGrammar(d *syntax.Dialect) {
	d.ConditionIsResolvedWhenItRuns = true
}

// An unknown condition ends the shell, and `eval` is the boundary that ends
// **itself** instead.
//
// Both halves are asserted together because the fix is one line and either
// half alone passes without it: the lexical rows below were already right
// through stopTheShell, and the borrowed-text rows are what that door cannot
// express. Measured 2026-09-29 on zsh 5.9.2 over script files, with a second
// statement inside the borrowed text and a third after it — the inner one
// never runs in any row.
//
//	[[ -fail x ]] on its own line, then another line     the shell ends
//	the same inside a function, then a line after it     the shell ends
//	the same inside a `while` body, then a line after    the shell ends
//	eval "[[ -fail x ]]; print inner"; print after       after runs, at 2
//	f(){ eval "…"; print fn }; f; print after            fn and after run
func TestAnUnknownConditionIsGivenUpByBorrowedTextAndNotByTheShell(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The rows that must not move. `ENDED` is never reached.
		{
			"at the top level", "[[ -fail badly ]]\necho ENDED\n", "",
		},
		{
			"inside a function",
			"f() { [[ -fail badly ]]; echo INNER; }\nf\necho ENDED\n", "",
		},
		{
			"inside a loop body",
			"for i in 1 2; do [[ -fail badly ]]; echo INNER; done\necho ENDED\n", "",
		},
		// The rows the change is for. The statement after the refusal
		// *inside* the borrowed text still never runs — what survives is the
		// caller.
		{
			"inside eval",
			"eval '[[ -fail badly ]]\necho INNER'\necho after=$?\n", "after=2\n",
		},
		{
			"inside eval inside a function",
			"f() { eval '[[ -fail badly ]]\necho INNER'; echo fn=$?; }\nf\necho ENDED\n",
			"fn=2\nENDED\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := runGrammar(t, tc.src, condUnknownGrammar, func(r *Runner) {
				sem := *r.Semantics
				// The axis that says borrowed text is the boundary at all,
				// which is the measured shell's answer and is what the eval
				// rows are *about* — see Runner.caughtBorrowedError. Without
				// it the lexical rows pass on their own and nothing here
				// would notice the door this change swaps.
				sem.FatalErrorEndsBorrowedTextOnly = Yes
				r.Semantics = &sem
			})
			// The complaint is written in every row and is not what these
			// assert, so it is taken off the front rather than matched.
			out := stripCondComplaint(got)
			if out != tc.want {
				t.Errorf("got %q, want %q (full output %q)", out, tc.want, got)
			}
			if !strings.Contains(got, "unknown condition: -fail") {
				t.Errorf("no complaint in %q", got)
			}
			if strings.Contains(got, "INNER") {
				t.Errorf("the statement after the refusal ran: %q", got)
			}
		})
	}
}

// stripCondComplaint drops the refusal's own line, which every row above
// writes and none of them is about.
func stripCondComplaint(out string) string {
	var keep []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "unknown condition") {
			continue
		}
		keep = append(keep, line)
	}
	return strings.Join(keep, "\n")
}

// The continue-on-error switch rescues a fatal error that carries a status of
// its own, exactly as it rescues one that takes the axis's.
//
// Two errors, because the point is that they go through two doors that had
// drifted: the unknown condition is fatalAtStatus's and `break abc` is the
// only other caller of it. Measured 2026-09-29 on zsh 5.9.2, script files:
// `setopt continueonerror` and then each refusal prints the line after it, at
// 2 and at 1 respectively, and without the switch neither does.
func TestTheContinueOnErrorSwitchRescuesAFatalErrorWithItsOwnStatus(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		rescued   bool
		want      string
	}{
		{"an unknown condition, rescued", "[[ -fail badly ]]\necho after=$?\n", true, "after=2\n"},
		{"an unknown condition, not rescued", "[[ -fail badly ]]\necho after=$?\n", false, ""},
		{"a loop control count, rescued", "while true; do break abc; done\necho after=$?\n", true, "after=2\n"},
		{"a loop control count, not rescued", "while true; do break abc; done\necho after=$?\n", false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := runGrammar(t, tc.src, condUnknownGrammar, func(r *Runner) {
				r.SetContinuesPastAFatalError(tc.rescued)
			})
			ran := strings.Contains(got, "after=")
			if tc.rescued != ran {
				t.Errorf("rescued=%v: the line after the refusal %s, in %q",
					tc.rescued, map[bool]string{true: "ran", false: "did not run"}[ran], got)
			}
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("rescued=%v: got %q, want it to carry %q", tc.rescued, got, tc.want)
			}
		})
	}
}
