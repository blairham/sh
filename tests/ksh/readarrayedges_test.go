// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// This column sits between the other two: `read -A` absorbs a closing run of
// IFS whitespace as bash does, and leaves one empty element for a line with no
// fields as zsh does. Measured 2026-09-07 (#1186).
func TestReadIntoAnArrayAbsorbsWhitespaceAndKeepsAnEmptyLine(t *testing.T) {
	dir := t.TempDir()
	count := `printf "n=%s" "${#r[@]}"; for e in "${r[@]}"; do printf "[%s]" "$e"; done`
	for _, tc := range []struct{ src, want string }{
		{`printf 'a  \n' | { read -A r; ` + count + `; }`, "n=1[a]"},
		{`printf '\n' | { read -A r; ` + count + `; }`, "n=1[]"},
		{`printf '   \n' | { read -A r; ` + count + `; }`, "n=1[]"},
	} {
		out, st := runKsh(t, dir, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}
