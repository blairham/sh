// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `command` in front of a special builtin takes that builtin's specialness
// away, which is the whole of why POSIX has the word: a special builtin's
// failure is fatal to a non-interactive shell, and this is the one spelling a
// script has for surviving one.
//
// Measured 2026-09-13 from a script file under `env -i`, each refusal written
// twice — once bare and once behind `command`. Without the word, bash invoked
// as `sh`, ksh93, dash and BusyBox ash print the refusal and stop; with it,
// all four print the same refusal, report the same status and carry on. bash
// under its own name does not stop here either way and zsh's `command` does
// not reach a builtin at all, so those two columns say nothing — and the four
// that speak are unanimous, which is why nothing below is asked of a dialect
// (#2741).
//
// Every test pins the fatality axes on, so a run that never made a refusal
// fatal could not pass by accident: the control half of each pair requires
// the script to *stop*.
func survivableSem() Semantics {
	sem := PosixSemantics()
	sem.CommandReachesABuiltin = Yes
	sem.BadSetOptionLetterFatal = Yes
	sem.BadSetOptionNameFatal = Yes
	sem.BadOptionToSpecialBuiltinFatal = Yes
	sem.ShiftPastEndFatal = Yes
	return sem
}

func runSurvivable(t *testing.T, src string) (string, int) {
	t.Helper()
	sem := survivableSem()
	return run(t, src, func(r *Runner) { r.Semantics = &sem })
}

// runSurvivableWith is the same vector with the #2755 axis moved, which is
// the only difference between the two readings of the word.
func runSurvivableWith(t *testing.T, a Answer, src string) (string, int) {
	t.Helper()
	sem := survivableSem()
	sem.FatalErrorEndsAtTheCommandWord = a
	return run(t, src, func(r *Runner) { r.Semantics = &sem })
}

// TestCommandMakesASpecialBuiltinsFailureSurvivable is the row the issue is
// named for, and the three refusals behind it are decided in three different
// places — `set`'s own axis per spelling, the shared bad-option route, and
// `shift` past the end. A fix at one of them would leave the other two.
func TestCommandMakesASpecialBuiltinsFailureSurvivable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure string
	}{
		{"a refused `set` option letter", "set -Z"},
		{"a refused `set -o` name", "set -o zzznosuch"},
		{"a bad option to another special builtin", "export -q"},
		{"a `shift` past the end", "shift 99"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The control first: bare, the failure ends the script, so
			// nothing behind it runs. Without this the test would pass under
			// a shell that had simply stopped calling the refusal fatal.
			out, st := runSurvivable(t, tc.failure+"\necho after\n")
			if strings.Contains(out, "after") || st == 0 {
				t.Fatalf("bare %q gave %q at %d, want the script to stop", tc.failure, out, st)
			}

			out, st = runSurvivable(t, "command "+tc.failure+"\necho \"st=$?\"\necho after\n")
			if !strings.HasSuffix(out, "after\n") {
				t.Errorf("`command %s` gave %q at %d, want the script to carry on", tc.failure, out, st)
			}
			if st != 0 {
				t.Errorf("`command %s` left the shell at %d, want 0 — the script outlived it", tc.failure, st)
			}
			if !strings.Contains(out, "st=") || strings.Contains(out, "st=0") {
				t.Errorf("`command %s` gave %q, want the refusal's own nonzero status reported", tc.failure, out)
			}
		})
	}
}

// The refusal itself is unchanged: the word takes the fatality away and
// nothing else. A `command` that swallowed the diagnostic as well would pass
// every assertion above.
func TestCommandLeavesTheRefusalItself(t *testing.T) {
	bare, _ := runSurvivable(t, "set -Z\n")
	behind, _ := runSurvivable(t, "command set -Z\n")
	if bare == "" {
		t.Fatalf("the bare refusal said nothing, so there is nothing to compare")
	}
	if behind != bare {
		t.Errorf("`command set -Z` said %q, want the same as the bare refusal's %q", behind, bare)
	}
}

