// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `wait -n` and `wait -p var`: the narrowed wait, and the name it writes the
// finished job's process id under.
//
// Both letters belong to one shell in the panel, so both are axes; what is
// asserted here is what the letters *do* where the axis says the shell has
// them, since a measurement can only say that some shell does it.

// The two jobs every row below starts: a short one and a long one, exiting at
// statuses that say which was waited for. The pair is the whole instrument —
// `wait -n` with no operands reports 4 and `wait -n %2` reports 5, and a
// builtin that ignores its operands gives 4 to both.
const twoJobs = "{ sleep 0.1; exit 4; } &\n{ sleep 0.5; exit 5; } &\n"

// waitSemantics has the letters and the axes a row here passes through
// answered, so a failure is about the letter under test. hasP is the one axis
// a row actually varies.
func waitSemantics(hasP Answer) Semantics {
	sem := testSemantics()
	sem.WaitReadsOptions = Yes
	sem.WaitNextJob = WaitNextJobFirstToFinish
	sem.WaitPNamesTheFinishedJob = hasP
	sem.WaitReportsAMissingJob = Yes
	// Answered because a row passes through it and not because it is the
	// subject: `$(…)` around the comparison below is a subshell, and a shell
	// holding a live job when one is made has to know what the subshell sees.
	// The majority answer, which two of the panel's shells give.
	sem.SubshellJobTable = SubshellJobsCleared
	return sem
}

func waitRun(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src,
		func(d *syntax.Dialect) { d.ArraySubscript = true },
		func(r *Runner) {
			sem := waitSemantics(Yes)
			r.Semantics = &sem
		})
}

// The bug: the function took no arguments at all, so `wait -n %2 %3` waited
// for whichever job of *every* job the shell held finished first.
func TestWaitDashNWaitsForTheJobsItWasGiven(t *testing.T) {
	for _, c := range []struct {
		name string
		wait string
		want string
	}{
		{"no operands takes the first to finish", "wait -n", "st=4"},
		{"an operand narrows it to that job", "wait -n %2", "st=5"},
		{"and to the other one", "wait -n %1", "st=4"},
		{"several operands take the first of those", "wait -n %1 %2", "st=4"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := waitRun(t, twoJobs+c.wait+"\necho \"st=$?\"\nwait\n")
			if st != 0 {
				t.Fatalf("status %d: %q", st, out)
			}
			if got := strings.TrimSpace(out); got != c.want {
				t.Errorf("%s gave %q, want %q", c.wait, got, c.want)
			}
		})
	}
}

// `wait -p var` stores the process id of the job the status came from. The
// id is compared rather than printed, because the number is the machine's.
func TestWaitDashPNamesTheJobTheStatusCameFrom(t *testing.T) {
	const named = "echo \"st=$? named=$([ \"$V\" = \"$b1\" ] && echo yes || echo no)\"\n"

	t.Run("the job it waited for", func(t *testing.T) {
		out, st := waitRun(t, "{ sleep 0.1; exit 4; } & b1=$!\nwait -p V %1\n"+named)
		if st != 0 || strings.TrimSpace(out) != "st=4 named=yes" {
			t.Errorf("got %q status %d, want st=4 named=yes", out, st)
		}
	})

	t.Run("the letters may be bundled", func(t *testing.T) {
		for _, form := range []string{"wait -np V %2", "wait -npV %2"} {
			out, st := waitRun(t, twoJobs+"b1=$!\n"+form+"\n"+named)
			if st != 0 || strings.TrimSpace(out) != "st=5 named=yes" {
				t.Errorf("%s gave %q status %d, want st=5 named=yes", form, out, st)
			}
		}
	})

	// A bare `wait` names no single job, and the letter still writes: the
	// variable is empty afterwards rather than holding what it held. That is
	// the half a store written only on success would get wrong.
	t.Run("no job to name empties the variable", func(t *testing.T) {
		out, st := waitRun(t, "V=preset\n{ sleep 0.1; } &\nwait -p V\necho \"st=$? V=[$V]\"\n")
		if st != 0 || strings.TrimSpace(out) != "st=0 V=[]" {
			t.Errorf("got %q status %d, want st=0 V=[]", out, st)
		}
	})

	// The name is stored through the route an assignment takes, so a
	// subscripted one reaches an element and the subscript is arithmetic.
	t.Run("through an array element", func(t *testing.T) {
		out, st := waitRun(t, "{ sleep 0.1; exit 4; } & b1=$!\nwait -p \"A[1+1]\" %1\n"+
			"echo \"st=$? named=$([ \"${A[2]}\" = \"$b1\" ] && echo yes || echo no)\"\n")
		if st != 0 || strings.TrimSpace(out) != "st=4 named=yes" {
			t.Errorf("got %q status %d, want st=4 named=yes", out, st)
		}
	})
}

// A shell whose dialect does not have the letter refuses the word, and says
// so — which is what four of the panel's shells do, each in its own words.
func TestWaitDashPIsRefusedWhereTheDialectLacksIt(t *testing.T) {
	out, _ := runGrammar(t, "V=preset\n{ sleep 0.1; } &\nwait -p V\necho \"st=$? V=[$V]\"\nwait\n",
		nil,
		func(r *Runner) {
			sem := waitSemantics(No)
			r.Semantics = &sem
		})
	if !strings.Contains(out, "V=[preset]") {
		t.Errorf("got %q, want the variable left as it was", out)
	}
	if strings.Contains(out, "st=0") {
		t.Errorf("got %q, want a refusal rather than a wait", out)
	}
}
