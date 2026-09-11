// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// An assignment whose right-hand side could not be expanded is a failed
// expansion like any other: it reports a nonzero status and the rest of the
// line does not run.
//
// It reported the failure out loud and then left 0. `x=$(( } )) || handle`
// never fired, `set -e` never tripped, and a script went on as though the
// value had been computed. The same expression in a command position was
// right all along, which is what says the bug was the assignment path and not
// the arithmetic: the check a command's arguments go through runs *before*
// the assignments do, so nothing had failed yet when it looked (#1191).
func TestAnAssignmentWhoseExpansionFailed(t *testing.T) {
	for _, c := range []struct {
		name     string
		abandons Answer
		one      Answer
		want     []string
		gone     string
		status   int
	}{
		{
			// The answer that gives up the statement and runs the next one,
			// which is what makes the status readable on the line after.
			"the line is given up and the next one runs",
			Yes, Yes,
			[]string{"bad math", "after st=1"},
			"",
			0,
		},
		{
			// And the answer that ends the script, at the status a fatal
			// error carries there. Nothing after it runs, so the status this
			// reads is the one the script exited with.
			"or the script ends, at the dialect's fatal status",
			No, No,
			[]string{"bad math"},
			"after",
			2,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := assignFailRun(t, "x=$(( } ))\necho \"after st=$?\"\n", c.abandons, c.one)
			for _, w := range c.want {
				if !strings.Contains(out, w) {
					t.Errorf("said %q, want %q in it", out, w)
				}
			}
			if c.gone != "" && strings.Contains(out, c.gone) {
				t.Errorf("said %q, want %q not in it", out, c.gone)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
		})
	}
}

// And the rest of the *line* goes with it whichever answer is given, which is
// the half both columns of the panel agree on: `x=$(( } )); echo after`
// prints no `after` anywhere.
func TestTheRestOfTheLineGoesWithAFailedAssignment(t *testing.T) {
	for _, abandons := range []Answer{Yes, No} {
		out, _ := assignFailRun(t, "x=$(( } )); echo after\n", abandons, Yes)
		if strings.Contains(out, "after") {
			t.Errorf("abandons=%v: said %q, want the rest of the line given up", abandons, out)
		}
	}
}

// An assignment that succeeds still reports success, which is what says the
// new door is only opened by a failure. The neighboring rules are asserted
// with it, because all three are decided in the same block: `$?` on the right
// reads the command before the assignment, a substitution reports through it,
// and a plain assignment reports 0.
func TestAnAssignmentThatWorkedStillReportsSuccess(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a plain assignment", "false\nx=1\necho st=$?\n", "st=0"},
		{"the previous status is readable", "false\nE=$?\necho E=$E\n", "E=1"},
		{"a substitution reports through it", "true\nx=$(false)\necho st=$?\n", "st=1"},
		{"and arithmetic that works does not", "x=$(( 1 + 1 ))\necho x=$x st=$?\n", "x=2 st=0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := assignFailRun(t, c.src, Yes, Yes)
			if !strings.Contains(out, c.want) {
				t.Errorf("said %q, want %q in it", out, c.want)
			}
		})
	}
}

func assignFailRun(t *testing.T, src string, abandons, one Answer) (string, int) {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.FailedExpansionAbandonsTheLine = abandons
	sem.FatalErrorStatusIsOne = one
	dg := Diagnostics{ArithOperandExpected: "bad math"}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "sh"})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return buf.String(), st
}
