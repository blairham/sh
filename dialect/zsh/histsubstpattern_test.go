// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestHistSubstPatternAndTheRepeatingModifiers pins `histsubstpattern`, the
// `f` and `F:n:` modifier prefixes, and `${v/#%pat/rep}`. Measured 2026-10-02
// on zsh 5.9.2 under `-f` with `extendedglob` (#5155).
func TestHistSubstPatternAndTheRepeatingModifiers(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"tmpfile1", "tmpfile2"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ src, want string }{
		{"setopt histsubstpattern; foo=(tmp*); print ${foo:s/??p/THUMP/}", "THUMPfile1 THUMPfile2\n"},
		{"foo=(tmp*); print ${foo:s/??p/THUMP/}", "tmpfile1 tmpfile2\n"},
		{"setopt histsubstpattern; foo=(one.c two.c three.c); print ${foo:s/#%(#b)t(*).c/T${match[1]}.X/}", "one.c Two.X Three.X\n"},
		{"setopt histsubstpattern; print *(#q:s/#(#b)tmp(*e)/'scrunchy${match[1]}'/)", "scrunchyfile1 scrunchyfile2\n"},
		{`setopt histsubstpattern; print ${${:-"left[({})]over"}:fs/(\\{\\}|\\(\\)|\\[\\])//}`, "leftover\n"},
		{"x=aaa; print ${x:fs/a/b/} ${x:F:2:s/a/b/} ${x:fu}", "bbb bba AAA\n"},
		{"x=two.c; print ${x/#%t*.c/X}; x=atwo.c; print ${x/#%t*.c/X}", "X\natwo.c\n"},
	} {
		got, _ := runZsh(t, dir, "setopt extendedglob; "+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
