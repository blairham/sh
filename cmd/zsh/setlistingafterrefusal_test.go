// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestASetListingAfterARefusalEndsAtZero pins
// Semantics.SetListingAfterARefusalLeavesZero: a refused `set -o` name with a
// listing behind it ends a command string at 0, and one with nothing listed
// behind it at 1; from a script file both end at 1. Measured 2026-10-03 on zsh
// 5.9.2 under -f.
func TestASetListingAfterARefusalEndsAtZero(t *testing.T) {
	for _, tc := range []struct {
		src          string
		c, fromAFile int
	}{
		{"set -o nosuch -o >/dev/null; print reached", 0, 1},
		{"set -o nosuch +o >/dev/null; print reached", 0, 1},
		{"set -o nosuch -e; print reached", 1, 1},
		{"set -o nosuch -o nosuch2 >/dev/null; print reached", 1, 1},
	} {
		out, _, st := runZsh(t, "-fc", tc.src)
		if st != tc.c || out != "" {
			t.Errorf("-c %q: status %d out %q, want %d and nothing", tc.src, st, out, tc.c)
		}
		script := filepath.Join(t.TempDir(), "s.zsh")
		if err := os.WriteFile(script, []byte(tc.src+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, st := runZsh(t, "-f", script); st != tc.fromAFile {
			t.Errorf("file %q: status %d, want %d", tc.src, st, tc.fromAFile)
		}
	}
}
