// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path/filepath"
	"testing"
)

// A line a startup file added with `history -s` keeps the session from
// reading its history file, and only that (#5920).
//
// Measured 2026-10-04 on bash 5.3, through a pty and with the lines on a pipe
// alike, a HISTFILE holding `echo h1` and `echo h2` and `history` typed at the
// first prompt; the full table is on dialect/bash's historySkipsTheFile.
func TestAStartupHistoryLineKeepsTheFileUnread(t *testing.T) {
	for _, tc := range []struct {
		name, rc, want string
	}{
		{"history -s", "history -s 'echo hs'", "    1  echo hs\n    2  history\n"},
		// Taken back with -d, the line no longer counts and the file is read.
		{"history -s then -d", "history -s 'echo hs'; history -d 1", "    1  echo h1\n    2  echo h2\n    3  history\n"},
		// And -r leaves the list full and still reads the file after it.
		{"history -r", "history -r ~/seed", "    1  echo s1\n    2  echo h1\n    3  echo h2\n    4  history\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := scratchHome(t)
			writeHomeFile(t, home, "hf", "echo h1\necho h2\n")
			writeHomeFile(t, home, "seed", "echo s1\n")
			writeHomeFile(t, home, ".bashrc", "HISTFILE="+filepath.Join(home, "hf")+"; "+tc.rc+"\n")
			out, errs, code := prompt(t, "history\n", "bash", "-i")
			if code != 0 {
				t.Fatalf("status %d, stderr %q", code, errs)
			}
			if out != tc.want {
				t.Errorf("out %q, want %q", out, tc.want)
			}
		})
	}
}
