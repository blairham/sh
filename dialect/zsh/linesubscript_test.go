// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// Writing through a `(f)` subscript on a string, which replaces the whole
// **line** and not the character it begins at.
//
// Measured 2026-09-25 and re-measured 2026-09-26 against zsh 5.9.2
// (aarch64-apple-darwin25.4.0) run `-f`, the one shell in the panel with the
// construct, with `v=$'aa\nbb\ncc'` each time. Every line here is two
// characters long on purpose: a one-character line cannot tell a span from
// the index of its first character, which is the wrong answer this construct
// was refused by name to avoid (#4497).
//
// An **array** is not a string, so the last row is the ordinary element write
// and must stay that way.
func TestWritingThroughALineSubscript(t *testing.T) {
	dir := t.TempDir()
	const v = `v=$'aa\nbb\ncc'; `
	for _, tc := range []struct{ name, src, want string }{
		{"a line replaced", v + `v[(f)2]=ZZ; print -- "$v"`, "aa\nZZ\ncc\n"},
		{"and joined at its end", v + `v[(f)2]+=XX; print -- "$v"`, "aa\nbbXX\ncc\n"},
		{"past the last clamps", v + `v[(f)4]=ZZ; print -- "$v"`, "aa\nbb\nZZ\n"},
		{"below the first clamps", v + `v[(f)0]=ZZ; print -- "$v"`, "ZZ\nbb\ncc\n"},
		{"a negative counts back", v + `v[(f)-1]=ZZ; print -- "$v"`, "aa\nbb\nZZ\n"},
		{"a search names its line", v + `v[(fr)bb]=ZZ; print -- "$v"`, "aa\nZZ\ncc\n"},
		{"unset takes the line out", v + `unset 'v[(f)2]'; print -- "$v"`, "aa\n\ncc\n"},
		{"and the last one", v + `unset 'v[(f)3]'; print -- "$v"`, "aa\nbb\n\n"},
		{"a value with a separator in it", v + `v[(f)2]=$'X\nY'; print -- "$v"`, "aa\nX\nY\ncc\n"},
		// The other two roads a store reaches the same subscript by.
		{"through arithmetic", v + `(( v[(f)2] = 9 )); print -- "$v"`, "aa\n9\ncc\n"},
		{
			"through a named reference",
			v + `n='v[(f)2]'; : ${(P)n::=Z}; print -- "$v"`, "aa\nZ\ncc\n",
		},
		// The control: a list is not a string.
		{"an array is the ordinary write", `a=(p q r); a[(f)2]=ZZ; print -- "${a[*]}"`, "p ZZ r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
