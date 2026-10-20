// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `hash` over the two tables this shell keeps, measured 2026-09-26 on zsh
// 5.9.2 (`/opt/homebrew/bin/zsh`), run `-f` under `env -i PATH=/usr/bin:/bin`.
// `hash name=value` was `no such command: name=value` at 1 (#4456) and
// `hash -m` was `-m is not implemented yet` (#4445).

// hashTool writes an executable into dir and returns its path, so a row can
// name a file whose path it knows without depending on the machine.
func hashTool(t *testing.T, dir, name string) string {
	t.Helper()
	at := filepath.Join(dir, name)
	if err := os.WriteFile(at, []byte("#!/bin/sh\nprintf 'ran %s\\n' "+name+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return at
}

// runZshHash runs src with dir on PATH and nothing else, so the table holds
// only what the snippet put there.
func runZshHash(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: dir, Vars: map[string]string{"PATH": dir},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// `hash name=value` writes the command table, and the name need not be the
// base name of the file it points at. The **run** is the half the status
// cannot show: a shell that took the operand and recorded nothing would
// answer 0 here and leave the name unrunnable.
func TestHashWritesTheCommandTableFromAnAssignment(t *testing.T) {
	dir := t.TempDir()
	tool := hashTool(t, dir, "zzc")
	out, st := runZshHash(t, dir, `hash foo=`+tool+`
print -r -- "st=$?"
foo
hash`)
	want := "st=0\nran zzc\nfoo=" + tool + "\n"
	if out != want || st != 0 {
		t.Errorf("hash name=value = %q (status %d), want %q", out, st, want)
	}
}

// Neither half is checked: the value is taken as written, and a word that is
// not a path at all goes in and comes back out at 0. That is what makes this
// a write of the table rather than a search that happened to succeed.
func TestHashTakesAnAssignmentsValueAsWritten(t *testing.T) {
	dir := t.TempDir()
	out, st := runZshHash(t, dir, `hash foo=notapath
print -r -- "st=$?"
hash a/b=c
hash`)
	want := "st=0\na/b=c\nfoo=notapath\n"
	if out != want || st != 0 {
		t.Errorf("hash foo=notapath = %q (status %d), want %q", out, st, want)
	}
}

// An operand with no `=` in it is unchanged, and the one thing that still
// refuses is the written pathname — measured, `hash /bin/ls` is `no such
// command` here where the assignment beside it is silence. The pair is what
// says the `=` is being read rather than the slash.
func TestHashStillRefusesAWrittenPathnameWithNoEquals(t *testing.T) {
	dir := t.TempDir()
	tool := hashTool(t, dir, "zzc")
	out, st := runZshHash(t, dir, `hash `+tool+` 2>&1
print -r -- "path=$?"
hash zzc
print -r -- "name=$?"
hash`)
	want := "zsh:hash:1: no such command: " + tool + "\npath=1\nname=0\nzzc=" + tool + "\n"
	if out != want || st != 0 {
		t.Errorf("hash with a written pathname = %q (status %d), want %q", out, st, want)
	}
}

// `hash -m` reads the operands as patterns over the command table: per
// pattern, and sorted within each, which is the order the operand list is in
// rather than the one sorted run the bare listing writes.
func TestHashDashMListsTheCommandTableByPattern(t *testing.T) {
	dir := t.TempDir()
	out, st := runZshHash(t, dir, `hash foo=/bin/ls bar=/bin/cat baz=/bin/pwd
hash -m "f*"
print -r -- "one=$?"
hash -m "f*" "b*"
print -r -- "two=$?"
hash -m "nothinglike*"
print -r -- "none=$?"`)
	want := "foo=/bin/ls\none=0\n" +
		"foo=/bin/ls\nbar=/bin/cat\nbaz=/bin/pwd\ntwo=0\n" +
		"none=0\n"
	if out != want || st != 0 {
		t.Errorf("hash -m = %q (status %d), want %q", out, st, want)
	}
}

// The same letter over the other table, which is the repro the issue reduces
// to — `-d` fills it in the same snippet, so the case has something in it.
// `hash -m 'l*'` on a shell that has hashed nothing prints nothing here *and*
// under a shell with no such letter, which is a pass an unimplemented option
// would also produce.
func TestHashDashMListsTheNamedDirectoriesByPattern(t *testing.T) {
	dir := t.TempDir()
	out, st := runZshHash(t, dir, `hash -d foo=/tmp bar=/usr
hash -d -m "f*"
print -r -- "list=$?"
hash -dLm "f*"
print -r -- "commands=$?"
hash -dm "nosuch*"
print -r -- "none=$?"`)
	want := "foo=/tmp\nlist=0\nhash -d foo=/tmp\ncommands=0\nnone=0\n"
	if out != want || st != 0 {
		t.Errorf("hash -d -m = %q (status %d), want %q", out, st, want)
	}
}

// `-m` is a listing and never a definition, and it writes nothing at all when
// nothing was named — which is where it parts from the bare listing. A shell
// that read an empty operand list as "everything" would write the table on
// the second row and agree with the reference nowhere else.
func TestHashDashMIsAListingAndNeverADefinition(t *testing.T) {
	dir := t.TempDir()
	out, st := runZshHash(t, dir, `hash -m foo=/bin/ls
print -r -- "assign=$?"
hash
print -r -- "afterwards=$?"
hash real=/bin/ls
hash -m
print -r -- "bare=$?"
hash -d -m
print -r -- "dirs=$?"`)
	want := "assign=0\nafterwards=0\nbare=0\ndirs=0\n"
	if out != want || st != 0 {
		t.Errorf("hash -m as a definition = %q (status %d), want %q", out, st, want)
	}
}

// The letters that have landed are no longer named as missing, and the ones
// this shell still has not got are — so a script can go on telling the two
// apart. `-t` is the third wording: zsh has no `-t` for `hash` at all, which
// is why it is `bad option` and 1 rather than the bad-option status the other
// two carry.
//
// `-v` was the middle row until #4964 and is now a working listing at 0; `-L`
// took its place, which is the pairing doing its job rather than the test
// being weakened — a letter leaves this row on the change that implements it
// and the row has to hold a letter that is really still absent, or it is
// three assertions about nothing.
func TestHashStillNamesTheLettersItHasNotGot(t *testing.T) {
	dir := t.TempDir()
	out, st := runZshHash(t, dir, `hash -f 2>&1
print -r -- "f=$?"
hash -L 2>&1
print -r -- "L=$?"
hash -t zzc 2>&1
print -r -- "t=$?"`)
	// `-f` has left the refused set too (#5266): it fills the table and
	// lists nothing, at 0, as zsh 5.9.2 does.
	want := "f=0\n" +
		"zsh:hash:3: -L is not implemented yet\nL=2\n" +
		"zsh:hash:5: bad option: -t\nt=1\n"
	if out != want || st != 0 {
		t.Errorf("the refused letters = %q (status %d), want %q", out, st, want)
	}
}
