// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// The EXIT trap runs once the `eval` text or the sourced file the `exit` was
// in has been left, so a redirection standing around the `.` or the `eval`
// has been put back by then and the `.`'s operands are gone — the other
// reading of Semantics.ExitTrapRunsInsideTheExitingText, which zsh takes
// (#6954).
//
// Measured 2026-10-30 on ksh93u+ 2012-08-01 over script files, `inc.sh`
// holding `exit 0`.
func TestTheExitTrapRunsOnceTheTextThatExitedIsLeft(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "inc.sh"), []byte("exit 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, src, want string }{
		{"a redirection around the `.`", "trap 'echo T' EXIT\n{ . ./inc.sh; } >/dev/null\n", "T\n"},
		{"a redirection around the eval", "trap 'echo T' EXIT\n{ eval 'exit 0'; } >/dev/null\n", "T\n"},
		{"the sourced file's operands", "trap 'echo 1=$1' EXIT\nset -- top\n. ./inc.sh inner\n", "1=top\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got, _ := runKsh(t, dir, c.src); got != c.want {
				t.Errorf("wrote %q, want %q", got, c.want)
			}
		})
	}
}
