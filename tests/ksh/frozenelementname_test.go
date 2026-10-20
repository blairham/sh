// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestAFrozenNameIsRefusedUnderItsElementOnThreeRoutes is
// interp.Diagnostics.FrozenElementRefusalNamesTheElement and
// ReadonlyVariableInReadArray: which routes name the element rather than the
// array, and which `read` refusal is not called a warning (#6734, #6774,
// #6775, #6811). Every row measured 2026-10-16 on ksh93u+ 2012-08-01, `-c`;
// the rows naming the array are the controls that keep the rule about the
// route and not about every refusal of an element.
func TestAFrozenNameIsRefusedUnderItsElementOnThreeRoutes(t *testing.T) {
	const tbl = "typeset -A a; readonly a; "
	for _, c := range []struct{ name, src, want string }{
		// read: the array -A fills is not a warning.
		{"read -A a frozen name", `typeset -r v; read -A v <<< x; echo $?`, "ksh: read: v: is read only\n1\n"},
		{"read a frozen name", `typeset -r v; read v <<< x; echo $?`, "ksh: read: warning: v: is read only\n1\n"},
		{"read -A then a frozen name", `readonly w; read -A v w <<< "x y"; echo $?`, "ksh: read: warning: w: is read only\n1\n"},
		// read: the first name, a table's element, as written.
		{"read a table element", tbl + `read 'a["p q"]' <<< x`, "ksh: read: warning: a[\"p q\"]: is read only\n"},
		{"read -A a table element", tbl + `read -A 'a[k]' <<< x`, "ksh: read: a[k]: is read only\n"},
		{"read -A an empty key", tbl + `read -A 'a[]' <<< x`, "ksh: read: a[]: is read only\n"},
		{"read a later table element", tbl + `read x 'a[k]' <<< x; echo $?`, "ksh: read: warning: a: is read only\n1\n"},
		{
			"read two table elements", tbl + `read 'a[k]' 'a[j]' <<< "x y"`,
			"ksh: read: warning: a[k]: is read only\nksh: read: warning: a: is read only\n",
		},
		{"read an indexed element", `b=(1 2); readonly b; read 'b[1]' <<< x`, "ksh: read: warning: b: is read only\n"},
		{"read -A an indexed element", `b=(1 2); readonly b; read -A 'b[1]' <<< x`, "ksh: read: b: is read only\n"},
		// An element literal under a table's key, under the key it came to.
		{"a literal under a key", tbl + `k='p q'; a[$k]=(p)`, "ksh: a[p q]: is read only\n"},
		{"an appended literal", tbl + `a[k]+=(p)`, "ksh: a[k]: is read only\n"},
		{"a declaration's literal", tbl + `typeset a[k]=(p)`, "ksh: a[k]: is read only\n"},
		{"a scalar under a key", tbl + `a[k]=p`, "ksh: a: is read only\n"},
		{"a declaration's scalar", tbl + `typeset a[k]=p`, "ksh: a: is read only\n"},
		// A chain, under its first subscript as it resolved.
		{"a chain", `readonly r; r[1+1][2]=z; echo after`, "ksh: r[2]: is read only\n"},
		{"a chain counted back", `r=(a b); readonly r; r[-1][2]=z`, "ksh: r[1]: is read only\n"},
		{"a chain from a key", `k='p q'; typeset -A r; readonly r; r[$k][2]=z`, "ksh: r[p q]: is read only\n"},
		{"a chain expands first", `readonly r; r[$(echo S >&2; echo 1)][2]=$(echo V >&2)`, "S\nV\nksh: r[1]: is read only\n"},
		{"a chain's subscript is evaluated once", `readonly r; i=0; r[i++][2]=z`, "ksh: r[0]: is read only\n"},
		{"a declaration's chain", `readonly r; typeset r[1+1][2]=z`, "ksh: r[2]: is read only\n"},
		{"one subscript", `readonly r; r[1]=z`, "ksh: r: is read only\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
