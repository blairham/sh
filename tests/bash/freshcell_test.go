// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// What this shell's declarations find in the cell a scope was just taken for.
// The rule is the core's — see interp/freshcell.go — and this shell is where
// three of its four shapes were measured, on 2026-09-09 against bash 5.3.15
// and bash 3.2.57 with `--norc --noprofile`.
//
// The valueless rows were already right here and are kept as the control: a
// declared name is *hidden* rather than set empty in this shell, and hiding a
// name takes its array with it, so `local -a arr` over a caller's array was
// never the bug #1660 reported. The rows with a value were not, because this
// shell also merges a scalar store into a standing compound — so the caller's
// array was there to merge with.

// The control: the valueless spellings, which this shell answers by hiding
// the name outright.
func TestAValuelessLocalOverACallersArrayStillHidesIt(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `arr=(a b)
f(){ local -a arr; echo "letter n=${#arr[@]} [${arr[*]}] [${arr[1]-NONE}]"; }
f
g(){ local arr; echo "bare n=${#arr[@]} [${arr[*]}] [${arr[1]-NONE}]"; }
g
echo "after n=${#arr[@]} [${arr[*]}]"`)
	want := "letter n=0 [] [NONE]\nbare n=0 [] [NONE]\nafter n=2 [a b]\n"
	if out != want || st != 0 {
		t.Errorf("a valueless local over a caller's array = %q (status %d), want %q", out, st, want)
	}
}

// A `local` that assigns is the row the same rule corrected here. The scalar
// store this shell merges into a standing array has nothing to merge with in
// a cell the declaration just made, so the name is the scalar that was
// written — `declare -- brr="x"`, not the caller's array with `x` in front of
// it.
func TestALocalWithAValueOverACallersArrayIsAScalar(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `brr=(a b)
f(){ local brr=x; declare -p brr; }
f
declare -p brr`)
	want := "declare -- brr=\"x\"\ndeclare -a brr=([0]=\"a\" [1]=\"b\")\n"
	if out != want || st != 0 {
		t.Errorf("a local with a value over a caller's array = %q (status %d), want %q", out, st, want)
	}
}

// The control that makes the row above about the cell: the same assignment
// through the same builtin merges where no scope was taken, at the top level
// and under the letter that declines the scope alike.
func TestADeclarationWithAValueAndNoScopeStillMergesWithTheArray(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `crr=(a b); declare crr=x; declare -p crr
drr=(a b); f(){ declare -g drr=x; declare -p drr; }; f`)
	want := "declare -a crr=([0]=\"x\" [1]=\"b\")\ndeclare -a drr=([0]=\"x\" [1]=\"b\")\n"
	if out != want || st != 0 {
		t.Errorf("a declaration with a value and no scope = %q (status %d), want %q", out, st, want)
	}
}

// And the subscripted operand, which is the third declaration loop: the
// element it writes is the only one in the cell, where the same line at the
// top level replaces one of three and leaves the other two.
func TestASubscriptedDeclarationIntoAFreshCellWritesTheOnlyElement(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `err=(a b c)
f(){ local err[1]=z; declare -p err; }
f
declare -p err
frr=(a b c); declare frr[1]=z; declare -p frr`)
	want := "declare -a err=([1]=\"z\")\ndeclare -a err=([0]=\"a\" [1]=\"b\" [2]=\"c\")\n" +
		"declare -a frr=([0]=\"a\" [1]=\"z\" [2]=\"c\")\n"
	if out != want || st != 0 {
		t.Errorf("a subscripted declaration into a fresh cell = %q (status %d), want %q", out, st, want)
	}
}
