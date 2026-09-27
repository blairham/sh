// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `type -s` and `-S`, which this shell refused at 2 with `-s is not
// implemented yet` while `whence -sv` and `whence -Sv` already wrote the
// lines (#4742). `type` here *is* `whence -v`, measured, so the two letters
// belong to it as well — and the two spellings write the same bytes, which is
// the row that would catch a second rendering drifting from the first.
//
// Measured 2026-09-26 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`,
// aarch64-apple-darwin25.4.0), run `-f` under `env -i PATH=/usr/bin:/bin`,
// over `c -> b -> sub/a` and a plain file beside them.

// runZshType runs src with the link tree on PATH, using the same fixture the
// `whence` rows are measured over so the two builtins cannot be compared
// across two different trees.
func runZshType(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir:  dir,
		Vars: map[string]string{"PATH": dir + ":" + filepath.Join(dir, "linkdir")},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// The two letters are one walk read to two depths. `plain` is the control and
// is load-bearing in both columns: pointed at a command that is a real file
// the letters print exactly what the bare form prints, so a shell with
// neither would agree on that row — and the chain is what tells `-s` from
// `-S`.
func TestTypeResolvesTheLinkACommandIsFoundThrough(t *testing.T) {
	dir := whenceLinkTree(t)
	out, st := runZshType(t, dir, `type -s plain
type -S plain
type plain
type -s one
type -S one
type -s two
type -S two
type two`)
	want := "plain is " + dir + "/plain\n" +
		"plain is " + dir + "/plain\n" +
		"plain is " + dir + "/plain\n" +
		"one is " + dir + "/one -> " + dir + "/real\n" +
		"one is " + dir + "/one -> " + dir + "/real\n" +
		"two is " + dir + "/two -> " + dir + "/real\n" +
		"two is " + dir + "/two -> " + dir + "/one -> " + dir + "/real\n" +
		"two is " + dir + "/two\n"
	if out != want || st != 0 {
		t.Errorf("type -s/-S = %q (status %d), want %q", out, st, want)
	}
}

// **The two spellings write the same bytes.** This is the row the issue is
// about: `type` is `whence -v` here, so a second rendering of the arrow would
// show up as these two lines disagreeing and as nothing else.
func TestTypeAndWhenceVerboseWriteTheSameLinkLine(t *testing.T) {
	dir := whenceLinkTree(t)
	out, st := runZshType(t, dir, `type -s two
whence -sv two
type -S two
whence -Sv two`)
	short := "two is " + dir + "/two -> " + dir + "/real\n"
	long := "two is " + dir + "/two -> " + dir + "/one -> " + dir + "/real\n"
	want := short + short + long + long
	if out != want || st != 0 {
		t.Errorf("type -s beside whence -sv = %q (status %d), want %q", out, st, want)
	}
}

// `-S` wins when both are written, in either order — which is *not* the
// later-letter rule `-t` and `-p` settle by, and is why the two letters are
// read separately rather than ordered.
func TestTypeTheLongerChainWinsWhenBothLettersAreGiven(t *testing.T) {
	dir := whenceLinkTree(t)
	out, st := runZshType(t, dir, `type -sS two
type -Ss two`)
	chain := "two is " + dir + "/two -> " + dir + "/one -> " + dir + "/real\n"
	if want := chain + chain; out != want || st != 0 {
		t.Errorf("type -sS = %q (status %d), want %q", out, st, want)
	}
}

// A link in a **directory** component is a step of its own. A walk that
// resolved only the last component would print this row with no arrow and
// still agree everywhere the directories hold no links, which is most places.
func TestTypeResolvesALinkInTheDirectoryComponent(t *testing.T) {
	dir := whenceLinkTree(t)
	out, st := runZshType(t, dir, `type -s deep
type -S deep`)
	line := "deep is " + dir + "/linkdir/deep -> " + dir + "/realdir/deep\n"
	if want := line + line; out != want || st != 0 {
		t.Errorf("type -s through a linked directory = %q (status %d), want %q", out, st, want)
	}
}

// `-a` carries the letters too: it decides how many rows there are, not what
// a row says. The builtin row is the control — a name that is not a file is
// never decorated.
func TestTypeTheLinkLettersReachEveryRowOfTheListing(t *testing.T) {
	dir := whenceLinkTree(t)
	out, st := runZshType(t, dir, `type -sa two
type -Sa two
type -a two`)
	want := "two is " + dir + "/two -> " + dir + "/real\n" +
		"two is " + dir + "/two -> " + dir + "/one -> " + dir + "/real\n" +
		"two is " + dir + "/two\n"
	if out != want || st != 0 {
		t.Errorf("type -sa = %q (status %d), want %q", out, st, want)
	}
}

// **The arrow goes outside the quoting**, which shows only on a path a
// dialect would quote. Rendering the whole string and handing *that* to the
// sentence quotes the arrow along with the path, and this is the one row that
// can tell the two apart.
func TestTypeQuotesThePathAndNotTheArrow(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "w s")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(dir, "sp")); err != nil {
		t.Fatal(err)
	}
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: dir, Vars: map[string]string{"PATH": dir},
	}, "type -s sp\ntype sp\nwhence -sv sp")
	if err != nil {
		t.Fatal(err)
	}
	want := "sp is '" + dir + "/sp' -> " + dir + "/real\n" +
		"sp is '" + dir + "/sp'\n" +
		"sp is '" + dir + "/sp' -> " + dir + "/real\n"
	if out != want || st != 0 {
		t.Errorf("type -s over a quoted path = %q (status %d), want %q", out, st, want)
	}
}

// Nothing but a file is decorated, and a name PATH does not hold is the
// ordinary not-found line at 1 — the same answers the bare form gives.
func TestTypeTheLinkLettersLeaveEverythingButAFileAlone(t *testing.T) {
	dir := whenceLinkTree(t)
	out, st := runZshType(t, dir, `f() { :; }
alias al=ls
type -s f
type -s al
type -s :
type -S nosuchzz`)
	// The function line names where the function came from, which is this
	// harness's script name rather than anything the link letters decide.
	want := "f is a shell function from zsh\n" +
		"al is an alias for ls\n" +
		": is a shell builtin\n" +
		"nosuchzz not found\n"
	if out != want || st != 1 {
		t.Errorf("type -s over non-files = %q (status %d), want %q at 1", out, st, want)
	}
}
