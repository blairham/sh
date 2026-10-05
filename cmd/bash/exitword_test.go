// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `bash -i -c 'exit 3'` writes `exit` on its way out, and `bash -i
// script.sh` whose script runs the same `exit 3` writes nothing.
//
// Measured 2026-09-21 on bash 5.3.20 and 3.2.57, `env -i` with a scratch HOME
// and no terminal on any of the three standard streams. Four rows, and the
// pair that matters is the first two — the same binary, interactive on both,
// ending through the same `exit`, and only one of them says the word:
//
//	-i -c 'exit 3'                      exit
//	-i script.sh whose script exits 3   nothing
//	-i -c 'true'                        nothing
//	-c 'exit 3'                         nothing
//
// This is the end-to-end row. driver/leavingcommandstring_test.go asks
// whether the front end reads a route set at all, with a dialect of its own;
// here the word, the route and the `exit` builtin are one binary, which is
// the only place a dialect that names no route and a front end that ignores
// the one it was given look different (#4008).
//
// The two job-control lines an interactive shell with no terminal writes are
// part of the transcript here and are not what is being asserted, so each row
// is read as the **last** line of the error stream.
func TestAnInteractiveCommandStringSaysExitOnItsWayOut(t *testing.T) {
	script := filepath.Join(t.TempDir(), "quits.sh")
	if err := os.WriteFile(script, []byte("echo ran\nexit 3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		argv []string
		last string
		code int
	}{
		{
			name: "the command string route",
			argv: []string{"-i", "-c", "exit 3"},
			last: "exit", code: 3,
		},
		{
			name: "a named script is interactive and silent",
			argv: []string{"-i", script},
			last: "bash: no job control in this shell", code: 3,
		},
		{
			name: "a command string that simply runs out",
			argv: []string{"-i", "-c", "true"},
			last: "bash: no job control in this shell",
		},
		{
			name: "and no -i at all",
			argv: []string{"-c", "exit 3"},
			last: "", code: 3,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs, code := captureArgs(t, "", tc.argv...)
			lines := strings.Split(strings.TrimSuffix(errs, "\n"), "\n")
			if got := lines[len(lines)-1]; got != tc.last {
				t.Errorf("the error stream ended %q, want %q (whole stream %q)", got, tc.last, errs)
			}
			if code != tc.code {
				t.Errorf("status %d, want %d", code, tc.code)
			}
		})
	}
}

// **A login shell says `logout` instead**, on every route that says `exit`,
// and the `logout` builtin says nothing at all (#5871).
//
// Measured 2026-10-04 against bash 5.3.20, no startup files: through a
// pseudo-terminal `exit`, `exit 3` and `^D` write `logout` under `-l` and
// under an argv[0] of `-bash`, and `logout` writes nothing; with no terminal,
//
//	-l -i -c 'exit 3'      logout
//	-l -i -c 'logout 3'    nothing
//
// The prompt's rows here are a session on a pipe, which is the same loop a
// terminal's session runs and writes the same word; the error stream's last
// line is read for the reason the rows above read it.
func TestALoginShellSaysLogoutOnItsWayOut(t *testing.T) {
	quiet := "bash: no job control in this shell"
	for _, tc := range []struct {
		name  string
		stdin string
		argv  []string
		last  string
		code  int
	}{
		{"exit in a login command string", "", []string{"-l", "-i", "-c", "exit 3"}, "logout", 3},
		{"logout in a login command string", "", []string{"-l", "-i", "-c", "logout 3"}, quiet, 3},
		{"exit at a login prompt", "exit 3\n", []string{"-l", "-i"}, "logout", 3},
		{"the input running out at a login prompt", "true\n", []string{"-l", "-i"}, "logout", 0},
		{"logout at a login prompt", "logout 3\n", []string{"-l", "-i"}, "", 3},
		{"exit at a prompt that is not a login one", "exit 3\n", []string{"-i"}, "exit", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs, code := captureArgs(t, tc.stdin, tc.argv...)
			lines := strings.Split(strings.TrimSuffix(errs, "\n"), "\n")
			got := lines[len(lines)-1]
			if tc.last == "" {
				if strings.HasSuffix(strings.TrimSuffix(errs, "\n"), "logout") || strings.HasSuffix(strings.TrimSuffix(errs, "\n"), "exit") {
					t.Errorf("logout said a word on its way out: error stream %q", errs)
				}
			} else if got != tc.last && !strings.HasSuffix(got, "$ "+tc.last) {
				// The word on the last prompt's row is the same word: where
				// it lands is #4011's question, not this one.
				t.Errorf("the error stream ended %q, want %q (whole stream %q)", got, tc.last, errs)
			}
			if code != tc.code {
				t.Errorf("status %d, want %d", code, tc.code)
			}
		})
	}
}
