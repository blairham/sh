// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// This column takes the export attribute off an array **literal** and leaves
// an append alone, which is the opposite way round from zsh on both rows.
//
// That opposition is the whole reason #4676 needed no axis of its own: the
// two columns disagree about what counts as a re-creation, three measured
// axes already hold that disagreement, and the export attribute follows them
// cell for cell. Measured 2026-09-26 on ksh93u+ 2012-08-01.
func TestAnArrayLiteralDropsTheExportAttribute(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a literal over an exported scalar", `export a=1; a=(x y); typeset -p a`, "typeset -a a=(x y)\n"},
		{"a literal over an exported array", `export a=(p q); a=(x y); typeset -p a`, "typeset -a a=(x y)\n"},
		{"an append keeps it", `export a=1; a+=(x y); typeset -p a`, "typeset -x -a a=(1 x y)\n"},
		{"and so does an element write", `export a=1; a[1]=z; typeset -p a`, "typeset -x -a a=(1 z)\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
