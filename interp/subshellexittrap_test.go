// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A subshell's own EXIT trap, and which give-ups take it away. The axis names
// the *kind* of give-up and never a shell.

// subshellTrapRun answers the axes these rows need and leaves the rest alone.
func subshellTrapRun(t *testing.T, src string, policy SubshellExitTrapPolicy) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		sem := permissive()
		sem.SubshellExitTrapAfterAGiveUp = policy
		sem.ReadonlyReassignmentByDeclarationFatal = Yes
		sem.ReadonlyReassignmentBySpecialBuiltinFatal = Yes
		sem.ArithDivisionByZeroYieldsAValue = No
		sem.BadSetOptionLetterFatal = Yes
		r.Semantics = &sem
		r.Diagnostics = &Diagnostics{}
	})
}

// The three values, over the two kinds of give-up that tell them apart and the
// two kinds that tell none of them apart.
//
// Each row is run plain and with `set -e` in front, because the option is the
// discriminator at the *top level* and deciding nothing here is half of what
// this axis says.
func TestWhichGiveUpsTakeASubshellsExitTrapAway(t *testing.T) {
	const handler = `( trap 'printf TRAP_RAN' EXIT` + "\n"
	for _, c := range []struct {
		name string
		body string
		// whether the handler runs, under each of the three values in the
		// order they are declared.
		always, reported, usage bool
	}{
		// An error the shell reported and gave the subshell up over.
		{"a readonly reassignment", "readonly r=1\nreadonly r=2", true, false, false},
		{"an unset parameter under nounset", "set -u\nprintf '[%s]' \"$nosuch\"", true, false, false},
		{"a division by zero", "printf '[%s]' \"$((1/0))\"", true, false, false},
		// The operator that is a request to stop at the top level and goes
		// with the errors here. This row is the whole reason the axis is not
		// the top-level field with a wider reach.
		{"a parameter asked about", "printf '[%s]' \"${nosuch?word}\"", true, false, false},
		// A special builtin complaining about how it was called, which is
		// where the two skipping columns part company.
		{"a refused set option", "set -Z", true, true, false},
		// Stops the script asked for: nothing takes the handler from these.
		{"an ordinary failure", "false", true, true, true},
		{"an explicit exit", "exit 3", true, true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, p := range []struct {
				policy SubshellExitTrapPolicy
				want   bool
			}{
				{SubshellExitTrapAlwaysRuns, c.always},
				{SubshellExitTrapSkippedByAReportedError, c.reported},
				{SubshellExitTrapSkippedByABuiltinsUsageToo, c.usage},
			} {
				for _, errexit := range []string{"", "set -e\n"} {
					src := handler + errexit + c.body + "\nprintf NOT-REACHED )\n"
					out, _ := subshellTrapRun(t, src, p.policy)
					if ran := strings.Contains(out, "TRAP_RAN"); ran != p.want {
						t.Errorf("%v errexit=%q: trap ran=%v, want %v — got %q",
							p.policy, errexit, ran, p.want, out)
					}
				}
			}
		})
	}
}

// A vector that has chosen nothing runs the handler, as the top-level field
// beside it does. Read without asking: an unanswered axis here would refuse
// every subshell that ends over an error, which is not a question a cleanup
// handler can be left waiting on.
func TestAnUnchosenSubshellExitTrapPolicyRunsTheHandler(t *testing.T) {
	out, _ := subshellTrapRun(t,
		"( trap 'printf TRAP_RAN' EXIT\nreadonly r=1\nreadonly r=2 )\n",
		SubshellExitTrapUnspecified)
	if !strings.Contains(out, "TRAP_RAN") {
		t.Errorf("got %q, want the handler to have run", out)
	}
	if strings.Contains(out, "unanswered") {
		t.Errorf("got %q, want no refusal", out)
	}
}

// The boundary is the subshell and not the statement, and it is the *shell's
// own* trap rather than an inherited one: the top level of the same script
// keeps its handler for the same error under the same value.
func TestTheSubshellAnswerIsAskedAtTheBoundaryAndNotAtTheTopLevel(t *testing.T) {
	out, _ := subshellTrapRun(t,
		"trap 'printf TOP_RAN' EXIT\n( readonly r=1\nreadonly r=2 )\nprintf ' after'\n",
		SubshellExitTrapSkippedByABuiltinsUsageToo)
	if !strings.Contains(out, "TOP_RAN") {
		t.Errorf("got %q, want the script's own handler to have run", out)
	}
	if !strings.Contains(out, "after") {
		t.Errorf("got %q, want the script to have carried on past the subshell", out)
	}
	// And the same error at the top level, which is the row that says the
	// gate is the boundary: the script ends over it and the handler still
	// runs, because this axis is not asked there at all.
	top, _ := subshellTrapRun(t,
		"trap 'printf TOP_RAN' EXIT\nreadonly r=1\nreadonly r=2\nprintf ' after'\n",
		SubshellExitTrapSkippedByABuiltinsUsageToo)
	if !strings.Contains(top, "TOP_RAN") {
		t.Errorf("at the top level: got %q, want the handler to have run", top)
	}
	if strings.Contains(top, "after") {
		t.Errorf("at the top level: got %q, want the script to have ended", top)
	}
}

// Every subshell boundary, not only `( … )`. Measured on the reference over a
// command substitution, a pipeline element, a background job and a subshell
// inside a subshell; all four lose the handler.
func TestEverySubshellBoundaryAsksTheSameQuestion(t *testing.T) {
	const body = "trap 'printf TRAP_RAN' EXIT\nreadonly r=1\nreadonly r=2\n"
	for _, c := range []struct{ name, src string }{
		{"a command substitution", "v=$( " + body + " )\nprintf %s \"$v\"\n"},
		{"a subshell inside a subshell", "( ( " + body + " ) )\n"},
		{"a background job", "( " + body + " ) &\nwait\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := subshellTrapRun(t, c.src, SubshellExitTrapSkippedByABuiltinsUsageToo)
			if strings.Contains(out, "TRAP_RAN") {
				t.Errorf("got %q, want the handler to have been skipped", out)
			}
			ran, _ := subshellTrapRun(t, c.src, SubshellExitTrapAlwaysRuns)
			if !strings.Contains(ran, "TRAP_RAN") {
				t.Errorf("under the other value: got %q, want the handler to have run", ran)
			}
		})
	}
}
