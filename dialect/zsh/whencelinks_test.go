// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `whence -s` and `-S`, measured 2026-09-26 on zsh 5.9.2
// (`/opt/homebrew/bin/zsh`, aarch64-apple-darwin25), run `-f` with the links
// below built under a path holding no links of its own. Both letters answered
// `-s is not implemented yet` here (#4446).

// whenceLinkTree builds the shape every row below is measured over: a real
// executable, a link to it, a link to that link, a plain executable that is
// nobody's link, and the same command reached through a **directory** that is
// a link.
//
// The temp directory is resolved first. `t.TempDir()` on this platform sits
// under `/var`, which is itself a link to `/private/var`, so an unresolved
// base would put a step of the machine's own into every expected line — and a
// row whose expectation is built from the same unresolved string would still
// pass while measuring the wrong thing.
func whenceLinkTree(t *testing.T) (dir string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	link := func(target, name string) {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "realdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "realdir", "deep"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	write("real")
	write("plain")
	link("real", "one")
	link("one", "two")
	link("realdir", "linkdir")
	return dir
}

// runZshLinks runs src with the link tree's two directories on PATH.
func runZshLinks(t *testing.T, dir, src string) (string, int) {
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

// The two letters are one walk read to two depths, and the three rows are
// what tell them apart. `plain` is the control: pointed at a command that is
// a real file both letters print what the bare form prints, so a shell with
// neither letter would agree on that row and on nothing else.
func TestWhenceResolvesTheLinkACommandIsFoundThrough(t *testing.T) {
	dir := whenceLinkTree(t)
	out, st := runZshLinks(t, dir, `whence -s plain
whence -S plain
whence plain
whence -s one
whence -S one
whence -s two
whence -S two
whence two`)
	want := dir + "/plain\n" + dir + "/plain\n" + dir + "/plain\n" +
		dir + "/one -> " + dir + "/real\n" +
		dir + "/one -> " + dir + "/real\n" +
		dir + "/two -> " + dir + "/real\n" +
		dir + "/two -> " + dir + "/one -> " + dir + "/real\n" +
		dir + "/two\n"
	if out != want || st != 0 {
		t.Errorf("whence -s/-S = %q (status %d), want %q", out, st, want)
	}
}

// A link in a **directory** component is a step of its own, and it comes
// before whatever the last component is. A walk that resolved only the final
// component would print the row below with no arrow at all and would still
// agree with the reference on every case where the directories hold no links
// — which is most of them, and is why this row is here.
func TestWhenceResolvesALinkInTheDirectoryComponent(t *testing.T) {
	dir := whenceLinkTree(t)
	out, st := runZshLinks(t, dir, `whence -s deep
whence -S deep`)
	want := dir + "/linkdir/deep -> " + dir + "/realdir/deep\n" +
		dir + "/linkdir/deep -> " + dir + "/realdir/deep\n"
	if out != want || st != 0 {
		t.Errorf("whence -s through a linked directory = %q (status %d), want %q", out, st, want)
	}
}

// `-S` wins when both are given, in either order.
func TestWhenceTheLongerChainWinsWhenBothLettersAreGiven(t *testing.T) {
	dir := whenceLinkTree(t)
	out, st := runZshLinks(t, dir, `whence -sS two
whence -Ss two`)
	chain := dir + "/two -> " + dir + "/one -> " + dir + "/real\n"
	want := chain + chain
	if out != want || st != 0 {
		t.Errorf("whence -sS = %q (status %d), want %q", out, st, want)
	}
}

// `-w` is not this question. Measured — the kind word is never decorated,
// whichever of the two letters stands beside it.
func TestWhenceTheKindWordIsNotDecoratedByTheLinkLetters(t *testing.T) {
	dir := whenceLinkTree(t)
	out, st := runZshLinks(t, dir, `whence -sw two
whence -Sw two
whence -w two`)
	want := "two: command\ntwo: command\ntwo: command\n"
	if out != want || st != 0 {
		t.Errorf("whence -sw = %q (status %d), want %q", out, st, want)
	}
}

// The other three shapes each write the arrow: the bare form above, `-p`,
// `-c` — which is what `which` and `where` are — and `-v`, which is `type`.
func TestWhenceTheLinkLettersReachEveryShapeThatWritesAPath(t *testing.T) {
	dir := whenceLinkTree(t)
	out, st := runZshLinks(t, dir, `whence -sp two
whence -sc two
which -s two
where -S two`)
	want := dir + "/two -> " + dir + "/real\n" +
		dir + "/two -> " + dir + "/real\n" +
		dir + "/two -> " + dir + "/real\n" +
		dir + "/two -> " + dir + "/one -> " + dir + "/real\n"
	if out != want || st != 0 {
		t.Errorf("the link letters over the shapes = %q (status %d), want %q", out, st, want)
	}
}

// The `-v` sentence puts the arrow **after** it rather than inside the path
// it names, which shows only on a path a dialect would quote. Measured on a
// directory with a space in its name:
//
//	sp is '/…/w s/sp' -> /bin/ls
//
// Rendering the whole string first and handing that to the sentence quoted
// the arrow along with the path, which is what this row catches.
func TestWhenceTheVerboseSentenceQuotesThePathAndNotTheArrow(t *testing.T) {
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
	}, "whence -sv sp\nwhence -v sp\nwhence -s sp")
	if err != nil {
		t.Fatal(err)
	}
	want := "sp is '" + dir + "/sp' -> " + dir + "/real\n" +
		"sp is '" + dir + "/sp'\n" +
		dir + "/sp -> " + dir + "/real\n"
	if out != want || st != 0 {
		t.Errorf("whence -sv over a quoted path = %q (status %d), want %q", out, st, want)
	}
}

// A name that is not a file never reaches the resolution, and a name PATH
// does not hold is the ordinary silence at 1.
func TestWhenceTheLinkLettersLeaveEverythingButAFileAlone(t *testing.T) {
	dir := whenceLinkTree(t)
	out, st := runZshLinks(t, dir, `f() { :; }
alias al=ls
whence -s f
whence -s al
whence -s :
whence -s nosuchcommand431
print -r -- "missing=$?"`)
	want := "f\nls\n:\nmissing=1\n"
	if out != want || st != 0 {
		t.Errorf("the link letters on a name that is not a file = %q (status %d), want %q", out, st, want)
	}
}

// The letters are on all three names and are no longer refused by any of
// them. `-m` was the row that stayed refused as missing until #5230 built it;
// it is answered now, and over the link tree it lists `two` bare — the table's
// rows draw no arrow, measured.
//
// **`-x` is no longer on this list** and the row for it has become the
// opposite: it is answered, and what it answers with is the body indented by
// the number it was given. Keeping it here as a refusal would have pinned the
// gap rather than the behavior — see whenceindent_test.go, and
// interp.Runner.FunctionBodyIndentOption for the reader the four names share.
func TestWhenceStillNamesTheLettersItHasNotGot(t *testing.T) {
	dir := whenceLinkTree(t)
	out, st := runZshLinks(t, dir, `whence -m "tw*" 2>&1
print -r -- "m=$?"
whence -x 2 -f two 2>&1
print -r -- "x=$?"
whence -z two 2>&1
print -r -- "z=$?"`)
	want := dir + "/two\nm=0\n" +
		dir + "/two\nx=0\n" +
		"zsh:whence:5: bad option: -z\nz=1\n"
	if out != want || st != 0 {
		t.Errorf("the refused letters = %q (status %d), want %q", out, st, want)
	}
}
