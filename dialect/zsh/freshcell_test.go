// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A valueless `local` of a name the caller is holding an array in. This shell
// sets a declared name *empty* rather than hiding it, and the emptying wrote
// the scalar table alone — so the cell went on holding the caller's elements,
// which live in a table of their own (#1660).
//
// Measured against zsh 5.9.2 under `-f` on 2026-09-09. `${(t)name}` is what
// says the kind there — `array-local` with the letter, `scalar-local` without
// — and this engine has no `(t)` yet, so the listing stands in for it: an
// empty array lists as `typeset -a a=(  )` and an empty scalar as
// `typeset a=''`, which is the same distinction by another spelling.

// Both spellings, and the caller's array afterwards as the control.
func TestAValuelessLocalOfANameHoldingAnArray(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `arr=(a b)
f(){ local -a arr; print -r -- "letter n=$#arr all=[${arr[*]}]"; typeset -p arr }
f
g(){ local arr; print -r -- "bare n=$#arr all=[${arr[*]}]"; typeset -p arr }
g
print -r -- "after n=$#arr all=[${arr[*]}]"`)
	want := "letter n=0 all=[]\ntypeset -a arr=(  )\n" +
		"bare n=0 all=[]\ntypeset arr=''\n" +
		"after n=2 all=[a b]\n"
	if out != want || st != 0 {
		t.Errorf("a valueless local over a caller's array = %q (status %d), want %q", out, st, want)
	}
}

// `typeset` is the same declaration under the other word here, since every
// function in this shell has a scope: the row is not a corollary but the
// second of the loops the drop has to reach.
func TestAValuelessTypesetInAFunctionOfANameHoldingAnArray(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `arr=(a b)
f(){ typeset arr; print -r -- "in n=$#arr all=[${arr[*]}]"; typeset -p arr }
f
print -r -- "after n=$#arr all=[${arr[*]}]"`)
	want := "in n=0 all=[]\ntypeset arr=''\nafter n=2 all=[a b]\n"
	if out != want || st != 0 {
		t.Errorf("a valueless typeset over a caller's array = %q (status %d), want %q", out, st, want)
	}
}

// The controls that say this is the scope and not the declaration: the same
// valueless declarations over the same array leave it alone where no scope is
// taken — at the top level, and inside a function under the letter that
// declines the scope.
//
// The bare `typeset arr` spelling is not among them, and deliberately. At the
// top level over a standing name that shell *lists* it — `arr=( a b )` on its
// own line before anything else runs — and this engine prints nothing there,
// which is a listing this change does not touch and is filed on its own.
func TestAValuelessDeclarationWithNoScopeKeepsTheArray(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `arr=(a b)
typeset -a arr
print -r -- "top letter n=$#arr all=[${arr[*]}]"
f(){ typeset -g arr; print -r -- "gflag n=$#arr all=[${arr[*]}]" }
f
print -r -- "after n=$#arr all=[${arr[*]}]"
brr=(a b)
h(){ typeset -ga brr; print -r -- "ga n=$#brr all=[${brr[*]}]" }
h`)
	want := "top letter n=2 all=[a b]\ngflag n=2 all=[a b]\n" +
		"after n=2 all=[a b]\nga n=2 all=[a b]\n"
	if out != want || st != 0 {
		t.Errorf("a valueless declaration with no scope = %q (status %d), want %q", out, st, want)
	}
}

// The association, which is the other table and so its own row.
func TestAValuelessLocalOfANameHoldingAnAssociation(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -A m; m=(k v j w)
f(){ local -A m; print -r -- "letter n=${#m} keys=[${(k)m}]"; typeset -p m }
f
g(){ local m; print -r -- "bare n=${#m} [${m[k]-NONE}]"; typeset -p m }
g
print -r -- "after n=${#m} [$m[k]]"`)
	want := "letter n=0 keys=[]\ntypeset -A m=( )\n" +
		"bare n=0 [NONE]\ntypeset m=''\n" +
		"after n=2 [v]\n"
	if out != want || st != 0 {
		t.Errorf("a valueless local over a caller's table = %q (status %d), want %q", out, st, want)
	}
}

// The line this was promoted on. `compaudit` opens with `local -a _i_files
// _i_addfiles _i_wdirs _i_wfiles` and its caller has already filled the first
// of them, so the function began with the caller's entries in it and appended
// its own — and it is `compaudit` that decides whether the completion
// directories are secure.
func TestALocalArrayAppendedToHoldsOnlyWhatTheFunctionAdded(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `_i_files=(stale1 stale2)
audit(){ local -a _i_files _i_addfiles; _i_files+=(real1)
  print -r -- "inside n=$#_i_files [${_i_files[*]}] add=$#_i_addfiles" }
audit
print -r -- "after n=$#_i_files [${_i_files[*]}]"`)
	want := "inside n=1 [real1] add=0\nafter n=2 [stale1 stale2]\n"
	if out != want || st != 0 {
		t.Errorf("the compaudit shape = %q (status %d), want %q", out, st, want)
	}
}
