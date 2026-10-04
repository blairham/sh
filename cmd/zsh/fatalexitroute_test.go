// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAFailedExpansionLeftAtZeroEndsAScriptAtOne is the route half of
// TestAFailedExpansionInADeclarationOrCaseLeavesTheStatus: the status a failed
// expansion in a declaration or a `case` leaves alone is what a `-c` string
// ends on, but a script file ends at 1 where that status is 0, and keeps one
// that is not. Measured 2026-10-03 on zsh 5.9.2 under -f.
func TestAFailedExpansionLeftAtZeroEndsAScriptAtOne(t *testing.T) {
	for _, tc := range []struct {
		src    string
		status int
	}{
		{"set -u\ncase ${NOPEV} in *) printf miss;; esac\n", 1},
		{"case x in $((1/0))) ;; esac\n", 1},
		{"local x=$((1/0))\n", 1},
		{"f(){ return 3; }; f; local x=$((1/0))\n", 3},
		{"(case x in $((1/0))) ;; esac); print st=$?\n", 0},
	} {
		script := filepath.Join(t.TempDir(), "s.zsh")
		if err := os.WriteFile(script, []byte(tc.src), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, status := runZsh(t, "-f", script); status != tc.status {
			t.Errorf("%q from a file: status %d, want %d", tc.src, status, tc.status)
		}
		if _, _, status := runZsh(t, "-fc", tc.src); tc.status != 3 && status != 0 {
			t.Errorf("%q through -c: status %d, want 0", tc.src, status)
		}
	}
}
