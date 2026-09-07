// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// Nothing a declaration carries makes this shell refuse an element operand:
// the attribute lands on the array and the element is written under it.
//
// The readonly row is the one that separates this column from the other two —
// bash refuses `readonly a[1]=v` as a bad name and zsh refuses the element
// itself, and here the element is written and the array frozen over it
// (#1203).
func TestASubscriptedOperandTakesWhateverTheDeclarationCarries(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`typeset a[1]=v; echo "[${a[1]}]"`, "[v]\n"},
		{`export a[1]=v; echo "st=$? [${a[1]}]"`, "st=0 [v]\n"},
		{`typeset -i a[1]=0x10; echo "[${a[1]}]"`, "[16]\n"},
		{`readonly a[1]=v; echo "st=$? [${a[1]}]"`, "st=0 [v]\n"},
	} {
		out, st := runKsh(t, dir, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// And the freeze is real: the array will not take another element afterwards.
func TestAReadonlyElementFreezesTheArrayOverIt(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `readonly a[1]=v; a[2]=q; echo "[${a[1]}][${a[2]}]"`)
	if want := "ksh: a: is read only\n"; out != want || st == 0 {
		t.Errorf("readonly a[1]=v; a[2]=q = %q (status %d), want %q and a failure", out, st, want)
	}
}
