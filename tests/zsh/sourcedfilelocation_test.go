// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// A failure inside a file sourced from a function is located at the file and
// the line within it, not at the enclosing function (#2037).
//
// This dialect is the only one that names a *function* where the others name
// a file, and that rule was applied wherever a function was anywhere below the
// failing line — including while a file the function had sourced was running.
// The innermost frame is what decides: a source frame names its file, a
// function frame names the function.
//
// Measured on zsh 5.9.2, 2026-09-11, with `/tmp/fp3/inc` holding two lines and
// a function `q` sourcing it:
//
//	zsh -f rc    /tmp/fp3/inc:2: command not found: zznotacommand
//	ours         q:1: command not found: zznotacommand
//
// It is worth more than its size: sourcing a file from inside a function is
// what every plugin manager does, so every diagnostic from anywhere inside a
// plugin named the loader instead of the plugin's own file and line.
func TestAFailureInAFileSourcedFromAFunctionNamesTheFile(t *testing.T) {
	t.Run("the file the function sourced", func(t *testing.T) {
		dir := sourceFixture(t, map[string]string{"inc.zsh": "nosuchcmd_zz\n"})
		src := "f() {\n  . ./inc.zsh\n}\nf\nprint \"st=$?\"\n"
		out, _ := runZsh(t, dir, src)
		wantWholeLines(t, out, "./inc.zsh:1: command not found: nosuchcmd_zz", "st=127")
	})

	t.Run("a file sourced by a file the function sourced", func(t *testing.T) {
		// Two source frames above the function's, which is what says the
		// rule reads the innermost frame rather than asking whether any
		// sourced file is running.
		dir := sourceFixture(t, map[string]string{
			"outer.zsh": ". ./inner.zsh\n",
			"inner.zsh": "nosuchcmd_zz\n",
		})
		src := "f() {\n  . ./outer.zsh\n}\nf\n"
		out, _ := runZsh(t, dir, src)
		wantWholeLines(t, out, "./inner.zsh:1: command not found: nosuchcmd_zz")
	})

	t.Run("a function defined in a sourced file is still a function", func(t *testing.T) {
		// The other direction, and the reason the question is about where
		// the failing line is being read from rather than about where the
		// function came from: `g` was read out of a file and the location is
		// still `g` and the offset into it.
		dir := sourceFixture(t, map[string]string{"def.zsh": "g() {\n  nosuchcmd_zz\n}\n"})
		src := "f() {\n  . ./def.zsh\n  g\n}\nf\n"
		out, _ := runZsh(t, dir, src)
		wantWholeLines(t, out, "g:1: command not found: nosuchcmd_zz")
	})

	t.Run("the top-level control is unchanged", func(t *testing.T) {
		// The same file sourced with no function anywhere, which was already
		// byte-identical to zsh and is what says the fix moved one rule
		// rather than the naming of a sourced file.
		dir := sourceFixture(t, map[string]string{"inc.zsh": "nosuchcmd_zz\n"})
		out, _ := runZsh(t, dir, ". ./inc.zsh\n")
		wantWholeLines(t, out, "./inc.zsh:1: command not found: nosuchcmd_zz")
	})

	t.Run("a failing line the function does hold still names the function", func(t *testing.T) {
		// The control on the other side: nothing is sourced, so the
		// function frame is innermost and this dialect's own rule applies —
		// the function's name and the offset from the line it was written on.
		out, _ := runZsh(t, t.TempDir(), "f() {\n  nosuchcmd_zz\n}\nf\n")
		wantWholeLines(t, out, "f:1: command not found: nosuchcmd_zz")
	})
}

// `$LINENO` inside such a file counts from the file, for the same reason.
//
// This dialect numbers a function's lines from the line the function was
// written on, and that too was decided by the name of the function the shell
// happened to be in. Measured on zsh 5.9.2: the two lines of a file a function
// sourced are 1 and 2, and the line after the `source` is the function's own
// count again.
func TestLinenoInAFileSourcedFromAFunctionCountsFromTheFile(t *testing.T) {
	dir := sourceFixture(t, map[string]string{"lin.zsh": "echo a=$LINENO\necho b=$LINENO\n"})
	src := "f() {\n  . ./lin.zsh\n  echo c=$LINENO\n}\nf\n"
	out, _ := runZsh(t, dir, src)
	if want := "a=1\nb=2\nc=2\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// sourceFixture writes files into a scratch directory and hands it back, for
// the cases that need a file to source. The names are what the snippet writes
// in its `.` operand, so they are also what the location has to report.
func sourceFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}
