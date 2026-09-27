// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What POSIX mode does to a failed expansion, and what one costs from a
// command string.
//
// The mode already swaps Semantics.FailedExpansionAbandonsTheLine from Yes to
// No — an expansion the shell cannot perform ends it rather than costing the
// line — and an assignment **prefix** whose value will not expand was reaching
// a different door: the kind-keyed containment two other columns have, which
// gave the mode's column their answer for a function, an external and a word
// that names nothing. Measured 2026-09-26 on bash 5.3.20 over a script file,
// `set -o posix` on the line before: `a=$((1/0))` in front of `echo RAN`, `:`,
// a function, `/bin/echo RAN`, `command echo RAN` and `nosuch_zz` writes the
// complaint and stops in all six, and writes the complaint and reaches the
// next line in all six without the mode (#4686).
//
// The status is the second half and is not the mode's: a failed expansion that
// ends the shell exits with
// Diagnostics.ExpansionFailureStatusFromCommandString when the program came
// from `-c`, which is 127 in that column and the same number `${x?word}`
// already exits with there. From a file it is the ordinary fatal status.
//
// Named for the fields and never for a shell.

func posixExpansionRun(t *testing.T, src string, posix bool, route Route, fromString int) (string, int) {
	t.Helper()
	sem := permissive()
	sem.FailedExpansionAbandonsTheLine = Yes
	sem.FatalErrorStatusIsOne = Yes
	// The containment the mode has to override, at the answer the column
	// with the mode gives on its own: a prefix refusal is never fatal there,
	// so without the override a failed prefix would be contained in the
	// command and the next line would run.
	sem.PrefixRefusalFatality = PrefixRefusalNeverFatal
	return sourceRunWith(t, t.TempDir(), src, sem,
		Diagnostics{ExpansionFailureStatusFromCommandString: fromString},
		func(r *Runner) {
			r.Route = route
			r.SetPosixMode(posix)
		})
}

// A prefix whose value will not expand takes the same door as an ordinary word
// under the mode, for every kind of command it can stand in front of.
func TestAFailedPrefixEndsTheShellWherePosixModeSharpensIt(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"true", ":", "f", "/nonexistent/zz", "command true", "nosuch_zz"} {
		src := "f() { :; }\na=$((1/0)) " + command + "\necho AFTER\n"
		if out, _ := posixExpansionRun(t, src, true, RouteScriptFile, 0); strings.Contains(out, "AFTER") {
			t.Errorf("%q in the mode: got %q, want the shell ended", command, out)
		}
		// The control on the other side: without the mode the same line
		// gives up the line and the next one runs.
		if out, _ := posixExpansionRun(t, src, false, RouteScriptFile, 0); !strings.Contains(out, "AFTER") {
			t.Errorf("%q without the mode: got %q, want the next line to run", command, out)
		}
	}
}

// And an ordinary word and an ordinary assignment take that door too, which is
// the row the prefix was being measured against.
func TestAFailedWordEndsTheShellWherePosixModeSharpensIt(t *testing.T) {
	t.Parallel()
	for _, probe := range []string{"echo $((1/0))", "x=$((1/0))"} {
		src := probe + "\necho AFTER\n"
		if out, _ := posixExpansionRun(t, src, true, RouteScriptFile, 0); strings.Contains(out, "AFTER") {
			t.Errorf("%q in the mode: got %q, want the shell ended", probe, out)
		}
		if out, _ := posixExpansionRun(t, src, false, RouteScriptFile, 0); !strings.Contains(out, "AFTER") {
			t.Errorf("%q without the mode: got %q, want the next line to run", probe, out)
		}
	}
}

// The status a failed expansion costs a shell handed a command string, and the
// two controls that say which half of the pair decides it: the route, and the
// field being set at all.
func TestAFailedExpansionCostsACommandStringItsOwnStatus(t *testing.T) {
	t.Parallel()
	for _, probe := range []string{"echo $((1/0))", "x=$((1/0))", "a=$((1/0)) true"} {
		src := "f() { :; }\n" + probe + "\necho AFTER\n"
		if _, st := posixExpansionRun(t, src, true, RouteCommandString, 127); st != 127 {
			t.Errorf("%q from a string: status %d, want 127", probe, st)
		}
		// The same failure from a file keeps the ordinary fatal status.
		if _, st := posixExpansionRun(t, src, true, RouteScriptFile, 127); st != 1 {
			t.Errorf("%q from a file: status %d, want the ordinary fatal status", probe, st)
		}
		// And a dialect that names no number for the route keeps it too,
		// which is what says the field decides rather than the route alone.
		if _, st := posixExpansionRun(t, src, true, RouteCommandString, 0); st != 1 {
			t.Errorf("%q with no number named: status %d, want the ordinary fatal status", probe, st)
		}
	}
}
