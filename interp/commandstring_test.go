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

// One dialect answers a failed expansion with a different status depending on
// how the shell was started, and on nothing else — the same two lines given
// with `-c` and read from a file exit differently.
//
// Both doors to it: an unset parameter under `set -u`, and one `${x?}` was
// asked about.
func TestAFailedExpansionMayAnswerByHowTheShellStarted(t *testing.T) {
	for _, src := range []string{
		"set -u\nunset V\necho \"$V\"\n",
		"unset V\necho \"${V?}\"\n",
	} {
		for _, c := range []struct {
			name          string
			commandString bool
			answer        int
			want          int
		}{
			{"from a file, the ordinary fatal status", false, 127, 1},
			{"from a command string, this dialect's own", true, 127, 127},
			{"and with no answer, the ordinary one either way", true, 0, 1},
			{"which is what a file gets too", false, 0, 1},
		} {
			t.Run(c.name, func(t *testing.T) {
				if got := cmdStringStatus(t, src, c.commandString, c.answer); got != c.want {
					t.Errorf("%q: status = %d, want %d", src, got, c.want)
				}
			})
		}
	}
}

// It is the expansion that answers this way and not every way the shell
// stops. Measured over eight other ways bash stops, none of which cares how
// it was invoked — so a shell that applied this everywhere would be wrong
// about all of them.
func TestOnlyAFailedExpansionAnswersByHowTheShellStarted(t *testing.T) {
	// A command that does not exist stops nothing and reports its own
	// status, whatever the invocation was.
	for _, cs := range []bool{true, false} {
		if got := cmdStringStatus(t, "nosuchcmd\n", cs, 127); got == 0 {
			t.Errorf("commandString=%v: status = %d, want a failure", cs, got)
		}
		if got := cmdStringStatus(t, "exit 3\n", cs, 127); got != 3 {
			t.Errorf("commandString=%v: status = %d, want 3", cs, got)
		}
	}
}

func cmdStringStatus(t *testing.T, src string, commandString bool, answer int) int {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.FatalErrorStatusIsOne = Yes
	dg := Diagnostics{UnsetParameterStatusFromCommandString: answer}
	r := &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh", CommandString: commandString,
		Stdout: &buf, Stderr: &buf,
	}
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// A readonly reassignment is fatal in three of the panel either way, and in
// the fourth only when the program came from an argument.
//
// Only for an assignment standing as a command of its own: the dialect that
// splits is not fatal for `export x=2` or `x=2 cmd` by either route, so
// those keep the ordinary answer.
func TestAReadonlyReassignmentMayAnswerByHowTheShellStarted(t *testing.T) {
	for _, c := range []struct {
		name              string
		base, fromCommand Answer
		commandString     bool
		carried           bool
	}{
		{"fatal either way, from a file", Yes, Yes, false, false},
		{"fatal either way, from an argument", Yes, Yes, true, false},
		{"and one that is not, from a file", No, Yes, false, true},
		{"but is from an argument", No, Yes, true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			var buf strings.Builder
			sem := PosixSemantics()
			sem.ReadonlyReassignmentFatal = c.base
			sem.ReadonlyReassignmentFatalFromCommandString = c.fromCommand
			sem.FatalErrorStatusIsOne = Yes
			r := &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
				CommandString: c.commandString, Stdout: &buf, Stderr: &buf,
			}
			f, err := syntax.Parse("readonly x=1\nx=2\necho after\n", syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if carried := strings.Contains(buf.String(), "after"); carried != c.carried {
				t.Errorf("said %q; carried on = %v, want %v", buf.String(), carried, c.carried)
			}
		})
	}
}

// The command-string answer is for an assignment standing alone. Setting a
// name any other way keeps the ordinary one, which is what makes the two
// questions two rather than one.
func TestOnlyAnAssignmentStandingAloneAsksTheOtherQuestion(t *testing.T) {
	var buf strings.Builder
	sem := PosixSemantics()
	sem.ReadonlyReassignmentFatal = No
	sem.ReadonlyReassignmentFatalFromCommandString = Yes
	sem.FatalErrorStatusIsOne = Yes
	r := &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		CommandString: true, Stdout: &buf, Stderr: &buf,
	}
	// `export x=2` is an assignment, but not one standing as a command of
	// its own — and the dialect that splits is not fatal for it by either
	// route.
	f, err := syntax.Parse("readonly x=1\nexport x=2\necho after\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "after") {
		t.Errorf("said %q, want the ordinary answer for a name set some other way", buf.String())
	}
}
