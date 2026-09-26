// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// runZshHome is runZsh with the case's own directory as `$HOME` as well as as
// the working directory, so that a case about `~` never reads the machine's.
func runZshHome(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: dir, Vars: map[string]string{"PATH": dir, "HOME": dir},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// `print -D` writes each operand back with its leading directory replaced by
// the `~name` the shell was told stands for it. Measured 2026-09-26 on zsh
// 5.9.2 (aarch64-apple-darwin25.4.0), run `-f` with `env -u FPATH` over a
// script file (#4444).
//
// Every case below sets the entry and reads it back in the same run, which is
// the half that makes the test discriminating: with nothing in the table
// `print -D /tmp` is `/tmp` in the reference too, so a case without the
// `hash -d` would agree for the wrong reason.
func TestPrintNamedDirectory(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `hash -d foo=/tmp
print -D /tmp/x
print -D /tmp
print -D /tmpx
print -D /var /tmp/a/b`)
	// The third row is the boundary: `/tmp` is not a directory of `/tmpx`,
	// however much of the spelling the two share.
	want := "~foo/x\n~foo\n/tmpx\n/var ~foo/a/b\n"
	if out != want || st != 0 {
		t.Errorf("print -D = %q (status %d), want %q", out, st, want)
	}
}

// Which entry wins, which is three separate rules and none of them obvious.
func TestPrintNamedDirectoryChoosesTheEntry(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `hash -d foo=/tmp
hash -d bar=/tmp/a
print -D /tmp/a/b
hash -d zz=/tmp/c
hash -d aa=/tmp/c
print -D /tmp/c
hash -d ccc=/tmp/d bb=/tmp/d dd=/tmp/d
print -D /tmp/d`)
	// The deepest directory first; then the *shortest* name, which is
	// neither the order they were written in — `zz` was written first and
	// lost — nor the order a listing writes them, since `bb` beats `ccc`
	// though the listing puts `bb` after it.
	want := "~bar/b\n~aa\n~bb\n"
	if out != want || st != 0 {
		t.Errorf("print -D over several entries = %q (status %d), want %q", out, st, want)
	}
}

// Home beats a name for the same directory, and beats nothing deeper.
func TestPrintNamedDirectoryAndHome(t *testing.T) {
	dir := t.TempDir()
	out, st := runZshHome(t, dir, `hash -d h=$HOME
print -D $HOME/sub
hash -d s=$HOME/sub
print -D $HOME/sub/deep`)
	want := "~/sub\n~s/deep\n"
	if out != want || st != 0 {
		t.Errorf("print -D beside HOME = %q (status %d), want %q", out, st, want)
	}
}

// Where in the command it happens: after the pattern and before the sort.
// Both halves are measured, and each rules out the other arrangement — the
// pattern matches the operand as it was written, and the order the sort
// produces is one only the abbreviated spellings have.
func TestPrintNamedDirectoryHappensBetweenTheMatchAndTheSort(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `hash -d foo=/tmp
print -D -o /var /tmp/x
print -D -m '/tmp*' /tmp/x /var
print -D -m '~foo*' /tmp/x /var
print -D -f '[%s]\n' /tmp/x`)
	want := "/var ~foo/x\n~foo/x\n\n[~foo/x]\n"
	if out != want || st != 0 {
		t.Errorf("print -D beside the other letters = %q (status %d), want %q", out, st, want)
	}
}

// And the letter is accepted rather than named as missing.
func TestPrintNamedDirectoryLetterIsNotAlsoCalledMissing(t *testing.T) {
	if got := zsh.Diagnostics().UnimplementedOptionLetters["print"]; strings.ContainsRune(got, 'D') {
		t.Errorf("UnimplementedOptionLetters[print] = %q, which still claims -D is missing", got)
	}
}
