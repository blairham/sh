// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// A login shell reads a file on its way *out* — bash's `~/.bash_logout`, and
// nothing else in the panel has one that a shell without a terminal can reach
// (#4159).
//
// What reaches it is the `exit` builtin having run, which is a narrower thing
// than the shell stopping. Measured 2026-09-22 against bash 5.3.20 with a
// marker in the file: `-lc 'exit 3'` reads it, `-lc '. f.sh'` whose sourced
// file exits reads it, and `-lc 'echo x'`, `-lc 'set -e; false'` and a line
// that will not parse read nothing — all four of those stop the shell too.
func TestALoginShellReadsItsLogoutFileWhenExitRan(t *testing.T) {
	home := t.TempDir()
	writeAt(t, filepath.Join(home, ".logout"), "echo LEAVING\n")
	sourced := filepath.Join(home, "f.sh")
	writeAt(t, sourced, "exit 3\n")

	for _, tc := range []struct {
		name  string
		login bool
		src   string
		read  bool
	}{
		{"exit, in a login shell", true, "exit 3", true},
		{"exit in a sourced file", true, ". " + sourced, true},
		{"the program running out", true, "echo x", false},
		{"a shell that is not a login shell", false, "exit 3", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh := shell()
			sh.Semantics.LogoutFile = ".logout"
			sh.Env = []string{"HOME=" + home}
			// A dashed argv[0] rather than the `-l` letter, because
			// login-ness is a fact about the vector and this front end's
			// test shell has no option namespace to spell the letter in.
			// bash reads the file on both routes — measured through `exec -a
			// -bash`.
			name := "testsh"
			if tc.login {
				name = "-testsh"
			}
			argv := []string{name, "-c", tc.src}
			out, errs, _ := runArgs(t, sh, argv...)
			if got := strings.Contains(out, "LEAVING"); got != tc.read {
				t.Errorf("read = %v, want %v — stdout %q stderr %q", got, tc.read, out, errs)
			}
		})
	}
}

// The status the shell was leaving with stands, and the file's own `exit`
// replaces it. Measured both ways: `-lc 'exit 3'` with an `echo` in the file
// is still 3, and with `exit 9` in the file it is 9.
func TestTheLogoutFileDoesNotTakeTheStatusUnlessItSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"a file that says nothing about it", "echo LEAVING\n", 3},
		{"a file that names one", "echo LEAVING\nexit 9\n", 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeAt(t, filepath.Join(home, ".logout"), tc.body)
			sh := shell()
			sh.Semantics.LogoutFile = ".logout"
			sh.Env = []string{"HOME=" + home}
			out, _, code := runArgs(t, sh, "-testsh", "-c", "exit 3")
			if !strings.Contains(out, "LEAVING") {
				t.Fatalf("stdout = %q, want the file read", out)
			}
			if code != tc.want {
				t.Errorf("status %d, want %d", code, tc.want)
			}
		})
	}
}

// Before the EXIT trap, which is the measured order: a login shell with both
// writes the file's output and then the trap's.
func TestTheLogoutFileRunsBeforeTheExitTrap(t *testing.T) {
	home := t.TempDir()
	writeAt(t, filepath.Join(home, ".logout"), "echo LEAVING\n")
	sh := shell()
	sh.Semantics.LogoutFile = ".logout"
	// The neighbor this test needs answered so it reaches its own question:
	// what a trap body does with text that only partly parsed is an axis, and
	// this one's body parses whole.
	sh.Semantics.TrapBodyRunsWhatParsed = interp.No
	sh.Env = []string{"HOME=" + home}
	out, _, _ := runArgs(t, sh, "-testsh", "-c", `trap 'echo TRAP' EXIT; exit 3`)
	if out != "LEAVING\nTRAP\n" {
		t.Errorf("stdout = %q, want the file and then the trap", out)
	}
}
