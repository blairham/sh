// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Nothing is selected until something selects it (#1858).
//
// The editing mode used to start at its zero value, which was emacs, so every
// shell this package built reported a keymap whether or not there was a line
// to edit. What a mode is *for* is which keymap a binding builtin acts on, and
// a script has no keys.
//
// One shell in the panel chooses emacs the moment it becomes interactive and
// three never choose one at all, which is Semantics.InteractiveSelectsEmacs.
// Measured 2026-09-11, at a terminal as well as without one.
func TestNoEditingModeIsSelectedUntilOneIs(t *testing.T) {
	shell := func(interactive bool, a Answer) func(*Runner) {
		return func(r *Runner) {
			s := *r.Semantics
			s.InteractiveSelectsEmacs = a
			r.Semantics = &s
			r.Interactive = interactive
		}
	}
	for _, tc := range []struct {
		name        string
		interactive bool
		a           Answer
		src         string
		want        string
	}{
		{"a script is in neither mode", false, Yes, "", "emacs off\nvi off"},
		{"and an interactive shell is in the one the dialect chooses", true, Yes, "", "emacs on\nvi off"},
		{
			// The other side of the axis, and the majority's answer: three
			// of the panel select nothing at any point.
			"a dialect that chooses none stays in none",
			true, No, "", "emacs off\nvi off",
		},
		{"and an unanswered dialect gets the majority's", true, Unspecified, "", "emacs off\nvi off"},
		// Never chosen and chosen off are two states, and this is the one
		// place they read differently: in the shell that would have selected
		// emacs, turning emacs off has to be remembered.
		{"chosen off is not never chosen", true, Yes, "set +o emacs\n", "emacs off\nvi off"},
		// And turning off the one that is not selected changes nothing,
		// which is the same rule a chosen mode follows.
		{"turning the other one off leaves the choice alone", true, Yes, "set +o vi\n", "emacs on\nvi off"},
		// A script selecting a mode is unaffected by the axis either way.
		{"a script may still select one", false, No, "set -o vi\n", "emacs off\nvi on"},
		{"in either dialect", false, Yes, "set -o emacs\n", "emacs on\nvi off"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src+"set -o\n", func(r *Runner) {
				shell(tc.interactive, tc.a)(r)
				withExtras(r)
			})
			for _, want := range strings.Split(tc.want, "\n") {
				name, state, _ := strings.Cut(want, " ")
				if !hasOptionRow(out, name, state) {
					t.Errorf("want %s %s; listing was %q", name, state, out)
				}
			}
		})
	}
}

// And the mode a binding builtin reads follows the same answer, which is the
// only thing the mode is for.
func TestTheModeABindingBuiltinReadsFollowsIt(t *testing.T) {
	for _, tc := range []struct {
		name        string
		interactive bool
		a           Answer
		src         string
		want        EditingMode
	}{
		{"a script has chosen nothing", false, Yes, "", EditingModeNone},
		{"an interactive shell in the dialect that chooses", true, Yes, "", EditingModeEmacs},
		{"and in one that does not", true, No, "", EditingModeNone},
		{"a script that asked for vi", false, Yes, "set -o vi\n", EditingModeVi},
		{"and one that turned it off again", false, Yes, "set -o vi\nset +o vi\n", EditingModeNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var shell *Runner
			run(t, tc.src, func(r *Runner) {
				s := *r.Semantics
				s.InteractiveSelectsEmacs = tc.a
				r.Semantics = &s
				r.Interactive = tc.interactive
				shell = r
			})
			if got := shell.EditingMode(); got != tc.want {
				t.Errorf("EditingMode = %v, want %v", got, tc.want)
			}
		})
	}
}
