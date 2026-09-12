// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"
)

// `typeset -r a[1]=v` is this shell's third answer to Semantics.ReadonlyElement,
// and it is an *ordering* rather than an emptiness: the array is frozen first
// and the element write is then lost to the freeze the same declaration has
// just applied.
//
// Measured 2026-09-12 on bash 5.3.15, `-c`:
//
//	typeset -r a[1]=v            a: readonly variable, st=0, declare -ar a=()
//	a=(x y z); typeset -r a[1]=v a: readonly variable, st=0, all three elements
//	a=scalar;  typeset -r a[1]=v a: readonly variable, st=0, [0]="scalar"
//	typeset -Ar m[k]=v           m: readonly variable, st=0, an empty table
//	typeset -r a[1]=v b=2        b is still frozen at 2
//
// `readonly a[1]=v` never reaches the question here — that spelling is refused
// as a bad name first — so this is the `typeset -r` route alone.
func TestAReadonlyElementDeclarationFreezesFirst(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		`typeset -r a[1]=v; echo "st=$?"; typeset -p a; echo after`)
	want := "bash: line 1: a: readonly variable\nst=0\ndeclare -ar a=()\nafter\n"
	if out != want || st != 0 {
		t.Errorf("a fresh name gave %q at %d, want %q at 0", out, st, want)
	}

	// The discriminator the empty case cannot give: an array that is already
	// populated keeps every element, which says the freeze landed before the
	// write rather than instead of the array.
	out, st = runBash(t, t.TempDir(),
		`a=(x y z); typeset -r a[1]=v; echo "st=$?"; typeset -p a`)
	want = "bash: line 1: a: readonly variable\nst=0\n" +
		`declare -ar a=([0]="x" [1]="y" [2]="z")` + "\n"
	if out != want || st != 0 {
		t.Errorf("over a populated array gave %q at %d, want %q at 0", out, st, want)
	}

	// A scalar is promoted into element 0 rather than discarded, and the
	// container letter is honored where one is written.
	out, _ = runBash(t, t.TempDir(), `a=scalar; typeset -r a[1]=v; typeset -p a`)
	if want := "bash: line 1: a: readonly variable\n" + `declare -ar a=([0]="scalar")` + "\n"; out != want {
		t.Errorf("over a scalar gave %q, want %q", out, want)
	}

	// Later operands on the same command are unaffected.
	out, st = runBash(t, t.TempDir(), `typeset -r a[1]=v b=2; echo "st=$?"; typeset -p b`)
	want = "bash: line 1: a: readonly variable\nst=0\n" + `declare -r b="2"` + "\n"
	if out != want || st != 0 {
		t.Errorf("a second operand gave %q at %d, want %q at 0", out, st, want)
	}
}

// The table letter written beside its own operand reaches that operand's
// subscript here — `typeset -A m[k]=v` stores under the key `k` — where ksh93
// evaluates it, the attribute not having landed yet. `k=7` is what tells the
// two apart; with `k` unset both readings answer to `${m[k]}`.
func TestATableLetterReachesItsOwnOperandsSubscript(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		`k=7; typeset -A m[k]=v; echo "st=$?"; echo "k=[${m[k]}] seven=[${m[7]}]"`)
	if want := "st=0\nk=[v] seven=[]\n"; out != want || st != 0 {
		t.Errorf("the letter beside its operand gave %q at %d, want %q at 0", out, st, want)
	}
}
