// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `read -A` here opens an element on a closing run of IFS whitespace, and
// leaves one empty element for a line that splits into nothing — measured
// 2026-09-07 against zsh 5.9.2.
//
// The whitespace row is the one worth keeping in sight: through an expansion
// this same shell *absorbs* a closing whitespace run, so the two are separate
// answers and not one (#1186).
func TestReadIntoAnArrayKeepsTheClosingWhitespaceField(t *testing.T) {
	dir := t.TempDir()
	count := `printf "n=%s" "${#r[@]}"; for e in "${r[@]}"; do printf "[%s]" "$e"; done`
	for _, tc := range []struct{ src, want string }{
		{`printf 'a  \n' | { read -A r; ` + count + `; }`, "n=2[a][]"},
		{`printf 'a\t\n' | { read -A r; ` + count + `; }`, "n=2[a][]"},
		{`printf ' a \n' | { read -A r; ` + count + `; }`, "n=2[a][]"},
		{`printf ' a\n' | { read -A r; ` + count + `; }`, "n=1[a]"},
		{`printf '\n' | { read -A r; ` + count + `; }`, "n=1[]"},
		// A closing run *and* no fields: one element, not two, which is what
		// fixes the order the two questions are asked in.
		{`printf '   \n' | { read -A r; ` + count + `; }`, "n=1[]"},
		// And through an expansion the closing run is absorbed, which is the
		// row that says these are two answers and not one.
		{`x=" a "; set -- ${=x}; printf "n=%s" "$#"`, "n=1"},
	} {
		out, st := runZsh(t, dir, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}
