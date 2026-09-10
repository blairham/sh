// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `set -t` is the front end's half of an interpreter option: the runner holds
// the state and this package holds the reading, so this is where the stopping
// has to be tested. Named by the axes rather than by a shell, which is the
// rule for everything outside dialect/.
//
// oneCommandShell is a shell that has the letter, with the command-string
// answer handed in — the one route the two shells that have the option
// disagree about.
func oneCommandShell(out, errs *strings.Builder, stopsACommandString interp.Answer) driver.Shell {
	sem := interp.PosixSemantics()
	sem.SetHasTheTLetter = interp.Yes
	sem.OneCommandStopsACommandString = stopsACommandString
	return driver.Shell{
		Name:        "testsh",
		Dialect:     syntax.Core(),
		Semantics:   sem,
		Diagnostics: interp.Diagnostics{},
		Stdout:      out,
		Stderr:      errs,
	}
}

// scriptAt writes src to a scratch file and answers its path.
func scriptAt(t *testing.T, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.sh")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestOneCommandStopsTheReadingLoop: the line that turned the option on
// finishes and nothing after it is read.
func TestOneCommandStopsTheReadingLoop(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"the next line is never read", "echo A\nset -t\necho B\n", "A\n"},
		{"the line that set it finishes", "set -t; echo B\necho C\n", "B\n"},
		// The check is after the line, not when the option is written, so
		// the same line can cancel it. `set -n` is one-way and this is not.
		{"the same line can take it back", "set -t; set +t\necho C\n", "C\n"},
		{"a compound command is one unit", "if true; then\n set -t\n echo IN\nfi\necho AFTER\n", "IN\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errs strings.Builder
			sh := oneCommandShell(&out, &errs, interp.No)
			if code := driver.MainArgs(sh, []string{"testsh", scriptAt(t, c.src)}); code != 0 {
				t.Errorf("status %d (stderr %q)", code, errs.String())
			}
			if out.String() != c.want {
				t.Errorf("ran %q, want %q", out.String(), c.want)
			}
		})
	}
}

// TestOneCommandStopsStandardInputToo: the same loop reads the other route
// that is a stream, so the option reaches it without a second answer.
func TestOneCommandStopsStandardInputToo(t *testing.T) {
	var out, errs strings.Builder
	sh := oneCommandShell(&out, &errs, interp.No)
	sh.Stdin = strings.NewReader("echo A\nset -t\necho B\n")
	if code := driver.MainArgs(sh, []string{"testsh", "-s"}); code != 0 {
		t.Errorf("status %d (stderr %q)", code, errs.String())
	}
	if out.String() != "A\n" {
		t.Errorf("ran %q, want A alone", out.String())
	}
}

// TestOneCommandOnACommandStringIsTheDialectsAnswer is the axis: one shell in
// the panel reads on through a `-c` string with the option on and the other
// stops there. Both answers, from the same front end.
func TestOneCommandOnACommandStringIsTheDialectsAnswer(t *testing.T) {
	for _, c := range []struct {
		answer interp.Answer
		want   string
	}{
		{interp.No, "B\n"},
		{interp.Yes, ""},
	} {
		var out, errs strings.Builder
		sh := oneCommandShell(&out, &errs, c.answer)
		if code := driver.MainArgs(sh, []string{"testsh", "-c", "set -t\necho B\n"}); code != 0 {
			t.Errorf("status %d (stderr %q)", code, errs.String())
		}
		if out.String() != c.want {
			t.Errorf("with the axis %v the string ran %q, want %q", c.answer, out.String(), c.want)
		}
	}
}

// TestOneCommandFromTheInvocationRunsOneLine: the option arriving before
// anything has been read, which is the shape a script cannot set up itself.
func TestOneCommandFromTheInvocationRunsOneLine(t *testing.T) {
	var out, errs strings.Builder
	sh := oneCommandShell(&out, &errs, interp.No)
	if code := driver.MainArgs(sh, []string{"testsh", "-t", scriptAt(t, "echo A\necho B\n")}); code != 0 {
		t.Errorf("status %d (stderr %q)", code, errs.String())
	}
	if out.String() != "A\n" {
		t.Errorf("ran %q, want A alone", out.String())
	}
}

// TestOneCommandLeavesTheStatusAlone: stopping is not an exit of its own, so
// the shell exits with what the last line left.
func TestOneCommandLeavesTheStatusAlone(t *testing.T) {
	var out, errs strings.Builder
	sh := oneCommandShell(&out, &errs, interp.No)
	if code := driver.MainArgs(sh, []string{"testsh", scriptAt(t, "set -t; (exit 7)\necho NO\n")}); code != 7 {
		t.Errorf("status %d, want the last command's 7 (stderr %q)", code, errs.String())
	}
	if out.String() != "" {
		t.Errorf("ran %q, want nothing after the line that stopped", out.String())
	}
}

// TestAShellWithoutTheLetterRefusesIt: the axis is the whole of what turns
// the letter on, so a preset that has not answered it refuses `-t` the way it
// refuses any letter it does not have — and never stops.
func TestAShellWithoutTheLetterRefusesIt(t *testing.T) {
	var out, errs strings.Builder
	sem := interp.PosixSemantics()
	sem.BadSetOptionNameFatal = interp.No
	sh := driver.Shell{
		Name:        "testsh",
		Dialect:     syntax.Core(),
		Semantics:   sem,
		Diagnostics: interp.Diagnostics{},
		Stdout:      &out,
		Stderr:      &errs,
	}
	driver.MainArgs(sh, []string{"testsh", scriptAt(t, "set -t\necho B\n")})
	if errs.String() == "" {
		t.Error("said nothing, want the letter refused")
	}
	if out.String() != "B\n" {
		t.Errorf("ran %q, want the refusal to leave the reading alone", out.String())
	}
}
