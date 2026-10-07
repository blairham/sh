// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Inside Emacs, the dialect that selects an editing mode for an interactive
// shell selects none. Measured 2026-10-07 on bash 5.3.20; see
// Semantics.InsideEmacsTurnsEditingOff for the table (#6310).
func TestInsideEmacsTurnsTheEditingModeOff(t *testing.T) {
	for _, tc := range []struct {
		name        string
		vars        map[string]string
		interactive bool
		axis        bool
		want        EditingMode
	}{
		{"TERM=emacs", map[string]string{"TERM": "emacs"}, true, true, EditingModeNone},
		{"EMACS=t on a dumb terminal", map[string]string{"TERM": "dumb", "EMACS": "t"}, true, true, EditingModeNone},
		{"EMACS=t with TERM unset", map[string]string{"EMACS": "t"}, true, true, EditingModeNone},
		{"INSIDE_EMACS on a dumb terminal", map[string]string{"TERM": "dumb", "INSIDE_EMACS": "29,comint"}, true, true, EditingModeNone},
		{"INSIDE_EMACS set and empty", map[string]string{"TERM": "dumb", "INSIDE_EMACS": ""}, true, true, EditingModeNone},
		{"INSIDE_EMACS with TERM unset", map[string]string{"INSIDE_EMACS": "x"}, true, true, EditingModeNone},
		// The controls: each one a single step from a row above.
		{"a dumb terminal alone", map[string]string{"TERM": "dumb"}, true, true, EditingModeEmacs},
		{"EMACS is exactly t", map[string]string{"TERM": "dumb", "EMACS": "T"}, true, true, EditingModeEmacs},
		{"TERM empty is not unset", map[string]string{"TERM": "", "EMACS": "t"}, true, true, EditingModeEmacs},
		{"EMACS=t on a real terminal", map[string]string{"TERM": "xterm", "EMACS": "t"}, true, true, EditingModeEmacs},
		{"INSIDE_EMACS on a real terminal", map[string]string{"TERM": "xterm", "INSIDE_EMACS": "x"}, true, true, EditingModeEmacs},
		{"a dialect without the rule", map[string]string{"TERM": "emacs"}, true, false, EditingModeEmacs},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var shell *Runner
			run(t, "", func(r *Runner) {
				s := *r.Semantics
				s.InteractiveSelectsEmacs = Yes
				s.InsideEmacsTurnsEditingOff = tc.axis
				r.Semantics = &s
				r.Interactive = tc.interactive
				shell = r
			})
			for _, name := range []string{"TERM", "EMACS", "INSIDE_EMACS"} {
				if _, set := shell.GetVar(name); set {
					t.Fatalf("%s is set before the test sets it", name)
				}
			}
			for name, value := range tc.vars {
				shell.SetVar(name, value)
			}
			shell.TurnEditingOffInsideEmacs()
			if got := shell.EditingMode(); got != tc.want {
				t.Errorf("EditingMode = %v, want %v", got, tc.want)
			}
		})
	}
}
