// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"testing"
)

// The name this shell gives the parameter whose patterns take names back out
// of a pathname expansion, and the one thing about it the core cannot say:
// that the switch an assignment writes is the option `shopt` already names.
//
// What the facility does is asserted in interp, against a seam and not
// against a shell. This file is the wiring.

// globIgnoreDir is the fixture, laid out beside the tests that use it: two
// `.txt` names, one that is not, and a hidden name that is — so the reveal
// and the filter can be told apart.
func globIgnoreDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"a.txt", "b.txt", "c.log", ".dot", ".hid.txt"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestGlobIgnoreIsTheParameterThisShellNames.
//
// Measured on bash 5.3.15 and bash 3.2.57, 2026-09-13, in this fixture. The
// second row is the control that makes the first an assertion about the
// *name*: `FIGNORE` is the parameter ksh93 gives the same facility and does
// nothing whatever here.
func TestGlobIgnoreIsTheParameterThisShellNames(t *testing.T) {
	dir := globIgnoreDir(t)
	for _, tc := range []struct{ src, want string }{
		{`GLOBIGNORE='*.txt'; printf "[%s]" *`, `[.dot][c.log]`},
		{`FIGNORE='*.txt'; printf "[%s]" *`, `[a.txt][b.txt][c.log]`},
	} {
		if out, st := runBash(t, dir, tc.src); st != 0 || out != tc.want {
			t.Errorf("%s = %q status %d, want %q", tc.src, out, st, tc.want)
		}
	}
}

// TestGlobIgnoreWritesTheDotglobOption.
//
// The switch the assignment writes is `dotglob` itself and not a state beside
// it, which is what these four rows say together: the option reports on after
// an assignment, a script may write it back and the expansion follows what
// the script wrote, and the unset writes it off whoever set it.
//
// A shell that merely behaved as though hidden names were on while the
// parameter held a value would pass the first row and fail the second.
func TestGlobIgnoreWritesTheDotglobOption(t *testing.T) {
	dir := globIgnoreDir(t)
	for _, tc := range []struct{ src, want string }{
		{`GLOBIGNORE='zzz'; shopt dotglob`, "dotglob             \ton\n"},
		{`GLOBIGNORE='zzz'; shopt -u dotglob; printf "[%s]" *`, `[a.txt][b.txt][c.log]`},
		{`GLOBIGNORE='zzz'; unset GLOBIGNORE; shopt dotglob`, "dotglob             \toff\n"},
		{`shopt -s dotglob; unset GLOBIGNORE; shopt dotglob`, "dotglob             \toff\n"},
	} {
		if out, _ := runBash(t, dir, tc.src); out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// TestGlobIgnoreFollowsTheMatchOptionsThatDecideAMiss pins the composition
// with the two `shopt` names that already decide what a word matching nothing
// becomes. The operand matched every `.txt` name in the directory, so this is
// the filter's miss and not the walk's.
func TestGlobIgnoreFollowsTheMatchOptionsThatDecideAMiss(t *testing.T) {
	dir := globIgnoreDir(t)
	for _, tc := range []struct{ src, want string }{
		{`GLOBIGNORE='*.txt'; printf "[%s]" *.txt`, `[*.txt]`},
		{`shopt -s nullglob; GLOBIGNORE='*.txt'; printf "[%s]" *.txt after`, `[after]`},
		{
			"shopt -s failglob\nGLOBIGNORE='*.txt'\nprintf \"[%s]\" *.txt\necho done",
			"bash: line 3: no match: *.txt\ndone\n",
		},
	} {
		if out, _ := runBash(t, dir, tc.src); out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}
