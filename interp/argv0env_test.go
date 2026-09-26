// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// One dialect lets a script choose the name a command is started under by
// **exporting** it, and the rule is keyed on that word and not on the shape
// the assignment was written in.
//
// The rows below are the grid measured on the reference, with the last two
// the discriminating pair: hold "there is a variable named ARGV0" fixed and
// vary only whether it is exported, and the answer moves. A rule written
// around the assignment *prefix* — which is what the shape invites — agrees
// with every other row here and is wrong on both of them.
func TestAnExportedArgv0NamesTheCommand(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		// The one row that needs `export` to have its `-n`, which is an
		// axis of its own and not every dialect's.
		unexports bool
	}{
		{
			"a prefix, which exports for the one command",
			`ARGV0=chosen sh -c 'echo "[$0]"'`, "[chosen]", false,
		},
		{
			"exported with no prefix in sight",
			`export ARGV0=chosen; sh -c 'echo "[$0]"'`, "[chosen]", false,
		},
		{
			"assigned first and exported after",
			`ARGV0=chosen; export ARGV0; sh -c 'echo "[$0]"'`, "[chosen]", false,
		},
		{
			// The first half of the pair: a variable of that name, and
			// nothing exports it.
			"assigned and never exported",
			"ARGV0=chosen\nsh -c 'echo \"[$0]\"'", "[sh]", false,
		},
		{
			// The second half: exported and then not, which leaves the
			// variable in place and takes only the attribute away.
			"exported and then not",
			`export ARGV0=chosen; export -n ARGV0; sh -c 'echo "[$0]"'`, "[sh]",
			true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, func(r *interp.Runner) {
				r.Semantics.ExportedArgv0NamesTheCommand = interp.Yes
				if tc.unexports {
					r.Semantics.ExportTakesTheAttributeOff = interp.Yes
				}
			})
			if st != 0 {
				t.Fatalf("status %d: %s", st, out)
			}
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("argv[0] = %q, want %q", got, tc.want)
			}
		})
	}
}

// And the name is **spent**: it is what the child is called, not one more
// thing the child is handed. Both halves are asserted, because a shell that
// named the child and passed the variable on as well would satisfy the test
// above and still not be doing what the reference does.
func TestAnExportedArgv0IsSpentOnTheNameRatherThanHandedOver(t *testing.T) {
	const src = `ARGV0=chosen sh -c 'echo "[$0][${ARGV0-unset}]"'`
	for _, tc := range []struct {
		name   string
		answer interp.Answer
		want   string
	}{
		{"read as a name", interp.Yes, "[chosen][unset]"},
		// Where the axis says no, nothing happens at all: the child keeps
		// the name it was typed as and finds the variable in its
		// environment, which is what every other column in the panel does.
		{"or left alone entirely", interp.No, "[sh][chosen]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, src, argv0Named(tc.answer))
			if st != 0 {
				t.Fatalf("status %d: %s", st, out)
			}
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// This shell's own copy survives the command, which is what says only the
// child's environment is touched.
func TestAnExportedArgv0SurvivesTheCommandItNamed(t *testing.T) {
	out, st := run(t, `export ARGV0=kept; sh -c : ; echo "[$ARGV0]"`, argv0Named(interp.Yes))
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	if got := strings.TrimSpace(out); got != "[kept]" {
		t.Errorf("after the command ARGV0 = %q, want %q", got, "[kept]")
	}
}

// The other half of the bar, and the reason the axis is keyed on the *name*
// rather than on the shape: every other assignment prefix goes on doing
// exactly what it did, naming nothing and reaching the child as a variable.
func TestAnOrdinaryPrefixStillReachesTheChildAndNamesNothing(t *testing.T) {
	out, st := run(t, `V=1 sh -c 'echo "[$0][$V]"'`, argv0Named(interp.Yes))
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	if got := strings.TrimSpace(out); got != "[sh][1]" {
		t.Errorf("got %q, want %q — the prefix must still export and must not rename", got, "[sh][1]")
	}
}

// `exec -a` is the other way to say this, and where both are written the
// option wins: it is per-command and explicit, and the variable is ambient.
//
// The second row is what makes the first one evidence — with no `-a` the
// variable still names the replacement, so a shell that had simply stopped
// reading ARGV0 at `exec` would pass the first row and fail this one.
func TestExecPrefersItsOwnNameOverAnExportedArgv0(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// In a subshell, so the test's own process survives being replaced.
		{"the option wins", `( export ARGV0=fromenv; exec -a chosen sh -c 'echo "[$0]"' )`, "[chosen]"},
		{"and with no option the variable still names it", `( export ARGV0=fromenv; exec sh -c 'echo "[$0]"' )`, "[fromenv]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, func(r *interp.Runner) {
				r.Semantics.ExportedArgv0NamesTheCommand = interp.Yes
				r.Semantics.ExecTakesOptions = interp.Yes
			})
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("argv[0] = %q, want %q", got, tc.want)
			}
		})
	}
}

// argv0Named answers the one axis these tests turn on, by name.
func argv0Named(a interp.Answer) func(*interp.Runner) {
	return func(r *interp.Runner) { r.Semantics.ExportedArgv0NamesTheCommand = a }
}

// The control for the axis: the standard has no variable that names the
// command it stands in front of, so the preset every dialect starts from
// answers no — and an axis only one shell says yes to is exactly the kind a
// preset change could switch on everywhere with no test noticing.
func TestTheStandardHasNoNameForTheCommand(t *testing.T) {
	if got, want := interp.PosixSemantics().ExportedArgv0NamesTheCommand, interp.No; got != want {
		t.Errorf("PosixSemantics().ExportedArgv0NamesTheCommand = %v, want %v", got, want)
	}
}

// Two spellings of the name on one command, which is the rule about *which*
// entry is read. The prefix is laid down after this shell's exported names,
// the same way `PATH=/a PATH=/b cmd` resolves to `/b`, so the last one wins —
// measured on the reference, where `export ARGV0=first; ARGV0=second cmd`
// names the child `second`.
//
// It also pins the other half: every entry of that name goes, so nothing is
// left for the child to find.
func TestTheLastExportedArgv0IsTheNameAndNoneOfThemReachTheChild(t *testing.T) {
	const src = `export ARGV0=first; ARGV0=second sh -c 'echo "[$0][${ARGV0-unset}]"'`
	out, st := run(t, src, argv0Named(interp.Yes))
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	if got := strings.TrimSpace(out); got != "[second][unset]" {
		t.Errorf("got %q, want %q", got, "[second][unset]")
	}
}
