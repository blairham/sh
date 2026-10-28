// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestAFrozenContainerAndAnElementLiteral is
// interp.Semantics.FrozenContainerTakesAnElementLiteral in this shell: an
// element literal is stored through the freeze — into an indexed array at any
// element, under a table's standing key, and at the end of any chain — and
// refused directly under a table's missing key. The scalar rows beside them
// are the controls that keep the freeze a freeze. Every row measured
// 2026-10-16 and 2026-10-27 on ksh93u+ 2012-08-01, `-c` (#6833).
func TestAFrozenContainerAndAnElementLiteral(t *testing.T) {
	const tbl = "typeset -A a; "
	for _, c := range []struct{ name, src, want string }{
		// Taken.
		{"a standing element", `b=(x y); readonly b; b[1]=(p); echo st=$?; typeset -p b`, "st=0\ntypeset -r -a b=(x (p) )\n"},
		{"an empty array", `typeset -a b; readonly b; b[1]=(p); echo st=$?; typeset -p b`, "st=0\ntypeset -r -a b=([1]=(p) )\n"},
		{"a name holding nothing", `readonly b; b[1]=(p); echo st=$?; typeset -p b`, "st=0\ntypeset -r -a b=([1]=(p) )\n"},
		{"past the end", `b=(x y); readonly b; b[5]=(p); echo st=$?; typeset -p b`, "st=0\ntypeset -r -a b=([0]=x [1]=y [5]=(p) )\n"},
		{"appended", `b=(x y); readonly b; b[1]+=(p); echo st=$?; typeset -p b`, "st=0\ntypeset -r -a b=(x (p) )\n"},
		{"the empty list", `b=(x y); readonly b; b[1]=(); echo st=$?; typeset -p b`, "st=0\ntypeset -r -a b=(x ())\n"},
		{"a declaration's", `b=(x y); readonly b; typeset b[1]=(p); echo st=$?; typeset -p b`, "st=0\ntypeset -r -a b=(x (p) )\n"},
		{"a declaration's with the letter", `b=(x y); readonly b; typeset -a b[1]=(p); echo st=$?; typeset -p b`, "st=0\ntypeset -r -a b=(x (p) )\n"},
		{"export's", `b=(x y); readonly b; export b[1]=(p); echo st=$?; typeset -p b`, "st=0\ntypeset -x -r -a b=(x (p) )\n"},
		{"readonly's", `b=(x y); readonly b; readonly b[1]=(p); echo st=$?; typeset -p b`, "st=0\ntypeset -r -a b=(x (p) )\n"},
		{"a declaration's empty list", `b=(x y); readonly b; typeset b[1]=(); echo st=$?; typeset -p b`, "st=0\ntypeset -r -a b=(x ())\n"},
		{"a chain from nothing", `readonly r; r[1][2]=(p); echo st=$?; typeset -p r`, "st=0\ntypeset -r -a r=([1]=([2]=(p) ) )\n"},
		{"a table's standing key", tbl + `a[k]=1; readonly a; a[k]=(p); echo st=$?; typeset -p a`, "st=0\ntypeset -r -A a=([k]=(p) )\n"},
		{"a declaration's under a standing key", tbl + `a[k]=1; readonly a; typeset a[k]=(p); echo st=$?; typeset -p a`, "st=0\ntypeset -r -A a=([k]=(p) )\n"},
		{"the empty list under a standing key", tbl + `a[k]=1; readonly a; a[k]=(); echo st=$?; typeset -p a`, "st=0\ntypeset -r -A a=([k]=())\n"},
		{"a chain from a missing key", tbl + `readonly a; a[k][1]=(p); echo st=$?; typeset -p a`, "st=0\ntypeset -r -A a=([k]=([1]=(p) ) )\n"},
		{"a chain from a standing scalar key", tbl + `a[k]=1; readonly a; a[k][1]=(p); echo st=$?; typeset -p a`, "st=0\ntypeset -r -A a=([k]=(1 (p) ) )\n"},
		{"the value is expanded, then stored", `b=(x y); readonly b; b[$(echo S >&2; echo 1)]=(p $(echo V >&2)); echo st=$?`, "S\nV\nst=0\n"},
		{"what it stored is readable", `b=(x y); readonly b; b[1]=(p); echo "${b[1]}|${b[1][0]}|${#b[@]}"`, "p|p|2\n"},
		// A scalar at the end of a chain, through the freeze where its root
		// is a table's standing key or holds an array of its own.
		{"a scalar chain into a nested element", `r=(x y); r[1]=(p q); readonly r; r[1][0]=z; echo st=$?; typeset -p r`, "st=0\ntypeset -r -a r=(x (z q) )\n"},
		{"a scalar chain under a standing key", tbl + `a[k]=1; readonly a; a[k][1]=z; echo st=$?; typeset -p a`, "st=0\ntypeset -r -A a=([k]=(1 z) )\n"},
		// Refused.
		{"a missing key", tbl + `readonly a; a[k]=(p); echo after`, "ksh: a[k]: is read only\n"},
		{"the empty list under a missing key", tbl + `readonly a; a[k]=(); echo after`, "ksh: a[k]: is read only\n"},
		{"a declaration's under a missing key expands first", tbl + `readonly a; typeset a[k]=(p $(echo V >&2)); echo after`, "V\nksh: a[k]: is read only\n"},
		{"a scalar element", `b=(x y); readonly b; b[1]=p; echo after`, "ksh: b: is read only\n"},
		{"a scalar over a nested element", `r=(x y); r[1]=(p q); readonly r; r[1]=z; echo after`, "ksh: r: is read only\n"},
		{"a scalar chain into a scalar element", `b=(x y); readonly b; b[1][2]=z; echo after`, "ksh: b[1]: is read only\n"},
		{"a scalar chain from nothing", `readonly r; r[1][2]=z; echo after`, "ksh: r[1]: is read only\n"},
		{"a scalar chain from a missing key", `k='p q'; ` + tbl + `readonly a; a[$k][2]=z; echo after`, "ksh: a[p q]: is read only\n"},
		{"the whole array", `b=(x y); readonly b; b=(p); echo after`, "ksh: b: is read only\n"},
		{"a compound body under the element", `readonly c; typeset c[1]=(a=1); echo after`, "ksh: c: is read only\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
			}
		})
	}
	// The literal's own ordinary store still works: the control that says
	// the rows above are the freeze lifting and not a store that was never
	// refused.
	out, _ := runKsh(t, t.TempDir(), `b=(x y); b[1]=(p); typeset -p b`)
	if want := "typeset -a b=(x (p) )\n"; out != want {
		t.Errorf("unfrozen control = %q, want %q", out, want)
	}
}
