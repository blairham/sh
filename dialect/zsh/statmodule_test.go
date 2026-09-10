// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// `zstat`, measured against zsh 5.9.2 (2026-09-09) with `zsh -f`.
//
// The elements a machine chooses — the device, the inode, the three times —
// are deliberately not asserted on: they are different on every run and in
// every checkout, and a test that pinned them would be pinning the tree it was
// written in. What is asserted is everything the *command* decides: which
// elements there are and in what order, how each is written, when a name and a
// type appear beside it, and what a failure says.

// statTree is a directory with one file of known size and mode, a second empty
// one, and a symbolic link to the first.
func statTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "g"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("f", filepath.Join(dir, "l")); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The fourteen elements, and the four ways one of them can be written.
//
// This is the test a hollow module fails. `zmodload -F zsh/stat b:zstat`
// answering 0 costs nothing to fake; a `zstat` that reports six bytes, a mode
// of `-rw-------` and the same mode as `0100600` is doing the system call.
func TestZstatReportsTheElementsOfAFile(t *testing.T) {
	out, st := runZsh(t, statTree(t), `zstat -A a -- f
print -r -- "n=$#a"
zstat +size -- f
zstat -s +mode -- f
zstat -o +mode -- f
zstat -r +mode -- f
zstat +nlink -- f`)
	want := "n=14\n6\n-rw-------\n0100600\n33152 (-rw-------)\n1\n"
	if out != want || st != 0 {
		t.Errorf("zstat = %q (status %d), want %q", out, st, want)
	}
}

// A whole listing names each element in a column seven wide, which is the
// width of the longest of them.
func TestZstatWritesAWholeListingInAColumn(t *testing.T) {
	out, st := runZsh(t, statTree(t), `zstat -- f`)
	if st != 0 {
		t.Fatalf("zstat status %d, output %q", st, out)
	}
	wantWholeLines(t, out, "size    6", "nlink   1", "blocks  8", "link    ")
}

// The names of the fourteen, which `-l` answers without looking at a file at
// all — to standard output, or into the array `-A` names.
func TestZstatDashLListsTheElementNames(t *testing.T) {
	out, st := runZsh(t, statTree(t), `zstat -l
zstat -l -A names
print -r -- "n=$#names first=$names[1] last=$names[-1]"`)
	want := "device inode mode nlink uid gid rdev size atime mtime ctime blksize blocks link\n" +
		"n=14 first=device last=link\n"
	if out != want || st != 0 {
		t.Errorf("zstat -l = %q (status %d), want %q", out, st, want)
	}
}

// **A name is shown when there is more than one file and the answer is going
// to standard output**, and the two letters override that either way. A type
// name is shown when no element was selected, and the same two letters do the
// same for it.
func TestZstatShowsNamesAndTypesWhenThereIsSomethingToTellApart(t *testing.T) {
	out, st := runZsh(t, statTree(t), `zstat +size -- f
zstat +size -- f g
zstat -N +size -- f g
zstat -n +size -- f
zstat -t +size -- f
zstat -n -t +size -- f`)
	want := "6\nf 6\ng 0\n6\n0\nf 6\nsize 6\nf size 6\n"
	if out != want || st != 0 {
		t.Errorf("names and types = %q (status %d), want %q", out, st, want)
	}
}

// An element may be shortened to any unique leading part, and `m` is not one:
// `mode` and `mtime` both begin with it.
func TestZstatResolvesAShortenedElementOrSaysWhyItCannot(t *testing.T) {
	out, st := runZsh(t, statTree(t), `zstat +si -- f
print -r -- "short=$?"
zstat +m -- f 2>&1
print -r -- "ambiguous=$?"
zstat +nosuch -- f 2>&1
print -r -- "unknown=$?"`)
	want := "6\nshort=0\n" +
		"zsh:zstat:3: m: ambiguous stat element\nambiguous=1\n" +
		"zsh:zstat:5: nosuch: no such stat element\nunknown=1\n"
	if out != want || st != 0 {
		t.Errorf("element names = %q (status %d), want %q", out, st, want)
	}
}

