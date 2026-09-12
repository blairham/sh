// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// `read -k`, which is this shell's letter for reading characters from the
// terminal rather than a line from the stream.
//
// What it *does* is measured in the corpus, where eleven cases grade it
// against the real binary — the count, the defaulting, characters against
// bytes, the newline that terminates nothing, the short read, the single name
// filled and the refusal without a terminal. What is here is the pair of
// tables that decide whether the letter exists at all, because those are this
// package's and a corpus case cannot tell which of the two is wrong.

// The letter is no longer refused as missing.
//
// This is the half that goes stale silently. A letter named in
// UnimplementedOptionLetters *and* accepted in ReadOptions is refused as
// missing while the code behind it works, and a letter in neither is `bad
// option` for something this shell's model of zsh says zsh has. The two tables
// have to move together, and this asserts that they did.
func TestReadKeysIsNoLongerRefusedAsMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(path, []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, st := runZsh(t, dir, `exec 5<`+path+`
read -k 2 -u 5 v
print -r -- "st=$? [$v]"
`)
	if strings.Contains(out, "not implemented yet") {
		t.Fatalf("got %q, want the letter answered rather than named as missing", out)
	}
	if strings.Contains(out, "bad option") {
		t.Fatalf("got %q, want the letter known", out)
	}
	if want := "st=0 [ab]\n"; out != want || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, want)
	}
}

// The letters this shell still has not got are still refused by name, and `-k`
// leaving the list must not have taken one of them with it.
//
// `-q`, `-e`, `-E`, `-z`, `-c` and `-l` are all about a terminal or the line
// editor too, which is exactly why removing one letter from a string of them
// is easy to overshoot.
func TestTheOtherTerminalLettersAreStillRefused(t *testing.T) {
	for _, letter := range []string{"q", "e", "E", "z", "c", "l"} {
		out, st := runZsh(t, t.TempDir(), "read -"+letter+" v\n")
		want := "zsh:read:1: -" + letter + " is not implemented yet\n"
		// Status 2 and not 1: an option this shell has not got stops the
		// script, where the refusals -k itself makes about a number or a
		// terminal are ordinary failures at 1.
		if out != want || st != 2 {
			t.Errorf("-%s = %q status %d, want %q at 2", letter, out, st, want)
		}
	}
}

// Without a terminal the refusal carries no location and no builtin name,
// which is the one complaint this builtin makes that way.
//
// Measured 2026-09-12 against zsh 5.9.2: `printf abc | zsh -c 'read -k v'`
// writes the bare sentence. Every other complaint `read` makes here is located
// — `zsh:read:1: number expected after -k: 2v` a few lines below — so a shell
// that sent this one through the same path would be inventing a prefix the
// shell it is imitating does not print.
func TestReadKeysWithNoTerminalSaysSoWithoutALocation(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "read -k v\nprint -r -- \"st=$?\"\n")
	want := "not interactive and can't open terminal\nst=1\n"
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q", out, st, want)
	}
}

// And a bad attached count is located and named, which is the contrast.
func TestReadKeysBadAttachedCountIsLocated(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "read -k2v x\n")
	want := "zsh:read:1: number expected after -k: 2v\n"
	if out != want || st != 1 {
		t.Errorf("got %q status %d, want %q at 1", out, st, want)
	}
}

// A non-digit after the letter is a letter, not a count, so the bundle carries
// on and an unknown one is refused as an option.
func TestReadKeysDoesNotSwallowALetter(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "read -kv x\n")
	if !strings.Contains(out, "-v") || st == 0 {
		t.Errorf("got %q status %d, want the letter v refused as an option", out, st)
	}
}

// No letter is in both tables at once, for `read` or for any other builtin.
//
// The two are a pair and the pairing is not checked anywhere by construction:
// a letter named in `UnimplementedOptionLetters` while it is *also* in the
// builtin's accepted optstring is simply accepted, because the unimplemented
// list is only ever consulted for a letter the optstring does not have. So the
// stale entry does nothing, says the opposite of the truth, and nothing fails
// — which is exactly how a letter gets implemented and left documented as
// missing.
//
// A mutation run is what asked for this one: putting `k` back in the
// unimplemented list broke nothing at all.
func TestNoLetterIsBothAcceptedAndCalledMissing(t *testing.T) {
	sem, dg := zsh.Semantics(), zsh.Diagnostics()
	accepted := map[string]string{"read": sem.ReadOptions}
	for builtin, missing := range dg.UnimplementedOptionLetters {
		take, known := accepted[builtin]
		if !known {
			continue
		}
		for _, letter := range missing {
			if i := strings.IndexRune(take, letter); i >= 0 {
				t.Errorf("%s: -%c is in the accepted set %q and also called missing in %q",
					builtin, letter, take, missing)
			}
		}
	}
}
