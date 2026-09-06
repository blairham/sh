// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// The ties this shell arrives with, measured against zsh 5.9.2 (2026-09-06)
// under `env -i` with a scratch HOME and no startup files. The machinery is
// `typeset -T`; these tests are about the eight pairs being tied before a
// script says anything.

// Every pair, in both directions, on names whose real values are the
// machine's — so each is set by the test first.
//
// Counted with an explicit `[@]` throughout, because `${#name}` on a *one*
// element array answers the scalar's length here rather than the count —
// #1097, which is the array store's own bug and nothing to do with a tie.
func TestTheEightBuiltInPairsAreTied(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `CDPATH=a:b
print -r -- "cdpath n=${#cdpath[@]} [${(j:|:)cdpath}]"
cdpath=(x y)
print -r -- "CDPATH=[$CDPATH]"
MANPATH=m1:m2
PSVAR=p1:p2
FIGNORE=f1:f2
MODULE_PATH=mp
MAILPATH=q1:q2
FPATH=/x:/y
print -r -- "${#manpath[@]} ${#psvar[@]} ${#fignore[@]} ${#module_path[@]} ${#mailpath[@]} ${#fpath[@]}"
psvar=(A B C)
print -r -- "PSVAR=[$PSVAR]"`)
	want := "cdpath n=2 [a|b]\nCDPATH=[x:y]\n2 2 2 1 2 2\nPSVAR=[A:B:C]\n"
	if out != want || st != 0 {
		t.Errorf("the built-in ties = %q (status %d), want %q", out, st, want)
	}
}

// The line every rc file writes, and the reason the ties are here: writing
// `path` puts a directory on the search path, and the *lookup* follows. Two
// executables of the same name in two directories is what proves the second
// half — a `$PATH` that merely reads right could still be resolving from the
// old one.
func TestWritingPathMovesWhereACommandIsFound(t *testing.T) {
	dir := t.TempDir()
	one, two := filepath.Join(dir, "one"), filepath.Join(dir, "two")
	for _, d := range []string{one, two} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("#!/bin/sh\necho "+text+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(one, "whichone"), "FROM_ONE")
	write(filepath.Join(two, "whichone"), "FROM_TWO")
	out, st := runZsh(t, one, `whichone
path=(`+two+` "${path[@]}")
print -r -- "PATH=[${PATH%%:*}]"
whichone`)
	want := "FROM_ONE\nPATH=[" + two + "]\nFROM_TWO\n"
	if out != want || st != 0 {
		t.Errorf("a path write = %q (status %d), want %q", out, st, want)
	}
}

// And the other direction: writing the scalar moves the array, so a shell
// that only mirrored one way would pass the test above and fail this.
func TestWritingPATHMovesTheArray(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `PATH=/aa:/bb
print -r -- "n=${#path[@]} [${(j:|:)path}]"
PATH=/cc:$PATH
print -r -- "first=[$path[1]] n=${#path[@]}"`)
	want := "n=2 [/aa|/bb]\nfirst=[/cc] n=3\n"
	if out != want || st != 0 {
		t.Errorf("a PATH write = %q (status %d), want %q", out, st, want)
	}
}

// **The export attribute is inherited, never conferred.** Measured both ways
// in the shell being copied: `PATH` lists as `export -T` when the environment
// supplied it and as plain `typeset -T` when it did not, and writing `cdpath`
// never puts `CDPATH` into a child's environment.
func TestATieInheritsTheExportAttributeAndConfersNone(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `export CDPATH=a
typeset -p CDPATH
MANPATH=b
typeset -p MANPATH`)
	// The exported half is exported because the *script* exported it, and
	// the other is not because nothing did. The tie carries neither way.
	want := "export -T CDPATH cdpath=( a )\ntypeset -T MANPATH manpath=( b )\n"
	if out != want || st != 0 {
		t.Errorf("the export attribute = %q (status %d), want %q", out, st, want)
	}
}

// A pair the environment says nothing about starts with **no** elements, not
// one empty one — and no invented default. zsh fills `FPATH` and
// `MODULE_PATH` with its own installation's directories when the environment
// names neither, and pointing this shell's `autoload` at another shell's
// function library would be a wrong answer wearing a right one.
func TestAPairTheEnvironmentDoesNotNameStartsEmpty(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `print -r -- "cdpath n=${#cdpath[@]} CDPATH=[${CDPATH-UNSET}]"
print -r -- "fpath n=${#fpath[@]} FPATH=[${FPATH-UNSET}]"
print -r -- "module_path n=${#module_path[@]}"`)
	want := "cdpath n=0 CDPATH=[]\nfpath n=0 FPATH=[]\nmodule_path n=0\n"
	if out != want || st != 0 {
		t.Errorf("an unnamed pair = %q (status %d), want %q", out, st, want)
	}
}

// `unset` of either half of a built-in tie takes both, the same rule a
// `typeset -T` tie follows — there is one mechanism, not two.
func TestUnsettingHalfABuiltInTieTakesBoth(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `CDPATH=a:b
unset cdpath
print -r -- "CDPATH=[${CDPATH-UNSET}] n=${#cdpath[@]}"`)
	want := "CDPATH=[UNSET] n=0\n"
	if out != want || st != 0 {
		t.Errorf("unsetting half a built-in tie = %q (status %d), want %q", out, st, want)
	}
}
