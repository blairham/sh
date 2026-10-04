// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// A value's trailing backslash is a character, and the script's live
// metacharacter behind it stays live. Measured 2026-10-03 on zsh 5.9.2 in a
// directory holding `a*b`, `a\*b` and `a\\*b`.
func TestAValueBackslashLeavesTheScriptsMetacharacterLive(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{`a*b`, `a\*b`, `a\\*b`} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct{ src, want string }{
		{`v='a\'; print -rn -- ${~v}*b`, `a\*b a\\*b`},
		{`v='a\'; print -rn -- ${~v}?b`, `a\*b`},
		{`setopt globsubst; v='a\'; print -rn -- $v*b`, `a\*b a\\*b`},
		// The control: a metacharacter the script quoted stays quoted.
		{`v='a\'; print -rn -- ${~v}"*"b`, `a\*b`},
	} {
		if out, _ := runZshOnPath(t, dir, c.src); out != c.want {
			t.Errorf("%s: %q, want %q", c.src, out, c.want)
		}
	}
}
