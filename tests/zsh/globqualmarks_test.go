// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// markTree is a directory holding one file of each type the `T` qualifier
// writes a character for, bar a socket and a device, which a test cannot
// make portably.
func markTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "a"), nil, 0o644))
	must(os.WriteFile(filepath.Join(dir, "x"), nil, 0o755))
	must(os.Mkdir(filepath.Join(dir, "d"), 0o755))
	must(os.Symlink("a", filepath.Join(dir, "lnk")))
	must(os.Symlink("d", filepath.Join(dir, "dl")))
	must(os.Symlink("x", filepath.Join(dir, "xl")))
	must(os.Symlink("nowhere", filepath.Join(dir, "broken")))
	must(syscall.Mkfifo(filepath.Join(dir, "f"), 0o600))
	return dir
}

// `M` is `markdirs` for one pattern and `T` its form that writes a character
// for every type of file; `^` turns either off, `-` makes a link answer for
// its target. Measured 2026-10-16 on zsh 5.9.2, `-f`, over the same tree
// (#6726).
func TestTheMarkQualifiers(t *testing.T) {
	t.Parallel()
	dir := markTree(t)
	for _, tc := range []struct{ src, want string }{
		{`print -r -- *(T)`, "a  broken@ d/ dl@ f| lnk@ x* xl@\n"},
		{`print -r -- *(-T)`, "a  broken@ d/ dl/ f| lnk  x* xl*\n"},
		{`print -r -- *(T-)`, "a  broken@ d/ dl@ f| lnk@ x* xl@\n"},
		{`print -r -- *(M)`, "a broken d/ dl f lnk x xl\n"},
		{`print -r -- *(-M)`, "a broken d/ dl/ f lnk x xl\n"},
		{`print -r -- *(MT^M)`, "a  broken@ d/ dl@ f| lnk@ x* xl@\n"},
		{`print -r -- *(T^T)`, "a broken d dl f lnk x xl\n"},
		{`setopt markdirs; print -r -- *(^M)`, "a broken d dl f lnk x xl\n"},
		{`setopt markdirs; print -r -- *(^T)`, "a broken d/ dl f lnk x xl\n"},
		{`print -r -- *(T.)`, "a  x*\n"},
		{`print -r -- */(T)`, "d// dl//\n"},
		// Before a modifier, and before an `e` qualifier's code is shown
		// the name.
		{`print -r -- *(T:u)`, "A  BROKEN@ D/ DL@ F| LNK@ X* XL@\n"},
		{`print -r -- *(Te:'REPLY+=q':)`, "a q broken@q d/q dl@q f|q lnk@q x*q xl@q\n"},
		{`print -r -- *(e:'REPLY+=q':M)`, "aq brokenq d/q dlq fq lnkq xlq xq\n"},
	} {
		if got, _ := runZsh(t, dir, tc.src); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// `markdirs` itself marks a directory and not a link to one, and marks the
// name an `e` qualifier's code was shown rather than the one it chose.
// Measured 2026-10-16 on zsh 5.9.2, `-f` (#6726).
func TestMarkDirsMarksWhatTheCodeWasShown(t *testing.T) {
	t.Parallel()
	dir := markTree(t)
	for _, tc := range []struct{ src, want string }{
		{`setopt markdirs; print -r -- *`, "a broken d/ dl f lnk x xl\n"},
		{`setopt markdirs; print -r -- */`, "d// dl//\n"},
		{`setopt markdirs; print -r -- *(e:'REPLY+=q':)`, "aq brokenq d/q dlq fq lnkq xlq xq\n"},
		{`setopt markdirs; print -r -- *(e:'REPLY=q':)`, "q q q q q q q q\n"},
	} {
		if got, _ := runZsh(t, dir, tc.src); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
