// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestQualifiersAskTheFilesystemOnlyWhatTheyNeed pins the answers #6363 must
// not move while taking system calls out of the qualifier filter: a list with
// no test in it no longer stats each name, and a test that follows asks stat
// before lstat.
//
// The rows are the ones whose answer depends on what was asked. A name that is
// not there must still be dropped by a test-less list, which is now the walk's
// job alone; a dangling link must still answer for itself under `-`; and a
// link to a directory must answer for its target under `-` and for itself
// without. Measured 2026-10-15 on zsh 5.9.2 (/opt/homebrew/bin/zsh, -f) in a
// directory holding `d1/sub/x`, a regular file `f1`, a link `l1` to it, a link
// `ld` to `d1` and a dangling link `dl`.
func TestQualifiersAskTheFilesystemOnlyWhatTheyNeed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d1", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"f1", filepath.Join("d1", "sub", "x")} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for link, target := range map[string]string{"l1": "f1", "ld": "d1", "dl": "nowhere"} {
		if err := os.Symlink(target, filepath.Join(dir, link)); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ src, want string }{
		// No test: the names are the walk's, and a literal that is not there
		// is not one of them.
		{`print -r -- ./nope(N) /nonexistent(N) nope/(N) f1/x(N) {f1,nope}(N) **/nope(N) x`, "f1 x"},
		{`print -r -- ./f1(N) f1/(N) d1/(N) dl(N) **/x(N) ./(N)`, "./f1 d1/ dl d1/sub/x ./"},
		{`print -r -- *(N) *([1]) *(:t)`, "d1 dl f1 l1 ld d1 d1 dl f1 l1 ld"},
		// Following: stat answers, and lstat only where stat cannot.
		{`print -r -- *(-.)`, "f1 l1"},
		{`print -r -- *(-/)`, "d1 ld"},
		{`print -r -- *(-@)`, "dl"},
		{`print -r -- dl(-.N) l1(-.) ld(-/) ld/*(N)`, "l1 ld ld/sub"},
		// Not following, and the two mixed in one list in either order.
		{`print -r -- *(@)`, "dl l1 ld"},
		{`print -r -- *(.) *(/)`, "f1 d1"},
		{
			`print -r -- *(-.,@) *(.,-@) *(-@,.) *(^-@) *(-^@)`,
			"dl f1 l1 ld dl f1 dl f1 d1 f1 l1 ld d1 f1 l1 ld",
		},
		{`print -r -- *(N^u0) *(-^u0N)`, "d1 dl f1 l1 ld d1 dl f1 l1 ld"},
	} {
		out, st := runZsh(t, dir, "export LC_ALL=C\ncd "+dir+"\n"+tc.src)
		if out != tc.want+"\n" || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}
