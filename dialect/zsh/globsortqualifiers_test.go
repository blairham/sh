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

// sortDir builds a directory whose names, sizes and times each give a
// different order, so no row can pass by agreeing with the name order:
//
//	name   bytes  modified
//	a      3      2020
//	b      5      2023 (newest)
//	c      2      2021
//	a10    0      2019
//	a9     4      2018 (oldest)
//	l      -      a link to `a`, so its own size is 1
//	d/     -      2022, holding `d/x` and `d/e/y`
func sortDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, size int, year int) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(strings.Repeat("z", size)), 0o600); err != nil {
			t.Fatal(err)
		}
		at := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "d", "e"), 0o755); err != nil {
		t.Fatal(err)
	}
	write("a", 3, 2020)
	write("b", 5, 2023)
	write("c", 2, 2021)
	write("a10", 0, 2019)
	write("a9", 4, 2018)
	write("d/x", 0, 2020)
	write("d/e/y", 0, 2020)
	if err := os.Symlink("a", filepath.Join(dir, "l")); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(dir, "d"), at, at); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestTheSortQualifiersOrderTheMatches is #5951: `o` and `O` with a key
// letter sort what a pattern matched, and `n` makes the name key numeric.
// Every row measured on zsh 5.9.2 (/opt/homebrew/bin/zsh, -f) under
// `LC_ALL=C`, 2026-10-05, against the directory sortDir builds. The link's
// own modification time is the moment the test made it, so the time rows
// leave it out with `^@` — written *after* the sort, because a `^` turns
// everything behind it in the section, a sort included: `*(^@om)` is the
// oldest first.
func TestTheSortQualifiersOrderTheMatches(t *testing.T) {
	dir := sortDir(t)
	for _, tc := range []struct{ src, want string }{
		{`print -r -- *(on)`, "a a10 a9 b c d l"},
		{`print -r -- *(On)`, "l d c b a9 a10 a"},
		{`print -r -- *(^on)`, "l d c b a9 a10 a"},
		{`print -r -- *(n)`, "a a9 a10 b c d l"},
		{`print -r -- *(nOn)`, "l d c b a10 a9 a"},
		{`print -r -- *(om^@)`, "b d c a a10 a9"},
		{`print -r -- *(^@om)`, "a9 a10 a c d b"},
		{`print -r -- *(Om^@)`, "a9 a10 a c d b"},
		{`print -r -- *(om^@[1])`, "b"},
		{`print -r -- *(om^@[2,3])`, "d c"},
		{`print -r -- *([1]om^@)`, "b"},
		{`print -r -- *(.oL)`, "a10 c a a9 b"},
		{`print -r -- *(.OL)`, "b a9 a c a10"},
		// The link is its own size, one byte, until `-` follows it to `a`.
		// Following makes it tie with `a`, and a tie is not the reference's to
		// order (see globSortSpec), so that row leaves `a` out.
		{`print -r -- *(oL^/)`, "a10 l c a a9 b"},
		{`print -r -- [bcl]*(-oL^/)`, "c l b"},
		// A second specifier breaks the ties the first leaves: `d/x` and
		// `d/e/y` are both empty.
		{`print -r -- **/*(.oLOn)`, "d/x d/e/y a10 c a a9 b"},
		// `d` decides where two paths part, so a directory's own entries
		// come after its subdirectories', and the directory after them all.
		{`print -r -- **/*(odon)`, "d/e/y d/e d/x a a10 a9 b c d l"},
		{`print -r -- **/*(Odon)`, "a a10 a9 b c d l d/e d/x d/e/y"},
		// Modifiers, sorts and picks run in that order.
		{`print -r -- *(om^@:s/a/Q/)`, "b d c Q Q10 Q9"},
		{`print -r -- *(.On[1]:s/a/Q/)`, "c"},
		{`print -r -- *(.[1]:s/a/z/)`, "b"},
		{`print -r -- zz*(Nom)`, ""},
	} {
		out, st := runZsh(t, dir, "export LC_ALL=C\ncd "+dir+"\n"+tc.src)
		if out != tc.want+"\n" || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// TestNoSortIsTheDirectorysOrder is `oN`: the order the directory gave, read
// here from the directory itself, so the row cannot pass by being sorted.
func TestNoSortIsTheDirectorysOrder(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"m", "b", "y", "a", "q", "c", "x", "k"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	names, err := f.Readdirnames(-1)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join(names, " ")
	out, st := runZsh(t, dir, "cd "+dir+"\nprint -r -- *(oN)")
	if out != want+"\n" || st != 0 {
		t.Errorf("*(oN) = %q (status %d), want the directory's %q", out, st, want)
	}
	if out, _ := runZsh(t, dir, "cd "+dir+"\nprint -r -- *(oNon)"); out != want+"\n" {
		t.Errorf("*(oNon) = %q, want %q: `N` turns sorting off whatever follows", out, want)
	}
}

// TestAnUnknownSortSpecifierIsRefused: the reference's own sentence, fatal.
func TestAnUnknownSortSpecifierIsRefused(t *testing.T) {
	dir := sortDir(t)
	for _, src := range []string{`print -r -- *(ox); print after`, `print -r -- *(o); print after`, `print -r -- *(O); print after`} {
		out, st := runZsh(t, dir, "cd "+dir+"\n"+src)
		if !strings.Contains(out, "unknown sort specifier") || strings.Contains(out, "after") || st == 0 {
			t.Errorf("%s = %q (status %d), want `unknown sort specifier`, fatal", src, out, st)
		}
	}
}
