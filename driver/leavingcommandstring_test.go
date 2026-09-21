// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// One shell in the panel writes a word as an interactive shell goes, and it
// writes it on `-i -c` — a route that draws no prompt, so the prompt
// session's own word (Diagnostics.LeavingAPromptSession, #2298) never reaches
// it.
//
// Measured 2026-09-21 on bash 5.3.20 and 3.2.57, `env -i` with a scratch HOME
// and no terminal on any of the three standard streams. `-i -c 'exit 3'`
// writes `exit` to the error stream and `-i script.sh` whose script runs the
// same `exit 3` writes nothing at all — the same binary, interactive on both,
// ending the same way. That is the split this file pins, and it is by route
// (#4008).
//
// The vector here is a dialect of this test's own rather than bash's: what is
// asserted is that the front end reads the two answers it is given, and a
// test built on the preset could not tell "the front end honors the route
// set" from "the front end writes it for every interactive shell", since bash
// is the only member with a word at all.
func leavingShell(routes syntax.ProgramRoutes) driver.Shell {
	sh := shell()
	sh.Diagnostics.LeavingAPromptSession = "leaving"
	sh.Diagnostics.LeavingIsAlsoSaidOnTheseRoutes = routes
	return sh
}

func TestAnInteractiveCommandStringSaysTheWordOnItsWayOut(t *testing.T) {
	t.Run("the command string route says it", func(t *testing.T) {
		sh := leavingShell(syntax.RouteFromCommandString)
		out, errs, code := runArgs(t, sh, "testsh", "-i", "-c", "echo ran; exit 3")
		if errs != "leaving\n" {
			t.Errorf("err = %q, want the word alone", errs)
		}
		if out != "ran\n" || code != 3 {
			t.Errorf("out = %q at %d, want the string to have run and exited 3", out, code)
		}
	})

	t.Run("and a route the dialect did not name says nothing", func(t *testing.T) {
		// The fourth row of the issue's table, and the reason this is a set
		// of routes: a named script is interactive too, runs the same
		// `exit`, and is silent.
		sh := leavingShell(syntax.RouteFromCommandString)
		path := writeScript(t, "echo ran\nexit 3\n")
		_, errs, code := runArgs(t, sh, "testsh", "-i", path)
		if errs != "" {
			t.Errorf("err = %q, want nothing on the script route", errs)
		}
		if code != 3 {
			t.Errorf("status %d, want 3", code)
		}
	})

	t.Run("a dialect that names no route says nothing", func(t *testing.T) {
		// Four of the five panel shells, and the zero value: the word alone
		// must not be enough, or every dialect that has one for its prompt
		// would have gained a second place to write it.
		sh := leavingShell(syntax.RouteOnNoRoute)
		_, errs, _ := runArgs(t, sh, "testsh", "-i", "-c", "exit 3")
		if errs != "" {
			t.Errorf("err = %q, want nothing", errs)
		}
	})

	t.Run("and a shell that was not made interactive says nothing", func(t *testing.T) {
		sh := leavingShell(syntax.RouteFromCommandString)
		_, errs, _ := runArgs(t, sh, "testsh", "-c", "exit 3")
		if errs != "" {
			t.Errorf("err = %q, want nothing without -i", errs)
		}
	})
}

// What the shell has to have *done* is run `exit`, outside any file it was
// reading. The route is a gate on the word and not the whole of it — measured
// the same day, `bash -i -c true` and `bash -i -c 'set -e; false'` are both
// silent, and so is an `exit` on a line of a file `.` opened.
func TestTheWordIsForExitAndNotForEveryWayOfEnding(t *testing.T) {
	sourced := filepath.Join(t.TempDir(), "quits.sh")
	if err := os.WriteFile(sourced, []byte("echo sourced\nexit 3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{name: "`exit` at the top of the string", src: "exit 3", want: "leaving\n"},
		{name: "`exit` in a function it defined", src: "f() { exit 3; }; f", want: "leaving\n"},
		{name: "`exit` in an `eval` of its own text", src: "eval exit 3", want: "leaving\n"},
		{name: "the string simply running out", src: "true", want: ""},
		{name: "a failing command with `set -e`", src: "set -e; false", want: ""},
		{name: "a subshell's own exit", src: "(exit 3); true", want: ""},
		{name: "`exit` on a line of a sourced file", src: ". " + sourced, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh := leavingShell(syntax.RouteFromCommandString)
			_, errs, _ := runArgs(t, sh, "testsh", "-i", "-c", tc.src)
			if errs != tc.want {
				t.Errorf("err = %q, want %q", errs, tc.want)
			}
		})
	}
}

// And it is written as `exit` runs rather than as the process ends: measured,
// `bash -i -c 'trap "echo BYE" EXIT; exit 3'` writes the word first and the
// trap's output after it.
//
// The two streams are separate buffers here, so the order is asserted by
// where the trap's own line is sent rather than by joining them — the trap
// writes to the error stream too, which is what makes one transcript out of
// the two halves.
func TestTheWordComesBeforeTheExitTrapsOwnOutput(t *testing.T) {
	sh := leavingShell(syntax.RouteFromCommandString)
	// Four of the six run a trap body that parsed, and this test needs one
	// that runs at all — the axis is not what is being asserted here.
	sh.Semantics.TrapBodyRunsWhatParsed = interp.Yes
	_, errs, _ := runArgs(t, sh, "testsh", "-i", "-c", `trap 'echo BYE >&2' EXIT; exit 3`)
	if want := "leaving\nBYE\n"; errs != want {
		t.Errorf("err = %q, want %q", errs, want)
	}
}
