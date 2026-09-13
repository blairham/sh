// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The directory stack is written as shell, and two of its functions are
// machinery: `__dirs_rotate` turns the stack and `__dirs_cdopts` reads the
// letters `cd` takes. Real zsh has neither (#2464).
//
// Measured 2026-09-12, `env -i PATH=/usr/bin:/bin` with a scratch HOME, over
// `-c`, against zsh 5.9.2:
//
//	whence -w __dirs_rotate   __dirs_rotate: none          (1)
//	whence -w __dirs_cdopts   __dirs_cdopts: none          (1)
//	which __dirs_rotate       __dirs_rotate not found      (1)
//	functions __dirs_rotate   nothing                      (1)
//
// `whence -w pushd` says `builtin` in real zsh and `function` here, which is
// #1117's trade and not this one's: whatever that ends up saying, a name the
// shell does not have says `none`.
func TestAPrivatePreludeHelperIsNotAName(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{"whence -w __dirs_rotate", "__dirs_rotate: none\n", 1},
		{"whence -w __dirs_cdopts", "__dirs_cdopts: none\n", 1},
		{"whence __dirs_rotate", "", 1},
		{"which __dirs_rotate", "__dirs_rotate not found\n", 1},
		{"functions __dirs_rotate", "", 1},
		{"typeset -f __dirs_rotate", "", 1},
	} {
		out, st := runZshPrelude(t, dir, tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s = %q at %d, want %q at %d — the name does not exist in the shell being modeled",
				tc.src, out, st, tc.want, tc.status)
		}
	}
}

// Hidden from a report is not taken away: `pushd +N` rotates through the
// helper and `pushd -q` reads its option letters through the other one, so
// both have to keep working while neither has a name a script can ask about.
func TestThePrivateHelpersStillDoTheWork(t *testing.T) {
	dir := t.TempDir()
	out, st := runZshPrelude(t, dir, "cd "+dir+"\npushd -q /usr\npushd +1\ndirs -l -p\n")
	if st != 0 {
		t.Fatalf("status = %d, want 0: %q", st, out)
	}
	if want := dir + "\n/usr\n"; out != want {
		t.Errorf("dirs -l -p = %q, want %q — the stack rotated back over /usr", out, want)
	}
}
