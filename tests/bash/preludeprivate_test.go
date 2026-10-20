// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// The directory stack is written as shell, so its helper is a function — and
// `__dirs_rotate` is a name real bash has never heard of, so a script asking
// about it is asking about nothing (#2464).
//
// Measured 2026-09-12, `env -i PATH=/usr/bin:/bin` with a scratch HOME, over
// `-c`, against the bash on this machine:
//
//	type __dirs_rotate        bash: line 1: type: __dirs_rotate: not found  (1)
//	command -v __dirs_rotate  nothing                                       (1)
//	declare -f __dirs_rotate  nothing                                       (1)
//	type -t __dirs_rotate     nothing                                       (1)
//
// `type pushd` is a different question and deliberately not asserted here:
// this shell answers `pushd is a function` because it is one, and whether that
// should be `shell builtin` instead is #1117's trade to make.
func TestAPrivatePreludeHelperIsNotAName(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{"type __dirs_rotate", "bash: line 1: type: __dirs_rotate: not found\n", 1},
		{"command -v __dirs_rotate", "", 1},
		{"declare -f __dirs_rotate", "", 1},
		{"typeset -f __dirs_rotate", "", 1},
		{"type -t __dirs_rotate", "", 1},
		{"type -a __dirs_rotate", "bash: line 1: type: __dirs_rotate: not found\n", 1},
		// `-p` is silent for a name that is not a file, missing or not:
		// measured, `type -p nosuchname` prints nothing at 1.
		{"type -p __dirs_rotate", "", 1},
	} {
		out, st := runBashPrelude(t, dir, tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s = %q at %d, want %q at %d — the name does not exist in the shell being modeled",
				tc.src, out, st, tc.want, tc.status)
		}
	}
}

// Hidden from a report is not taken away: the helper is what `pushd +N` turns
// the stack with, so a rule that made the name unreachable would break the
// three functions the prelude does present.
func TestThePrivateHelperStillDoesTheWork(t *testing.T) {
	dir := t.TempDir()
	out, st := runBashPrelude(t, dir, "cd "+dir+"\npushd /usr >/dev/null\npushd +1 >/dev/null\ndirs -l -p\n")
	if st != 0 {
		t.Fatalf("status = %d, want 0: %q", st, out)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], dir) || lines[1] != "/usr" {
		t.Errorf("dirs -l -p = %q, want the stack rotated back to the scratch directory over /usr", out)
	}
}

// The name is the mark, and it marks the *prelude's* declaration only. A
// script writing its own `__dirs_rotate` has written a function of its own,
// and the shell answers for it — the same rule that moves a diagnostic's voice
// back to a script that redefines `pushd`.
func TestAScriptsOwnUnderscoredFunctionIsVisible(t *testing.T) {
	dir := t.TempDir()
	out, st := runBashPrelude(t, dir, "__dirs_rotate() { :; }\ntype -t __dirs_rotate\n__helper() { :; }\ntype -t __helper\n")
	if want := "function\nfunction\n"; out != want || st != 0 {
		t.Errorf("out = %q at %d, want %q at 0", out, st, want)
	}
}
