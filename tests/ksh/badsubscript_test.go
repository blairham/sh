// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// `unset` names itself in front of the sentence, having worded the identical
// failure in an expansion without one, and leaves a failed builtin behind
// rather than ending the script. Measured against ksh93u+ (2026-09-05).
func TestABadSubscriptToUnsetNamesTheBuiltin(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `a=(x y z); unset "a[1+]"; echo "st=$? n=${#a[@]}"`)
	want := "ksh: unset: 1+: more tokens expected\nst=1 n=3\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// An expansion gets the same sentence with nothing in front of it.
func TestABadSubscriptInAnExpansionNamesNoBuiltin(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `a=(x y z); echo "[${a[1+]}]"; echo after`)
	want := "ksh: 1+: more tokens expected\n"
	if out != want || st != 1 {
		t.Errorf("got %q (status %d), want %q at 1", out, st, want)
	}
}

// A failing substring offset is blamed together with everything after it in
// the range; a failing length has nothing after it and is named alone.
func TestABadSubstringOffsetNamesTheWholeRange(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`x=abcdef; echo "[${x:1/0:2}]"`, "ksh: 1/0:2: divide by zero\n"},
		{`x=abcdef; echo "[${x:2:1+}]"`, "ksh: 1+: more tokens expected\n"},
	} {
		out, st := runKsh(t, t.TempDir(), c.src)
		if out != c.want || st != 1 {
			t.Errorf("%s = %q (status %d), want %q at 1", c.src, out, st, c.want)
		}
	}
}
