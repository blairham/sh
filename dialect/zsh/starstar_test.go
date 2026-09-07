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
