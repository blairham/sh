// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// What a session appends to its history file as it ends is the newest
// entries bash's own count of unwritten lines covers, not the lines it saw
// typed — so a startup file's `history -s` line is written, and what `-c` and
// `-a` did to the count is kept (#5967).
//
// Measured 2026-10-05 on bash 5.3.20 (/opt/homebrew/bin/bash), interactive on
// a pipe, `env -i` with a scratch HOME and a HISTFILE of `echo h1`, `echo h2`;
// `want` is the file afterwards. This shell appended only the lines it had
// recorded: it lost `echo hs`, kept the `history -c` the clearing took back,
// and wrote `history -a` a second time.
func TestASessionWritesWhatBashCountsAsUnwritten(t *testing.T) {
	for _, tc := range []struct {
		name, rc, typed, want string
	}{
		{"history -s", "history -s 'echo hs'", "echo t1\nexit\n", "echo h1\necho h2\necho hs\necho t1\nexit\n"},
		{"history -s, nothing typed", "history -s 'echo hs'", "exit\n", "echo h1\necho h2\necho hs\nexit\n"},
		{"history -c at the prompt", "history -s 'echo hs'", "history -c\necho t1\nexit\n", "echo h1\necho h2\necho t1\nexit\n"},
		{"history -a at the prompt", "history -s 'echo hs'", "history -a\necho t1\nexit\n", "echo h1\necho h2\necho hs\nhistory -a\necho t1\nexit\n"},
		{"control", "true", "echo t1\nexit\n", "echo h1\necho h2\necho t1\nexit\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := scratchHome(t)
			hf := filepath.Join(home, "hf")
			writeHomeFile(t, home, "hf", "echo h1\necho h2\n")
			writeHomeFile(t, home, ".bashrc", "HISTFILE="+hf+"; "+tc.rc+"\n")
			if _, errs, code := prompt(t, tc.typed, "bash", "-i"); code != 0 {
				t.Fatalf("status %d, stderr %q", code, errs)
			}
			got, err := os.ReadFile(hf)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("history file %q, want %q", got, tc.want)
			}
		})
	}
}
