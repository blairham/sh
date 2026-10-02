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

// The home directory a shell holds, as opposed to the `HOME` parameter a
// script can see — #4654, and two halves of one mechanism.
//
// The rows name an axis and never a shell; which dialect holds which is
// asserted in the dialect packages.

// homeRunner is a shell with no `HOME` at all and a password database that
// answers one fixed path, so that "the entry" is a string this test chose
// rather than whoever is running it. A test that read the real database would
// pass on a laptop and fail on a runner with no such user, which is what
// internal/testenv exists to stop.
func homeRunner(t *testing.T, set func(*Semantics)) (*Runner, *strings.Builder) {
	t.Helper()
	sem := PosixSemantics()
	set(&sem)
	errs := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Stderr: errs,
		Env: []string{"PATH=/usr/bin:/bin"},
		UserHomeDir: func(name string) (string, bool) {
			if name != "" {
				return "", false
			}
			return "/the/password/entry", true
		},
	})
	return r, errs
}

func runHome(t *testing.T, r *Runner, src string) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RunPart(context.Background(), f); err != nil {
		t.Fatal(err)
	}
}

// TestAShellSeedsAHomeTheEnvironmentDidNotGiveIt is the first half: a startup
// that fills in an absent `HOME` from the password entry, for the one column
// that does.
//
// The Yes row and the No row are the same shell with one field moved, which is
// what says the seed is the axis and not the hook — both have a database to
// ask and only one of them asks it.
func TestAShellSeedsAHomeTheEnvironmentDidNotGiveIt(t *testing.T) {
	t.Run("the column that seeds", func(t *testing.T) {
		r, errs := homeRunner(t, func(s *Semantics) { s.StartupFillsAnAbsentHome = Yes })
		r.SeedHomeDirectory()
		if got, ok := r.GetVar("HOME"); !ok || got != "/the/password/entry" {
			t.Errorf("HOME = %q (set %v), want the password entry — %s", got, ok, errs)
		}
	})
	t.Run("and the seeded home is exported", func(t *testing.T) {
		// Measured 2026-10-01 on zsh 5.9.2 under `env -i PATH=/usr/bin:/bin`:
		// `env | grep -c HOME` is 1, so a child sees the home the shell gave
		// itself (#5332).
		r, errs := homeRunner(t, func(s *Semantics) { s.StartupFillsAnAbsentHome = Yes })
		r.SeedHomeDirectory()
		var out strings.Builder
		r.Stdout = &out
		runHome(t, r, `/usr/bin/env`)
		if !strings.Contains(out.String(), "HOME=/the/password/entry\n") {
			t.Errorf("a child's environment was %q, want the seeded HOME in it — %s", out.String(), errs)
		}
	})
	t.Run("the column that does not", func(t *testing.T) {
		r, errs := homeRunner(t, func(s *Semantics) { s.StartupFillsAnAbsentHome = No })
		r.SeedHomeDirectory()
		if got, ok := r.GetVar("HOME"); ok {
			t.Errorf("HOME = %q, want nothing — %s", got, errs)
		}
	})
	t.Run("a HOME that was handed over is left alone", func(t *testing.T) {
		r, _ := homeRunner(t, func(s *Semantics) { s.StartupFillsAnAbsentHome = Yes })
		r.SetVar("HOME", "/was/given")
		r.SeedHomeDirectory()
		if got, _ := r.GetVar("HOME"); got != "/was/given" {
			t.Errorf("HOME = %q, want the one the shell was handed", got)
		}
	})
	t.Run("an unanswered axis seeds nothing and says nothing", func(t *testing.T) {
		// A startup seed has no line of script in front of it, so refusing
		// it would write a sentence before the shell had run anything and
		// set a status nothing has read. An unanswered value is No here.
		sem := PosixSemantics()
		sem.StartupFillsAnAbsentHome = Answer(0)
		errs := &strings.Builder{}
		r := newTestRunner(t, &Runner{
			Semantics: &sem, Diagnostics: &Diagnostics{}, Stderr: errs,
			Env: []string{"PATH=/usr/bin:/bin"},
			UserHomeDir: func(string) (string, bool) {
				return "/the/password/entry", true
			},
		})
		r.SeedHomeDirectory()
		if errs.Len() != 0 {
			t.Errorf("an unanswered axis said %q at a startup with no script in front of it", errs)
		}
		if got, ok := r.GetVar("HOME"); ok {
			t.Errorf("HOME = %q, want nothing", got)
		}
	})
	t.Run("no database, no seed", func(t *testing.T) {
		// A library embedded in a program with no business reading a
		// password file has nothing to ask, whatever its dialect says.
		r, _ := homeRunner(t, func(s *Semantics) { s.StartupFillsAnAbsentHome = Yes })
		r.UserHomeDir = nil
		r.SeedHomeDirectory()
		if got, ok := r.GetVar("HOME"); ok {
			t.Errorf("HOME = %q, want nothing", got)
		}
	})
}

