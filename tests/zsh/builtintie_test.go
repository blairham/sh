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

// The seeding: a scalar the shell arrives holding fills its array *before*
// any script runs, which is a different moment from the mirror every later
// write goes through. Nothing else here can see it — every other test sets
// its scalar first and is then watching the mirror.
func TestATieIsSeededFromTheValueTheShellArrivesWith(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, `print -r -- "n=${#path[@]} [$path[1]]"`)
	want := "n=1 [" + dir + "]\n"
	if out != want || st != 0 {
		t.Errorf("the seeded array = %q (status %d), want %q", out, st, want)
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

// `unset` of either half of a built-in tie takes both *names*, the same rule
// a `typeset -T` tie follows.
func TestUnsettingHalfABuiltInTieTakesBoth(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `CDPATH=a:b
unset cdpath
print -r -- "CDPATH=[${CDPATH-UNSET}] n=${#cdpath[@]}"`)
	want := "CDPATH=[UNSET] n=0\n"
	if out != want || st != 0 {
		t.Errorf("unsetting half a built-in tie = %q (status %d), want %q", out, st, want)
	}
}

// **The pairing is what it does not take.** This is where the two kinds of
// tie part company: a `typeset -T` tie is forgotten by `unset` and the next
// assignment writes a plain scalar, where one of the shell's own pairs is
// still a pair and the next assignment to either half re-makes the other.
//
// Measured 2026-09-12 against zsh 5.9.2 under `-f` with `PATH=/bin:/usr/bin`.
// It is not a corner: `unset PATH; PATH=…` is how a script pins a search
// path from scratch, and without this the rest of the run had an empty
// `$path` that nothing would refill (#1631).
func TestTheBuiltInPairingSurvivesAnUnset(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `unset PATH
PATH=/y
print -r -- "1=[${(j:,:)path}]"
unset path
path=(/q /r)
print -r -- "2=[$PATH]"
typeset -T MYS mys
MYS=a:b
unset MYS
MYS=c:d
print -r -- "3=[${#mys[@]}][$MYS]"`)
	want := "1=[/y]\n2=[/q:/r]\n3=[0][c:d]\n"
	if out != want || st != 0 {
		t.Errorf("a built-in pairing across an unset = %q (status %d), want %q", out, st, want)
	}
}

// A `local` of either half of one of the shell's own pairs displaces **both**,
// and the caller gets both back. Saving the name written and not the pair let
// a function-local search path outlive the function: `f() { local PATH=/x; }`
// left `path` at `/x` for the rest of the script (#1630).
//
// Measured 2026-09-09 against zsh 5.9.2 under `-f` with a two-entry `PATH`.
func TestALocalOfOneHalfOfABuiltInTieShadowsTheOther(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `PATH=/bin:/usr/bin
f() { local PATH=/x; print -r -- "in=[$PATH][${(j:,:)path}]"; }
f
print -r -- "after=[$PATH][${(j:,:)path}]"
g() { local -a path=(/p /q); print -r -- "gin=[$PATH][${(j:,:)path}]"; }
g
print -r -- "after=[$PATH][${(j:,:)path}]"`)
	want := "in=[/x][/x]\nafter=[/bin:/usr/bin][/bin,/usr/bin]\n" +
		"gin=[/p:/q][/p,/q]\nafter=[/bin:/usr/bin][/bin,/usr/bin]\n"
	if out != want || st != 0 {
		t.Errorf("a local of half a built-in tie = %q (status %d), want %q", out, st, want)
	}
}

// `compaudit`'s own first line, and the answer that decides what it audits.
// The local array is a fresh, empty one — zsh hands the function nothing —
// where the caller's entries in view mean the code that decides whether the
// completion directories are secure is walking the caller's `fpath` instead
// of a copy of it. #1621 took the `+h` letter; this is what the line then has
// to hold.
func TestCompauditsFirstLineGetsAnEmptyLocalArray(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `FPATH=/usr/share/zsh/functions:/opt/extra
comp() { local -a -U +h fpath; print -r -- "in n=$#fpath first=[${fpath[1]-NONE}] FPATH=[$FPATH]"; }
comp
print -r -- "after n=$#fpath FPATH=[$FPATH]"`)
	want := "in n=0 first=[NONE] FPATH=[]\n" +
		"after n=2 FPATH=[/usr/share/zsh/functions:/opt/extra]\n"
	if out != want || st != 0 {
		t.Errorf("compaudit's first line = %q (status %d), want %q", out, st, want)
	}
}

// The two halves are emptied in their own kinds, which is why the counts
// differ by one. A valueless `local` of the scalar sets an empty string and
// the mirror splits it into the single field it has; a valueless `local` of
// the array sets no elements at all and the mirror joins them into nothing.
func TestAValuelessLocalOfEachHalfEmptiesItInItsOwnKind(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `PATH=/bin:/usr/bin
f() { local PATH; print -r -- "scalar=[$PATH] n=$#path first=[${path[1]-NONE}]"; }
f
g() { local path; print -r -- "array=[$PATH] n=$#path"; }
g`)
	want := "scalar=[] n=1 first=[]\narray=[] n=0\n"
	if out != want || st != 0 {
		t.Errorf("a valueless local of a tie half = %q (status %d), want %q", out, st, want)
	}
}

// A tie the *script* made is the other answer, and the row is here so the two
// are read side by side: `local` makes a new parameter, which is not tied at
// all, and the other half goes on naming the outer cell. A pair-shadow
// applied to every tie would answer the first line `[zzz][zzz]`.
func TestALocalOfHalfAScriptTieIsAnOrdinaryLocal(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -T SCA sca
SCA=a:b:c
f() { local SCA=zzz; print -r -- "in=[$SCA] n=$#sca"; }
f
g() { local -a sca=(p q); print -r -- "gin=[$SCA] n=$#sca"; }
g
print -r -- "after=[$SCA] n=$#sca"`)
	// The `g` row is written with a value on purpose: a *valueless* local of
	// an array name still shows the caller's elements here, which is a bug of
	// its own (#1660) and would decide this row rather than the tie.
	want := "in=[zzz] n=3\ngin=[a:b:c] n=2\nafter=[a:b:c] n=3\n"
	if out != want || st != 0 {
		t.Errorf("a local of half a script tie = %q (status %d), want %q", out, st, want)
	}
}
