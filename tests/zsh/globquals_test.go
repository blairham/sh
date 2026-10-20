// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// TestTheThreeFileTimeQualifiers: `a` and `c` are qualifiers this shell has,
// beside the `m` that was the only one implemented (#3533).
//
// The ages are set with a real change to each clock rather than left to the
// machine's: the files are stamped a hundred days back, so `-1` (newer than a
// day) keeps neither and `+1` (older than a day) keeps both, and no row
// depends on how long the test took to run. The inode-change time cannot be
// stamped — it is the kernel's own — so `c` is asserted where it is certainly
// recent instead.
//
// Measured 2026-09-18 on zsh 5.9.2 in a directory holding no match:
// `zz*(a+1)` and `zz*(c1)` are `no matches found` where this engine said
// `unknown file attribute`, and `zz*(a)` with no number behind it is `number
// expected` — the same refusal `m` gives, which is what says the letter and
// the operand are two complaints rather than one.
func TestTheThreeFileTimeQualifiers(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"t1", "t2"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-100 * 24 * time.Hour)
		if err := os.Chtimes(filepath.Join(dir, n), old, old); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ src, want string }{
		{`printf "[%s]" *(m+1)`, `[t1][t2]`},
		{`printf "[%s]" *(a+1)`, `[t1][t2]`},
		{`printf "[%s]" *(md+99)`, `[t1][t2]`},
		{`printf "[%s]" *(ad+99)`, `[t1][t2]`},
		{`printf "[%s]" *(c-1)`, `[t1][t2]`},
	} {
		out, st := runZsh(t, dir, "cd "+dir+"\n"+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
	// A time qualifier with no number behind it is the operand's complaint
	// and not the letter's, in all three.
	for _, letter := range []string{"m", "a", "c"} {
		out, st := runZsh(t, dir, "cd "+dir+"\nprintf '[%s]' *("+letter+")")
		if !strings.Contains(out, "number expected") || st == 0 {
			t.Errorf("*(%s) = %q at %d, want `number expected` and a refusal", letter, out, st)
		}
	}
}
