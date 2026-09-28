// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A dialect may let a script move what `$0` answers while it runs, and the
// preset is then the shell's **default** rather than its only reading.
//
// **Every row has to be run with the switch thrown**, which is the whole
// point: the one dialect with this option has it *on* by default, so a grid
// that never turns it off agrees with the fixed reading on every row and
// shows nothing (#4436).
func TestARunTimeSwitchMovesWhatDollarZeroNames(t *testing.T) {
	// A switch a test can throw, standing in for the dialect's option.
	off := false
	setup := func(r *Runner) {
		r.Semantics.DollarZeroNames = DollarZeroIsTheInnermostCall
		r.SetDollarZeroScopeSwitch(func(*Runner) (DollarZeroScope, bool) {
			if off {
				return DollarZeroIsTheShellsOwnName, true
			}
			return 0, false
		})
	}
	for _, tc := range []struct{ name, src, thrown string }{
		{"a function", `f() { echo "0=[$0]"; }; f`, "f"},
		{"a nested call", `g() { echo "0=[$0]"; }; f() { g; }; f`, "g"},
		{"an eval inside one", `f() { eval 'echo "0=[$0]"'; }; f`, "f"},
		{"a subshell inside one", `f() { ( echo "0=[$0]" ); }; f`, "f"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Not thrown: the preset answers, which is the control.
			off = false
			out, _ := run(t, tc.src, setup)
			if !strings.Contains(out, "0=["+tc.thrown+"]") {
				t.Errorf("switch off: %s = %q, want the preset's reading %q", tc.src, out, tc.thrown)
			}
			// Thrown: the shell's own name, whatever the stack holds.
			off = true
			out, _ = run(t, tc.src, setup)
			if strings.Contains(out, "0=["+tc.thrown+"]") {
				t.Errorf("switch on: %s = %q, want the shell's own name", tc.src, out)
			}
		})
	}

	// Reporting false means "the script has not moved it", so a dialect can
	// install one switch and leave every other reading alone.
	off = false
	out, _ := run(t, `f() { echo "0=[$0]"; }; f`, func(r *Runner) {
		r.Semantics.DollarZeroNames = DollarZeroIsTheInnermostCall
		r.SetDollarZeroScopeSwitch(func(*Runner) (DollarZeroScope, bool) { return 0, false })
	})
	if !strings.Contains(out, "0=[f]") {
		t.Errorf("a switch that declines = %q, want the preset to answer", out)
	}
}
