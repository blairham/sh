// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Where a session's lines go is decided by HISTFILE **when the session ends**,
// not when it started.
//
// This shell read the variable once, at startup, and wrote there whatever the
// session had since done to it — so `HISTFILE=` at the prompt, which is a
// session asking in the plainest way it can not to be recorded, rewrote the
// file it used to name anyway. The suite's inner interactive shells do exactly
// that on their first line, which is how it was found (#4177).
//
// Measured 2026-09-23 on bash 5.3.20 with two lines already in the file:
// `HISTFILE=` leaves it untouched, and `HISTFILE=other` gives `other` this
// session's lines and leaves the first file as it was.
func TestTheHistoryGoesWhereHistfileSaysWhenTheSessionEnds(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first")
	second := filepath.Join(dir, "second")
	seed := "echo 0\necho 1\n"

	read := func(path string) string {
		b, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return "<absent>"
		}
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	run := func(text string) {
		t.Helper()
		if err := os.WriteFile(first, []byte(seed), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = os.Remove(second)
		var out, errs strings.Builder
		r := newTestRunner(map[string]string{"PS1": "", "PS2": "", "HISTFILE": first})
		r.Stdout = &out
		s := Shell{Runner: r, In: strings.NewReader(text), Out: &out, Err: &errs}
		if _, err := s.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("cleared, so nothing is recorded anywhere", func(t *testing.T) {
		run("HISTFILE=\n: one\n")
		if got := read(first); got != seed {
			t.Errorf("the file it used to name holds %q, want it untouched at %q", got, seed)
		}
	})

	t.Run("pointed elsewhere, so the lines go there", func(t *testing.T) {
		run("HISTFILE=" + second + "\n: one\n")
		if got := read(first); got != seed {
			t.Errorf("the first file holds %q, want it untouched at %q", got, seed)
		}
		got := read(second)
		if !strings.Contains(got, ": one") {
			t.Errorf("the second file holds %q, want this session's lines", got)
		}
		if strings.Contains(got, "echo 0") {
			t.Errorf("the second file holds %q, want the first file's lines left behind", got)
		}
	})

	t.Run("left alone, so the lines are appended where they were", func(t *testing.T) {
		run(": one\n")
		got := read(first)
		if !strings.HasPrefix(got, seed) || !strings.Contains(got, ": one") {
			t.Errorf("the file holds %q, want the seed and this session's line", got)
		}
	})
}
