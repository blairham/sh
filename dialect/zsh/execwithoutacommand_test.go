// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAnExecOfOptionsAloneIsRefused pins the refusals `exec` makes of its own
// option words, ahead of the line's redirections and fatal at 1. Measured
// 2026-10-02 on zsh 5.9.2 under `-f` (#5380).
func TestAnExecOfOptionsAloneIsRefused(t *testing.T) {
	const none = "zsh:1: exec requires a command to execute\n"
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{"exec -c; echo after", none, 1},
		{"exec -a; echo after", none, 1},
		{"exec --; echo after", none, 1},
		{"exec -z; echo after", none, 1},
		{"exec -a x -c; echo after", none, 1},
		{"exec -c -z; echo after", none, 1},
		{"exec -a x; echo after", "zsh:1: exec flag -a requires a parameter\n", 1},
		{"exec -z -c; echo after", "zsh:1: unknown exec flag -z\n", 1},
		{"exec -z ls; echo after", "zsh:1: unknown exec flag -z\n", 1},
		{"exec -a x -z ls; echo after", "zsh:1: unknown exec flag -z\n", 1},
		{"(exec -c); echo st=$?", none + "st=1\n", 0},
		{"f(){ exec -c; echo in; }; f; echo after", "f: exec requires a command to execute\n", 1},
		// The controls: a lone dash, and a command behind the options.
		{"exec -; echo st=$?", "st=0\n", 0},
		{"exec -c echo ok", "ok\n", 0},
	} {
		got, status := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want || status != tc.status {
			t.Errorf("%s\n got %q status %d\nwant %q status %d", tc.src, got, status, tc.want, tc.status)
		}
	}
	// Before the redirection: no file is made.
	dir := t.TempDir()
	runZsh(t, dir, "exec -c >made")
	if _, err := os.Stat(filepath.Join(dir, "made")); err == nil {
		t.Error("exec -c >made made the file, want the refusal first")
	}
	// And the control, which says the probe can see a file being made.
	runZsh(t, dir, "exec >control")
	if _, err := os.Stat(filepath.Join(dir, "control")); err != nil {
		t.Errorf("exec >control made no file: %v", err)
	}
}
