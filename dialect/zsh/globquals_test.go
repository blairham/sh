// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// qualifierDir builds the directory these rows were measured against: a
// directory, two regular files, a symbolic link, and a hidden name.
func qualifierDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"f1", "f2", ".dot"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "d1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("f1", filepath.Join(dir, "l1")); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Where a group *came from* decides whether it is one, and this shell's own
// answers are what settle it.
//
// The substrate's tests name the grammar flag; this one names the shell,
// because the axes it rests on are this dialect's: an unquoted expansion is
// neither split nor globbed here, so a `(` arriving from a value is four
// characters of text however it is written. Measured 2026-09-06 on zsh 5.9.2.
func TestParenthesesFromAValueAreNotAGroup(t *testing.T) {
	dir := qualifierDir(t)
	for _, tc := range []struct{ src, want string }{
		{`p="*(.)"; printf "[%s]" $p`, `[*(.)]`},
		{`p="f(1|2)"; printf "[%s]" $p`, `[f(1|2)]`},
		{`p="( x )"; printf "[%s]" $p`, `[( x )]`},
		// The same text written literally *is* a group, which is what makes
		// the rows above a fact about provenance rather than about
		// parentheses.
		{`printf "[%s]" *(.)`, `[f1][f2]`},
		{`printf "[%s]" f(1|2)`, `[f1][f2]`},
	} {
		out, st := runZsh(t, dir, "cd "+dir+"\n"+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// With globbing off the words are the same words and only the group's
// reading has changed, which is what says the grammar half is unconditional.
//
// Measured: `setopt no_glob; print -l MY ( x )` prints `MY` and `( x )` on
// two lines there, and a function counting `$#` says 2.
func TestWithGlobbingOffTheGroupIsOnlyText(t *testing.T) {
	dir := qualifierDir(t)
	out, st := runZsh(t, dir, "setopt no_glob\nprintf '[%s]' MY ( x )")
	if want := `[MY][( x )]`; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
	out, st = runZsh(t, dir, "f() { printf '%s' \"$#\"; }\nsetopt no_glob\nf MY ( x )")
	if out != "2" || st != 0 {
		t.Errorf("got %q (status %d), want two arguments", out, st)
	}
}

// The diagnostics, whole rendered lines and all, because the wording is the
// promise: the character no qualifier claims is named, and a space is such a
// character.
func TestAQualifierListIsRefusedWithTheCharacterNamed(t *testing.T) {
	dir := qualifierDir(t)
	for _, tc := range []struct{ src, want string }{
		{`echo MY ( x )`, "zsh:2: unknown file attribute:  "},
		{`echo *(qqq)`, "zsh:2: unknown file attribute: q"},
		{`echo *(./)`, "zsh:2: no matches found: *(./)"},
	} {
		out, st := runZsh(t, dir, "cd "+dir+"\n"+tc.src)
		if !strings.Contains(out, tc.want) || st == 0 {
			t.Errorf("%s = %q (status %d), want %q and a failure", tc.src, out, st, tc.want)
		}
	}
}
