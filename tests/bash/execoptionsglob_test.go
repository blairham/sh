// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExecNameIsMatchedLikeAnyWord is the control for zsh's
// ExecOptionsAreReadBeforeGlobbing: here the name `-a` takes is an ordinary
// word, so beside files foo1 and foo2 it becomes foo1 and foo2 is the command
// — here a file on PATH that is not executable.
// Measured 2026-10-02 on bash 5.3.20: `exec: foo2: not found`, where zsh
// 5.9.2 runs the child under the name `foo*` (#5138).
func TestExecNameIsMatchedLikeAnyWord(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"foo1", "foo2"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ src, want string }{
		{`(exec -a foo* /bin/sh -c 'printf "[%s]\n" "$0"') 2>&1`, "foo2: Permission denied"},
		{`(exec -a fo? /bin/sh -c 'printf "[%s]\n" "$0"') 2>&1`, "[fo?]"},
	} {
		out, _ := runBash(t, dir, tc.src)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s = %q, want it to contain %q", tc.src, out, tc.want)
		}
	}
}
