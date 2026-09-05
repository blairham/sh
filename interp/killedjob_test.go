// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The notice that writes a killed command back out names the *job*, and a
// subshell is one where a group, a function body and an `if` are not.
//
// SIGUSR1 for the same reason killednotice_test.go uses it: its default
// action ends the process without dumping core.

// killedJobRun is killedRun with the answers this file needs to vary.
func killedJobRun(t *testing.T, src string, set func(*Semantics)) string {
	t.Helper()
	dg := Diagnostics{
		Location:           LocationTightLine,
		SignalDescriptions: map[syscall.Signal]string{syscall.SIGUSR1: "Boom"},
	}
	sem := PosixSemantics()
	sem.ReportsACommandKilledBySignal = Yes
	if set != nil {
		set(&sem)
	}
	var errs strings.Builder
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh",
		Stdout: &strings.Builder{}, Stderr: &errs,
	})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return errs.String()
}

const killedInner = `/bin/sh -c 'kill -USR1 $$'`

// TestASubshellIsTheJobThatIsNamed: the notice writes `( cmd )` back out
// rather than the command inside it, because the subshell is what a shell
// forks and reaps. Measured against the dialect that writes the command at
// all: a group, a function body and an `if` all name the command, and none
// of them forks.
func TestASubshellIsTheJobThatIsNamed(t *testing.T) {
	for _, tc := range []struct {
		name, src, want, notWant string
	}{
		{
			"a subshell", "true\n( " + killedInner + " )\n",
			"( " + killedInner + " )", "",
		},
		{
			"a subshell of two commands", "true\n( true; " + killedInner + " )\n",
			"( true; " + killedInner + " )", "",
		},
		{
			"a group", "true\n{ " + killedInner + "; }\n",
			killedInner, "{",
		},
		{
			"a function body", "f() { " + killedInner + "; }\nf\n",
			killedInner, "{",
		},
		{
			"an if", "if true; then " + killedInner + "; fi\n",
			killedInner, "if",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := killedJobRun(t, tc.src, nil)
			if !strings.Contains(got, tc.want) {
				t.Errorf("notice = %q, want it to name %q", got, tc.want)
			}
			if tc.notWant != "" && strings.Contains(got, tc.notWant) {
				t.Errorf("notice = %q, want no %q in it", got, tc.notWant)
			}
		})
	}
}

// TestACommandSubstitutionIsAskedSeparately is why the substitution is a
// second field rather than the subshell one in another spelling: one dialect
// remarks on a killed command inside `( … )` and says nothing about the same
// command inside `$(…)`.
func TestACommandSubstitutionIsAskedSeparately(t *testing.T) {
	src := "true\nx=$(" + killedInner + ")\n"

	silent := killedJobRun(t, src, func(s *Semantics) {
		s.ReportsAKilledCommandInACommandSubstitution = No
	})
	if silent != "" {
		t.Errorf("said %q, want nothing", silent)
	}

	spoken := killedJobRun(t, src, func(s *Semantics) {
		s.ReportsAKilledCommandInACommandSubstitution = Yes
	})
	if !strings.Contains(spoken, "Boom") {
		t.Errorf("said %q, want the notice", spoken)
	}

	// And the answer is about the substitution alone: a subshell written out
	// is still remarked on by the dialect that says nothing inside `$(…)`.
	sub := killedJobRun(t, "true\n( "+killedInner+" )\n", func(s *Semantics) {
		s.ReportsAKilledCommandInACommandSubstitution = No
	})
	if !strings.Contains(sub, "Boom") {
		t.Errorf("a subshell said %q, want the notice", sub)
	}
}

// TestAPipelineElementNamesItself, because each element is its own job
// component: without that the copy running one would inherit whatever the
// shell last ran, and name that instead.
func TestAPipelineElementNamesItself(t *testing.T) {
	got := killedJobRun(t, "true\necho hi | "+killedInner+"\n", func(s *Semantics) {
		s.ReportsAnyKilledPipelineElement = Yes
	})
	if !strings.Contains(got, killedInner) {
		t.Errorf("notice = %q, want it to name the element", got)
	}
	if strings.Contains(got, "Boom  true") {
		t.Errorf("notice = %q, named the command before the pipeline", got)
	}
}