// What it must not take is a request to *stop*, which is the line abandonKind
// already draws for `.` and `eval`. Measured in the same run and unanimous:
// `command eval 'exit 5'` exits 5 and `set -e; command eval false` stops.
func TestCommandDoesNotSwallowARequestToStop(t *testing.T) {
	out, st := runSurvivable(t, "command eval 'exit 5'\necho after\n")
	if strings.Contains(out, "after") || st != 5 {
		t.Errorf("`command eval 'exit 5'` gave %q at %d, want the shell to exit 5", out, st)
	}

	out, st = runSurvivable(t, "set -e\ncommand eval false\necho after\n")
	if strings.Contains(out, "after") || st == 0 {
		t.Errorf("errexit under `command` gave %q at %d, want the script to stop", out, st)
	}
}

// And how far past the builtin the word named it reaches is the axis, which
// is where the panel parts: bash invoked as `sh` ends the script for `command
// eval 'export -q'` where ksh93, dash and BusyBox ash end only the `eval`'s
// text (#2755).
//
// The vector here keeps `eval` from being a boundary of its own
// (FatalErrorEndsBorrowedTextOnly is No in PosixSemantics), so the only thing
// between the error and the end of the script is the `command`.
func TestFatalErrorEndsAtTheCommandWordOrDoesNot(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup string
		text  string
	}{
		{"a special builtin's refusal", "", "export -q"},
		{"`${x?word}` firing", "", `echo "${NOPE?bad}"`},
		{"a readonly reassignment", "readonly rv=1\n", "rv=2"},
		{"an unset name under `set -u`", "set -u\n", `echo "${NOPE}"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.setup + "command eval '" + tc.text + "; echo inner'\necho \"st=$?\"\necho after\n"

			// The control: with the word taken off, every one of these ends
			// the script. Without it the test would pass under a shell that
			// had simply stopped calling the error fatal.
			bare, st := runSurvivableWith(t, No, tc.setup+"eval '"+tc.text+"; echo inner'\necho after\n")
			if strings.Contains(bare, "after") || st == 0 {
				t.Fatalf("bare %q gave %q at %d, want the script to stop", tc.text, bare, st)
			}

			out, st := runSurvivableWith(t, No, src)
			if strings.Contains(out, "after") || st == 0 {
				t.Errorf("answering no gave %q at %d, want the error to reach past the `command`", out, st)
			}

			out, st = runSurvivableWith(t, Yes, src)
			if !strings.HasSuffix(out, "after\n") {
				t.Errorf("answering yes gave %q at %d, want the script to carry on", out, st)
			}
			if st != 0 {
				t.Errorf("answering yes left the shell at %d, want 0 — the script outlived the error", st)
			}
			if strings.Contains(out, "inner") {
				t.Errorf("answering yes gave %q, want the `eval`'s remaining text abandoned", out)
			}
			if !strings.Contains(out, "st=") || strings.Contains(out, "st=0") {
				t.Errorf("answering yes gave %q, want the error's own nonzero status reported", out)
			}
		})
	}
}

// The wide answer must not widen what the word catches from a *request to
// stop*, which is the line abandonKind draws and which the panel is unanimous
// about. TestCommandDoesNotSwallowARequestToStop asks this of the narrow
// answer; a boundary that caught an `exit` would make `command eval 'exit 5'`
// unreachable in three dialects.
func TestTheCommandWordBoundaryStillLetsARequestToStopThrough(t *testing.T) {
	out, st := runSurvivableWith(t, Yes, "command eval 'exit 5'\necho after\n")
	if strings.Contains(out, "after") || st != 5 {
		t.Errorf("`command eval 'exit 5'` gave %q at %d, want the shell to exit 5", out, st)
	}

	out, st = runSurvivableWith(t, Yes, "set -e\ncommand eval false\necho after\n")
	if strings.Contains(out, "after") || st == 0 {
		t.Errorf("errexit under `command` gave %q at %d, want the script to stop", out, st)
	}

	out, st = runSurvivableWith(t, Yes, "command exec /nonexistent/prog\necho after\n")
	if strings.Contains(out, "after") || st == 0 {
		t.Errorf("`command exec /nonexistent/prog` gave %q at %d, want the shell to end", out, st)
	}
}
