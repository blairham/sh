// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `read -a` reads its edges as the ordinary field split does: a closing run of
// separators is absorbed whether it is whitespace or not, and a line with no
// fields in it fills no elements at all. Measured 2026-09-07 (#1186).
func TestReadIntoAnArrayReadsItsEdgesAsTheFieldSplitDoes(t *testing.T) {
	dir := t.TempDir()
	count := `printf "n=%s" "${#r[@]}"`
	for _, tc := range []struct{ src, want string }{
		{`printf 'a  \n' | { read -a r; ` + count + `; }`, "n=1"},
		{`printf '\n' | { read -a r; ` + count + `; }`, "n=0"},
		{`printf '   \n' | { read -a r; ` + count + `; }`, "n=0"},
	} {
		out, st := runBash(t, dir, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}
