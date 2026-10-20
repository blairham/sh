// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestAValuelessElementPastTheFirst: over a frozen name holding nothing, a
// valueless operand past the first element is a write and is refused naming
// the builtin; over a frozen name holding a value, and over a frozen table's
// key, it is taken (#6536). Measured 2026-10-16 on ksh93u+ 2012-08-01, `-c`.
func TestAValuelessElementPastTheFirst(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"an element there already", `x=(a b); typeset x[1]; typeset -p x`, "typeset -a x=(a b)\n"},
		{"frozen, holding a string", `x=s; readonly x; typeset x[2]; echo st=$?`, "st=0\n"},
		{"frozen, holding an array", `x=(a); readonly x; typeset x[3]; echo st=$?`, "st=0\n"},
		{"frozen, holding an empty string", `x=''; readonly x; typeset x[2]; echo st=$?`, "st=0\n"},
		{"frozen, holding nothing", `readonly x; typeset x[2]; echo st=$?`, "ksh: typeset: x: is read only\n"},
		{"frozen, an empty array", `typeset -a x; readonly x; typeset x[2]; echo st=$?`, "ksh: typeset: x: is read only\n"},
		{"frozen twice", `readonly x[1]; readonly x[2]; echo st=$?`, "ksh: readonly: x: is read only\n"},
		{"in a function", `function f { readonly x[1] x[2]; typeset -p x; }; f; echo after`, "ksh: f[1]: readonly: x: is read only\nafter\n"},
		{"export", `readonly x; export x[2]; echo st=$?`, "ksh: export: x: is read only\n"},
		{"the first element", `readonly x; typeset x[0]; echo st=$?`, "st=0\n"},
		{"a frozen table's key", `typeset -A m; readonly m; typeset m[k]; typeset -p m`, "typeset -r -A m=([k]=)\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
