// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `typeset -U`, measured against zsh 5.9.2 (2026-09-06). It is this shell's
// letter alone: bash answers `typeset: -U: invalid option` and ksh93u+
// answers `typeset: -U: unknown option`, so nothing here is an axis.

// The letter this shell has and this engine used to name as missing. Every
// line is one this shell's own output, taken side by side.
func TestTypesetUniqueKeepsTheFirstOfEachElement(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -U a
a=(1 1 2 2 3 1)
print -r -- "write: ${(j:,:)a} n=${#a}"
a+=(2 4 4)
print -r -- "append: ${(j:,:)a} n=${#a}"
b=(1 1 2)
typeset -U b
print -r -- "afterdecl: ${(j:,:)b} n=${#b}"
typeset -U c=(9 9 8)
print -r -- "inline: ${(j:,:)c} n=${#c}"
typeset -U s
s=abcabc
print -r -- "scalar: [$s]"
typeset -U d=(1 2 3)
d[2]=3
print -r -- "elemassign: ${(j:,:)d} n=${#d}"
typeset -U e=(1 2)
e[5]=1
print -r -- "gap: ${(j:,:)e} n=${#e}"
typeset -U g=(1 2 3)
g=(3 $g)
print -r -- "prepend: ${(j:,:)g} n=${#g}"
typeset +U g
g+=(1)
print -r -- "plusU: ${(j:,:)g} n=${#g}"`)
	want := "write: 1,2,3 n=3\nappend: 1,2,3,4 n=4\nafterdecl: 1,2 n=2\ninline: 9,8 n=2\n" +
		"scalar: [abcabc]\nelemassign: 1,3 n=2\ngap: 1,2, n=3\nprepend: 3,1,2 n=3\n" +
		"plusU: 3,1,2,1 n=4\n"
	if out != want || st != 0 {
		t.Errorf("typeset -U = %q (status %d), want %q", out, st, want)
	}
}

// `local -U` too — the letter is in both sets here.
func TestLocalUniqueDedupesInAFunction(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `f() { local -U u=(1 1 2); print -r -- "local: ${(j:,:)u}"; }
f
print -r -- "st=$?"`)
	want := "local: 1,2\nst=0\n"
	if out != want || st != 0 {
		t.Errorf("local -U = %q (status %d), want %q", out, st, want)
	}
}

// And it lists back, last of the letters — with `export` taking `x`'s place
// wherever `x` stood rather than only at the end of the word.
func TestTypesetUniqueListsBackLastOfTheLetters(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -aUxr A1=(1 2)
typeset -p A1
typeset -Uxi n1=5
typeset -p n1
typeset -Ur r1=q
typeset -p r1
typeset -Ux x1=v
typeset -p x1
typeset -aU e1=()
typeset -p e1`)
	want := "typeset -arxU A1=( 1 2 )\nexport -iU n1=5\ntypeset -rU r1=q\nexport -U x1=v\n" +
		"typeset -aU e1=(  )\n"
	if out != want || st != 0 {
		t.Errorf("typeset -p under -U = %q (status %d), want %q", out, st, want)
	}
}

// The seven `typeset -gxU` lines a plugin manager opens with, in the shape it
// writes them: the global and export letters bundled with this one, an array
// and a scalar named in the same command, and the array prepended to
// afterwards. It was seven refusals and an array left unmade.
func TestTypesetGlobalExportUniqueBuildsAPathArray(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `mypath=(/usr/bin /bin)
typeset -gxU mypath MYPATH
mypath=( /usr/bin "${mypath[@]}" )
print -r -- "mypath: ${(j:,:)mypath} n=${#mypath}"`)
	want := "mypath: /usr/bin,/bin n=2\n"
	if out != want || st != 0 {
		t.Errorf("typeset -gxU = %q (status %d), want %q", out, st, want)
	}
}
