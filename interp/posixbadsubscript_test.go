// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// POSIX mode turns "a failed expansion gives up the line" into "it ends the
// shell" — and a **bracketed** expression is the one shape it does not reach.
//
// Named for the fields and never for a shell. The discriminator is the
// bracket: every row below is run in both modes, and the subscript rows are
// the ones whose two modes are the same.
//
// Measured 2026-09-27 on bash 5.3.20 and on the 3.2.57 macOS ships, which
// agree row for row (#4784).
func TestPosixModeDoesNotSharpenABadSubscript(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name     string
		probe    string
		sharpens bool
	}{
		{"a subscript that will not evaluate", "echo ${a[1+]}", false},
		{"the same subscript in a store", "v=${a[1+]}", false},
		{"the same subscript inside a function", "g() { echo ${a[1+]}; }\ng", false},

		// The controls, each of which the mode does sharpen: they are the
		// rows that say the mode is on and doing its work, so a subscript
		// row reading "the line survived" is the bracket rather than a mode
		// that never took effect.
		{"a division by zero", "echo $((1/0))", true},
		{"a division by zero in a store", "x=$((1/0))", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			src := "a=(1 2)\n" + c.probe + "\necho AFTER\n"
			inMode, _ := posixExpansionRun(t, src, true, RouteScriptFile, 0)
			without, _ := posixExpansionRun(t, src, false, RouteScriptFile, 0)
			if !strings.Contains(without, "AFTER") {
				t.Fatalf("%q without the mode: got %q, want the next line to run", c.probe, without)
			}
			if c.sharpens && strings.Contains(inMode, "AFTER") {
				t.Errorf("%q in the mode: got %q, want the shell ended", c.probe, inMode)
			}
			if !c.sharpens && !strings.Contains(inMode, "AFTER") {
				t.Errorf("%q in the mode: got %q, want the next line to run", c.probe, inMode)
			}
		})
	}
}

// And the status a bad subscript costs a command string is the ordinary fatal
// one rather than the number a failed expansion costs that route — in either
// mode, which is the pair that says the mode does not reach this either.
//
// The control is the same line without a bracket in it, which takes the
// route's number under the mode and gives up the line without it.
func TestABadSubscriptFromACommandStringKeepsTheFatalStatus(t *testing.T) {
	t.Parallel()
	for _, posix := range []bool{true, false} {
		src := "a=(1 2)\necho ${a[1+]}\necho AFTER\n"
		out, st := posixExpansionRun(t, src, posix, RouteCommandString, 127)
		if st != 1 {
			t.Errorf("posix=%v: status %d, want the ordinary fatal status", posix, st)
		}
		if strings.Contains(out, "AFTER") {
			t.Errorf("posix=%v: got %q, want the command string given up whole", posix, out)
		}
	}
	bare := "a=(1 2)\necho $((1/0))\necho AFTER\n"
	if _, st := posixExpansionRun(t, bare, true, RouteCommandString, 127); st != 127 {
		t.Errorf("a bare failure in the mode: status %d, want the route's own number", st)
	}
	if out, _ := posixExpansionRun(t, bare, false, RouteCommandString, 127); !strings.Contains(out, "AFTER") {
		t.Errorf("a bare failure without the mode: got %q, want the next line to run", out)
	}
}

// A dialect that does not give up the line at all is unmoved by any of this:
// the mode never sharpened anything there, so a bad subscript ends the shell
// exactly as it did.
//
// This is the row that keeps the change off the other columns, and it is
// written as an axis rather than as a shell: the only thing varied is
// Semantics.FailedExpansionAbandonsTheLine.
func TestADialectThatNeverGivesUpTheLineIsUnmovedByTheMode(t *testing.T) {
	t.Parallel()
	for _, posix := range []bool{true, false} {
		sem := permissive()
		sem.FailedExpansionAbandonsTheLine = No
		sem.FatalErrorStatusIsOne = Yes
		out, st := sourceRunWith(t, t.TempDir(), "a=(1 2)\necho ${a[1+]}\necho AFTER\n", sem,
			Diagnostics{ExpansionFailureStatusFromCommandString: 127},
			func(r *Runner) {
				r.Route = RouteScriptFile
				r.SetPosixMode(posix)
			})
		if strings.Contains(out, "AFTER") {
			t.Errorf("posix=%v: got %q, want the shell ended", posix, out)
		}
		if st != 1 {
			t.Errorf("posix=%v: status %d, want the ordinary fatal status", posix, st)
		}
	}
}