// **A file that cannot be statted leaves the array alone**, which is the
// difference between an answer and half of one: a script writing `zstat -A
// stat +mtime -- $dirs || return` reads the array only when every name in the
// list was answered.
func TestZstatLeavesTheArrayAloneWhenAFileFails(t *testing.T) {
	out, st := runZsh(t, statTree(t), `zstat -A a +mtime -- f nosuch 2>&1
print -r -- "st=$? n=$#a"`)
	want := "zsh:zstat:1: nosuch: no such file or directory\nst=1 n=0\n"
	if out != want || st != 0 {
		t.Errorf("a failed file = %q (status %d), want %q", out, st, want)
	}
}

// A link is followed unless `-L` says not to, which is the opposite way round
// from `ls`, and the `link` element is the one that makes the difference
// visible. Selecting it turns `-L` on by itself.
func TestZstatFollowsALinkUnlessTheQuestionIsAboutTheLink(t *testing.T) {
	out, st := runZsh(t, statTree(t), `zstat +size -- l
zstat -L +size -- l
zstat -L +link -- l
zstat +link -- l
print -r -- "empty=[$(zstat -L +link -- f)]"`)
	want := "6\n1\nf\nf\nempty=[]\n"
	if out != want || st != 0 {
		t.Errorf("a symbolic link = %q (status %d), want %q", out, st, want)
	}
}

// `-H` fills an association, and one file is all it will take.
func TestZstatFillsAnAssociationForOneFile(t *testing.T) {
	out, st := runZsh(t, statTree(t), `zstat -H h -- f
print -r -- "size=$h[size] mode=$h[mode] link=[$h[link]]"
zstat -H h2 -n +size -- f
print -r -- "name=$h2[name] size=$h2[size]"
zstat -H h3 -- f g 2>&1
print -r -- "two=$?"`)
	want := "size=6 mode=33152 link=[]\n" +
		"name=f size=6\n" +
		"zsh:zstat:5: only one file allowed with -H\ntwo=1\n"
	if out != want || st != 0 {
		t.Errorf("zstat -H = %q (status %d), want %q", out, st, want)
	}
}

// `-f` asks about a descriptor this shell has open rather than about a name,
// and refuses a list of names beside it.
func TestZstatReadsADescriptorInsteadOfANameWithDashF(t *testing.T) {
	out, st := runZsh(t, statTree(t), `exec 7< f
zstat -f 7 +size
print -r -- "st=$?"
zstat -f 7 +size f 2>&1
print -r -- "both=$?"
zstat -f 33 +size 2>&1
print -r -- "closed=$?"`)
	want := "6\nst=0\n" +
		"zsh:zstat:4: no files allowed with -f\nboth=1\n" +
		"zsh:zstat:6: 33: bad file descriptor\nclosed=1\n"
	if out != want || st != 0 {
		t.Errorf("zstat -f = %q (status %d), want %q", out, st, want)
	}
}

// What the command says when it was given nothing it can use.
func TestZstatRefusesWhatItCannotRead(t *testing.T) {
	out, st := runZsh(t, statTree(t), `zstat 2>&1
print -r -- "none=$?"
zstat -Q f 2>&1
print -r -- "letter=$?"
zstat -A 2>&1
print -r -- "missing=$?"
zstat -H h -A a f 2>&1
print -r -- "both=$?"`)
	want := "zsh:zstat:1: no files given\nnone=1\n" +
		"zsh:zstat:3: bad option: -Q\nletter=1\n" +
		"zsh:zstat:5: missing parameter name\nmissing=1\n" +
		"zsh:zstat:7: both array and hash requested\nboth=1\n"
	if out != want || st != 0 {
		t.Errorf("zstat refusals = %q (status %d), want %q", out, st, want)
	}
}

// The three times are written as numbers unless something asks for words, and
// `-F` and `-g` are two of the things that ask — the format language being the
// one `strftime` speaks here, so `%s` comes back as the number it started as.
func TestZstatWritesATimeAsWordsWhenAskedTo(t *testing.T) {
	out, st := runZsh(t, statTree(t), `raw=$(zstat +mtime -- f)
same=$(zstat -F '%s' +mtime -- f)
print -r -- "same=$(( raw == same ))"
gmt=$(zstat -g -F '%Y-%m-%d' +mtime -- f)
print -r -- "shape=$(( ${#gmt} == 10 ))"`)
	want := "same=1\nshape=1\n"
	if out != want || st != 0 {
		t.Errorf("zstat times = %q (status %d), want %q", out, st, want)
	}
}