// TestCdRemembersAHomeThatWasUnset is the second half. The discriminating pair
// is the same shell, the same absent `HOME`, and nothing between them but an
// assignment: only a shell that has **never** had a home says `HOME not set`.
//
// It remembers *that* it had one and not which, so the destination is empty
// rather than the old value — the `pwd` row is what says so.
func TestCdRemembersAHomeThatWasUnset(t *testing.T) {
	for _, tc := range []struct {
		name       string
		remembers  Answer
		src        string
		wantStatus int
		wantSaid   bool
	}{
		{"never had one", Yes, "cd", 1, true},
		{"assigned, then unset", Yes, "HOME=/somewhere\nunset HOME\ncd", 0, false},
		{"assigned empty, then unset", Yes, "HOME=\nunset HOME\ncd", 0, false},
		// The other reading of the same three lines, which is what makes
		// the row above a measurement rather than a shell that cannot fail.
		{"never had one, not remembering", No, "cd", 1, true},
		{"assigned then unset, not remembering", No, "HOME=/somewhere\nunset HOME\ncd", 1, true},
		// An `unset` of a name that was never there records nothing.
		{"unset with nothing to remove", Yes, "unset HOME\ncd", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, errs := homeRunner(t, func(s *Semantics) {
				s.CdWithoutHomeIsAnError = Yes
				// The remembered home is an *empty* home, so it lands on the
				// axis that already answers those rather than on a second
				// reading of its own — which is what this line being needed
				// at all says.
				s.CdEmptyHomeIsAnError = No
				s.CdRemembersAHomeThatWasUnset = tc.remembers
			})
			dir := r.Dir
			runHome(t, r, tc.src+"\n")
			if r.ExitStatus() != tc.wantStatus {
				t.Errorf("status %d, want %d — %s", r.ExitStatus(), tc.wantStatus, errs)
			}
			if said := strings.Contains(errs.String(), "HOME not set"); said != tc.wantSaid {
				t.Errorf("said %q, want `HOME not set` present=%v", errs, tc.wantSaid)
			}
			// And the shell went nowhere in either case: a remembered home is
			// an *empty* destination, not the value that was removed.
			if r.Dir != dir {
				t.Errorf("the shell moved to %q, want to stay in %q", r.Dir, dir)
			}
		})
	}
}

// TestASubshellsHomeDoesNotReachTheParent: the flag is Runner state and a
// subshell is a copy, so an assignment inside one records nothing outside it —
// which is the row the reference gives and is what a flag written at the
// `unset` gets right for free.
func TestASubshellsHomeDoesNotReachTheParent(t *testing.T) {
	r, errs := homeRunner(t, func(s *Semantics) {
		s.CdWithoutHomeIsAnError = Yes
		s.CdEmptyHomeIsAnError = No
		s.CdRemembersAHomeThatWasUnset = Yes
	})
	runHome(t, r, "(HOME=/somewhere; unset HOME)\ncd\n")
	if r.ExitStatus() != 1 || !strings.Contains(errs.String(), "HOME not set") {
		t.Errorf("status %d, said %q; want 1 and `HOME not set`", r.ExitStatus(), errs)
	}
}
