// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A session at a prompt and the `history` builtin used to keep two lists that
// could not see each other.
//
// The front end remembered what was typed — the up arrow walked it, and the
// file was written at exit — and the dialect's list, which is the one
// `history`, `fc` and every `!` reference read, stayed empty. So a shell
// would draw a line back for a person and answer `history` with nothing, and
// `!!` at a prompt had nothing to match (#4177).
//
// Measured 2026-09-22 on bash 5.3.20, `--norc -i` with the lines on a pipe
// and a scratch HOME, one shape at a time. Every want here is what bash
// printed for the same input.

// promptWithHistory runs the session with a HISTFILE of its own, seeded with
// lines, and answers standard output.
func promptWithHistory(t *testing.T, typed string, seed ...string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "hf")
	if len(seed) > 0 {
		if err := os.WriteFile(file, []byte(strings.Join(seed, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HISTFILE", file)
	out, _, _ := prompt(t, typed, "bash", "--norc", "-i")
	return out
}

// The lines a session types reach the builtin's list.
func TestAPromptsLinesReachTheHistoryBuiltin(t *testing.T) {
	out := promptWithHistory(t, "echo one\necho two\nhistory\n")
	want := "one\ntwo\n    1  echo one\n    2  echo two\n    3  history\n"
	if out != want {
		t.Errorf("out %q, want %q", out, want)
	}
}

// And so do the lines an earlier session left in the file, which the front
// end reads before either loop starts.
func TestAnEarlierSessionsLinesReachTheHistoryBuiltin(t *testing.T) {
	out := promptWithHistory(t, "history\n", "echo alpha", "echo beta")
	want := "    1  echo alpha\n    2  echo beta\n    3  history\n"
	if out != want {
		t.Errorf("out %q, want %q", out, want)
	}
}

// A `!` reference at a prompt resolves against that same list, which the loop
// with no editor never consulted.
func TestAHistoryReferenceIsExpandedWithNoEditor(t *testing.T) {
	out := promptWithHistory(t, "echo one two three\n!!\necho !$\n")
	want := "one two three\none two three\nthree\n"
	if out != want {
		t.Errorf("out %q, want %q", out, want)
	}
}

// And the builtins that drop the line they are written on can do it here too,
// which is what says the front end has told the runner it is the one filling
// the list: `fc -l` leaves its own line out.
func TestABuiltinAtAPromptDropsItsOwnLine(t *testing.T) {
	out := promptWithHistory(t, "echo one\nfc -l\n")
	want := "one\n1\t echo one\n"
	if out != want {
		t.Errorf("out %q, want %q", out, want)
	}
}
