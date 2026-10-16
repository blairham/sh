// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestReadArrayIntoATableKeyNests: `read -A` into a table's element stores the
// words as that element's own array, which stored nothing at all here (#6685).
// Measured 2026-10-16 on ksh93u+ 2012-08-01, `-c`.
func TestReadArrayIntoATableKeyNests(t *testing.T) {
	const tbl = "typeset -A a; "
	for _, c := range []struct{ name, src, want string }{
		{"a key", tbl + `read -A 'a[k]' <<< 'p q'; typeset -p a; echo "${a[k]} ${#a[k][@]}"`, "typeset -A a=([k]=(p q) )\np 2\n"},
		{"the empty key", tbl + `read -A 'a[]' <<< 'p q'; typeset -p a`, "typeset -A a=(['']=(p q) )\n"},
		{"the key as written", tbl + `read -A 'a["p q"]' <<< x; typeset -p a`, "typeset -A a=(['\"p q\"']=(x) )\n"},
		{"over a string", `typeset -A a=([k]=old); read -A 'a[k]' <<< 'x y'; typeset -p a`, "typeset -A a=([k]=(x y) )\n"},
		{"over an array, in place", `typeset -A a=([k]=(o1 o2)); read -A 'a[k]' <<< x; typeset -p a`, "typeset -A a=([k]=(x o2) )\n"},
		{"an empty line", tbl + `read -A 'a[k]' <<< ''; typeset -p a`, "typeset -A a=([k]=('') )\n"},
		{
			"the warning", tbl + "set -x; read -A 'a[]' <<< x",
			"+ read -A 'a[]'\n+ 0<<< x\nksh: read: warning: adding empty subscript\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// TestACountedReadFillsItsFirstNameWhole is the Yes side of
// interp.Semantics.ReadCountFillsTheFirstNameWhole: with `-n` or `-N` the text
// goes to the first name unsplit and untrimmed, the names after it keep what
// they held, and under `-A` the array takes it as its one element — or, where
// nothing was there, the name takes it as a scalar (#6776). Measured
// 2026-10-16 on ksh93u+ 2012-08-01, `-c`.
func TestACountedReadFillsItsFirstNameWhole(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"-N -A over nothing", `read -N 3 -A x <<< pqrs; typeset -p x`, "x=pqr\n"},
		{"-N -A over an array", `x=(o1 o2); read -N 3 -A x <<< pqrs; typeset -p x`, "typeset -a x=(pqr)\n"},
		{"-n -A over an array", `x=(o1 o2); read -n 3 -A x <<< 'p q rs'; typeset -p x`, "typeset -a x=('p q')\n"},
		{"-n -A", `read -n 3 -A x <<< 'p q rs'; typeset -p x`, "x='p q'\n"},
		{"-N -A into a key", `typeset -A a; read -N 3 -A 'a[k]' <<< pqrs; typeset -p a`, "typeset -A a=([k]=pqr)\n"},
		{"-n two names", `y=keep; read -n 3 x y <<< 'p q rs'; echo "[$x] [$y]"`, "[p q] [keep]\n"},
		{"-N two names", `y=keep; read -N 3 x y <<< 'p q rs'; echo "[$x] [$y]"`, "[p q] [keep]\n"},
		{"-n untrimmed", `read -n 4 x <<< '  pq'; echo "[$x]"`, "[  pq]\n"},
		{"-n into REPLY", `read -n 4 <<< '  pq'; echo "[$REPLY]"`, "[  pq]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
