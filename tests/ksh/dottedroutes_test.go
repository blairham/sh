// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestTheRestOfTheDottedNameRoutes: the routes #6628 left — a loop's variable,
// `read`, an arithmetic store and `let` under a missing parent, a reference
// aimed at `.sh`, `${.sh}` itself, and the trace of a prefix that will be
// refused (#6794). Measured 2026-10-16 on ksh93u+ 2012-08-01 from a script
// file; the harness runs a string, which writes no line.
func TestTheRestOfTheDottedNameRoutes(t *testing.T) {
	const members = "command dollar edchar edcol edmode edtext file fun level lineno " +
		"match math name pool stats subscript subshell type value version"
	for _, c := range []struct{ name, src, want string }{
		{"for", `for a.b in 1; do :; done; echo "st=$?"`, "ksh: a.b: no parent\n"},
		{"for expands first", `for a.b in $(echo X >&2); do :; done`, "X\nksh: a.b: no parent\n"},
		{"select", `select a.b in 1; do break; done <<< 1`, "ksh: a.b: no parent\n"},
		{"read", `read a.b <<< x; echo "st=$?"`, "ksh: read: a.b: no parent\nst=1\n"},
		{"read stops there", `read c a.b d <<< "x y z"; echo "st=$? c=$c d=$d"`, "ksh: read: a.b: no parent\nst=1 c=x d=\n"},
		{"(( ))", `((a.b=1)); echo "st=$?"`, "ksh: a.b: no parent\n"},
		{"$(( ))", `x=$((a.b=1)); echo "st=$?"`, "ksh: a.b: no parent\n"},
		{"let", `let a.b=1; echo "st=$?"`, "ksh: let: a.b: no parent\nst=1\n"},
		{"a read asks nothing", `(( a.b )); echo "st=$?"`, "st=1\n"},
		{"a parent that is there", `typeset -C c; read c.x <<< v; for c.y in 1 2; do :; done; ((c.z=3)); echo ${c.x} ${c.y} ${c.z}`, "v 2 3\n"},
		{
			"a reference aimed at .sh", `nameref r=.sh; echo "[$r]"; r=1; echo st`,
			"[" + members + "]\nksh: .sh: is read only\n",
		},
		{"${.sh}", `echo "[${.sh}]" "${#.sh}"; [[ -v .sh ]]; echo $?`, "[" + members + "] 128\n0\n"},
		{"a refused prefix is not traced", `set -x; x=1 a.b=$(echo v) :; echo after`, "+ x=1\n+ echo v\nksh: a.b=v: no parent\n"},
		{"a prefix that is taken is", `typeset -C c; set -x; c.x=1 :`, "+ c.x=1\n+ :\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
