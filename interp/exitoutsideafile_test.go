// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether `exit` ran, and whether a file was being read when it did.
//
// One question with two halves because a front end needs both: a shell that
// stopped some other way and a shell whose `exit` was on a line of a file it
// was reading are the same answer, and neither is the shell running the word
// itself. What reads it is the word one dialect writes on its way out of an
// interactive invocation — Diagnostics.LeavingIsAlsoSaidOnTheseRoutes carries
// what was measured for that, and driver is where it is written (#4008).
//
// Asserted here as well as through the front end because this is where the
// note is *taken*: it has to be recorded as the request is made, since by the
// time anyone asks, every file the shell was reading has been unwound and the
// answer would always be yes. A reading taken at the end would pass every row
// below but the last two.
func TestTheRunnerSaysWhetherExitRanOutsideAFile(t *testing.T) {
	sourced := filepath.Join(t.TempDir(), "quits.sh")
	if err := os.WriteFile(sourced, []byte("exit 3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		src  string
		want bool
	}{
		{name: "`exit` at the top level", src: "exit 3", want: true},
		{name: "`exit` in a function", src: "f() { exit 3; }; f", want: true},
		{name: "`exit` in an `eval`", src: "eval exit 3", want: true},
		{name: "nothing ran `exit`", src: "true"},
		{name: "a command failed under `set -e`", src: "set -e; false"},
		{name: "a subshell exited, not this shell", src: "(exit 3); true"},
		{name: "`exit` on a line of a sourced file", src: ". " + sourced},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var r *Runner
			run(t, tc.src, func(got *Runner) { r = got })
			if got := r.ExitRanOutsideAFile(); got != tc.want {
				t.Errorf("ExitRanOutsideAFile() = %v, want %v", got, tc.want)
			}
		})
	}
}
