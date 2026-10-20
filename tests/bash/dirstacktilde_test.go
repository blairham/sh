// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// This shell's numbered tilde, over the stack its own `pushd` builds.
//
// The interp package pins the rule against a stack a test supplies; this pins
// the *wiring* — that the parameter the dialect names is the view the prelude
// keeps, so `pushd` and `~1` are talking about one stack. Measured 2026-09-22
// on bash 5.3.20.
func TestANumberedTildeReadsThisShellsDirectoryStack(t *testing.T) {
	home := t.TempDir()
	out, st := runBashPrelude(t, home, `HOME=`+home+`
cd
pushd / >/dev/null; pushd /tmp >/dev/null
echo ~0 ~1 ~2
echo ~+0 ~+1
echo ~-0 ~-1 ~-2
echo ~3 ~-3 ~1a
echo ~1/x`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	wantWholeLines(t, out,
		"/tmp / "+home,
		"/tmp /",
		home+" / /tmp",
		"~3 ~-3 ~1a",
		"//x",
	)
}

// And `dirs -v` numbers the one entry an index picks, with that entry's own
// index rather than a zero this line started.
func TestDirsNumbersASingleEntryUnderTheVerboseLetter(t *testing.T) {
	home := t.TempDir()
	out, st := runBashPrelude(t, home, `HOME=`+home+`
cd
pushd / >/dev/null; pushd /tmp >/dev/null
dirs -v +1
dirs -v -1
dirs -v +0
dirs -p +1`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	wantWholeLines(t, out, " 1  /", " 0  /tmp", "/")
}
