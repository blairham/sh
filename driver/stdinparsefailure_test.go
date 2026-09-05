// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// What a line that will not parse does to a program on standard input.
//
// The axis is Diagnostics.StdinProgramSurvivesAParseFailure, and it is asked
// only on this route: the same lines from a file stop every dialect, which the
// last test here holds.

// parseFailureShell is a front end with the axis set one way or the other, and
// with a parse failure numbered so the tests can tell the failure's own status
// from a command's.
func parseFailureShell(readOn bool) driver.Shell {
	sh := shell()
	sh.Semantics = interp.PosixSemantics()
	sh.Diagnostics = interp.Diagnostics{
		Location:                          interp.LocationLineWord,
		SyntaxErrorStatus:                 9,
		StdinProgramSurvivesAParseFailure: readOn,
	}
	return sh
}

// TestABadLineOnStandardInputEndsTheShellOrDoesNot is the axis from both
// sides, on the program that shows it: a good line, a bad one, and a good one
// after it.
func TestABadLineOnStandardInputEndsTheShellOrDoesNot(t *testing.T) {
	const program = "echo one\n{ fi; }\necho three\n"
	for _, c := range []struct {
		name     string
		readOn   bool
		wantOut  string
		wantCode int
	}{
		{"stops at the bad line", false, "one\n", 9},
		{"reads the next line anyway", true, "one\nthree\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runStdinProgram(t, parseFailureShell(c.readOn), program)
			if out != c.wantOut {
				t.Errorf("output %q, want %q", out, c.wantOut)
			}
			if code != c.wantCode {
				t.Errorf("status %d, want %d", code, c.wantCode)
			}
			// One complaint either way: the answer decides what happens
			// after the report, not whether there is one.
			if n := strings.Count(errs, "\n"); n != 1 {
				t.Errorf("stderr %q, want exactly one complaint", errs)
			}
		})
	}
}

// The status is left behind rather than chosen, which takes two programs to
// say: what runs after the bad line reports as it always would, and where
// nothing runs after it the parse failure's own status is what is left.
func TestReadingOnDoesNotInventAStatus(t *testing.T) {
	for _, c := range []struct {
		name     string
		program  string
		wantCode int
	}{
		{"a command after the bad line reports for itself", "{ fi; }\nexit 7\n", 7},
		{"and a failing one still fails", "{ fi; }\nfalse\n", 1},
		{"with nothing after it the failure is what is left", "echo one\n{ fi; }\n", 9},
		{"as it is when the bad line is the only one", "{ fi; }\n", 9},
		// The line after the bad one exists and runs nothing, so the shell
		// reaches the end still carrying the failure's status rather than
		// being handed it on the way out. This is the row that says the
		// status is *recorded* — without that, the two rows above would
		// still pass on the return value alone.
		{"and it is carried past a line that runs nothing", "{ fi; }\n\n", 9},
		{"a comment being such a line too", "{ fi; }\n# nothing here\n", 9},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, _, code := runStdinProgram(t, parseFailureShell(true), c.program)
			if code != c.wantCode {
				t.Errorf("status %d, want %d", code, c.wantCode)
			}
		})
	}
}

// Two bad lines are two complaints and two recoveries, which is what says the
// reader is put back rather than the first failure being forgiven once.
func TestReadingOnRecoversMoreThanOnce(t *testing.T) {
	const program = "{ fi; }\necho two\n{ done; }\necho four\n"
	out, errs, code := runStdinProgram(t, parseFailureShell(true), program)
	if want := "two\nfour\n"; out != want {
		t.Errorf("output %q, want %q", out, want)
	}
	if n := strings.Count(errs, "\n"); n != 2 {
		t.Errorf("stderr %q, want two complaints", errs)
	}
	if code != 0 {
		t.Errorf("status %d, want 0", code)
	}
}

// A failure inside a construct spanning several lines recovers the same way,
// which is the case a reader that put back only one line would get wrong.
func TestReadingOnRecoversInsideAConstruct(t *testing.T) {
	const program = "echo one\nif :; then\n  fi fi\nfi\necho five\n"
	out, _, code := runStdinProgram(t, parseFailureShell(true), program)
	if !strings.HasPrefix(out, "one\n") || !strings.HasSuffix(out, "five\n") {
		t.Errorf("output %q, want one first and five last", out)
	}
	if code != 0 {
		t.Errorf("status %d, want 0", code)
	}
}

// And the route is the whole of the condition: the same three lines in a file,
// or in a command string, stop the same shell whichever way the axis is
// answered.
func TestACommandStringStopsAtABadLineWhateverTheAnswer(t *testing.T) {
	for _, readOn := range []bool{false, true} {
		out, _, code := runArgs(t, parseFailureShell(readOn), "testsh", "-c",
			"echo one\n{ fi; }\necho three\n")
		if want := "one\n"; out != want {
			t.Errorf("readOn=%v: output %q, want %q", readOn, out, want)
		}
		if code != 9 {
			t.Errorf("readOn=%v: status %d, want 9", readOn, code)
		}
	}
}

// And the route is the whole of the condition: the same three lines in a file
// stop the same shell, whichever way the axis is answered.
func TestAFileStopsAtABadLineWhateverTheAnswer(t *testing.T) {
	const program = "echo one\n{ fi; }\necho three\n"
	for _, readOn := range []bool{false, true} {
		sh := parseFailureShell(readOn)
		path := writeScript(t, program)
		out, _, code := runArgs(t, sh, "testsh", path)
		if want := "one\n"; out != want {
			t.Errorf("readOn=%v: output %q, want %q", readOn, out, want)
		}
		if code != 9 {
			t.Errorf("readOn=%v: status %d, want 9", readOn, code)
		}
	}
}
