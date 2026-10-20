// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// Under globsubst a value's `|`, `(` and `)` build a group that reaches the
// filesystem. Measured 2026-10-03 on zsh 5.9.2 in a directory holding `aa`,
// `ab` and a file literally named `aa|ab`, which is what would match were the
// bar a character.
func TestAResultSuppliesGroupSyntaxUnderGlobsubst(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"aa", "ab", "aa|ab"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src := `setopt globsubst; for P in 'aa|ab' '(aa|ab)' 'a(a|b)'; do set -- $P; print -rn -- "[$#:$*]"; done`
	if out, st := runZshOnPath(t, dir, src); out != "[2:aa ab][2:aa ab][2:aa ab]" || st != 0 {
		t.Errorf("got %q at %d", out, st)
	}
}
