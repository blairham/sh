// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// starStarDir is the tree these rows were measured against: two names at the
// top, one of them a directory, and a name repeated two levels down so a
// pattern's answer says how many levels it reached.
func starStarDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cx", "dx"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"ax", "bx", "cx/ax", "cx/dx/ax"} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(n)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// `**/` crosses directory levels here with no option asked for, and it stands
// for **zero or more** of them — so the match in the starting directory is
// part of the answer rather than the one the listing leaves out.
//
// Measured 2026-09-07 on zsh 5.9.2 in the tree above. The zero-level row is
// the one that matters: `**/a*` answering `cx/ax` alone is a *shorter* listing
// at status 0, which is the failure shape with nothing to notice (#1339). The
// deepest row is the control that separates "zero levels missing" from "only
// one level reached" — a component read as `*` answers `cx/ax` too.
func TestStarStarSlashCrossesLevelsWithNoOption(t *testing.T) {
	dir := starStarDir(t)
	for _, tc := range []struct{ src, want string }{
		{`print -r -- **/a*`, "ax cx/ax cx/dx/ax"},
		{`print -r -- **/ax`, "ax cx/ax cx/dx/ax"},
		{`print -r -- cx/**/ax`, "cx/ax cx/dx/ax"},
		{`print -r -- **/*x`, "ax bx cx cx/ax cx/dx cx/dx/ax"},
		// The control: one level, which is what the component used to mean
		// and what a single star still means.
		{`print -r -- */a*`, "cx/ax"},
	} {
		out, st := runZsh(t, dir, "cd "+dir+"\n"+tc.src)
		if got := strings.TrimSpace(out); got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, got, st, tc.want)
		}
	}
}

// Bare `**` is not that construct. With nothing behind it the component is an
// ordinary pattern here, where adjacent stars collapse to one — measured on
// zsh 5.9.2, `print -r -- **` is `ax bx cx` and `print -r -- cx/**` is
// `cx/ax cx/dx`, both of which are what a single star answers.
//
// This is the half bash and ksh93 answer the other way, and it is why the
// crossing is two questions in the substrate rather than one flag: turning
// `**/` on for this shell must not turn `**` into a recursive listing.
func TestBareStarStarIsAnOrdinaryPattern(t *testing.T) {
	dir := starStarDir(t)
	for _, tc := range []struct{ src, want string }{
		{`print -r -- **`, "ax bx cx"},
		{`print -r -- cx/**`, "cx/ax cx/dx"},
		// And the same words with a single star, which is the claim.
		{`print -r -- *`, "ax bx cx"},
		{`print -r -- cx/*`, "cx/ax cx/dx"},
		// Anything more than exactly `**` is ordinary in either position.
		{`print -r -- a**`, "ax"},
		{`print -r -- c**/ax`, "cx/ax"},
	} {
		out, st := runZsh(t, dir, "cd "+dir+"\n"+tc.src)
		if got := strings.TrimSpace(out); got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, got, st, tc.want)
		}
	}
}

// TestStarStarSlashListsTheLevelsItWalked is this shell's half of #2360, and
// it is the row where the panel splits.
//
// `**/` here stands for the directory levels the walk crossed, and a symbolic
// link to a directory is not one of them: measured 2026-09-12 on zsh 5.9.2 in
// a tree holding `r/x`, a symlink `s` to `r` and a symlink `up` to the
// directory itself, `echo **/` is `r/` — where bash 5.3.15 under `shopt -s
// globstar` and ksh93 under `set -o globstar` both answer `r/ s/ up/`.
//
// What does **not** split is where the component goes: no shell in the panel
// enters a link, so `**/x` is `r/x` everywhere and the walk is bounded even
// though `up` points at its own parent. The two rows are here together
// because reading the first as the second is the mistake the option exists to
// prevent.
//
// `***/` is the construct that does follow links, and it is separate on
// purpose rather than this reading turned on — pointed at this tree zsh walks
// it until the kernel refuses and reports `too many levels of symbolic
// links`. It is not implemented, and the row below says so by measuring what
// `**/` does instead.
func TestStarStarSlashListsTheLevelsItWalked(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "r"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "r", "x"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("r", filepath.Join(dir, "s")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".", filepath.Join(dir, "up")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ src, want string }{
		{`print -r -- **/`, "r/"},
		{`print -r -- **/x`, "r/x"},
		// The starting directory is still the pattern's to name, however it
		// was reached: measured, `s/**/x` is `s/x` here and in bash.
		{`print -r -- s/**/x`, "s/x"},
		// Two controls, and both are needed. An ordinary component goes
		// through a link — `*/x` is `r/x s/x`, so the fixture does have
		// something to find that way — and an ordinary component with the
		// same trailing slash lists every link as a directory: `*/` is
		// `r/ s/ up/`, measured, which is what makes the `**/` row above a
		// statement about `**` rather than about trailing slashes.
		{`print -r -- */x`, "r/x s/x"},
		{`print -r -- */`, "r/ s/ up/"},
	} {
		out, st := runZsh(t, dir, "cd "+dir+"\n"+tc.src)
		if got := strings.TrimSpace(out); got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, got, st, tc.want)
		}
	}
}
