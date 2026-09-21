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
