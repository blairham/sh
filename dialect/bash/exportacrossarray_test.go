// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// This column keeps the export attribute however the array arrives, which is
// the same answer it gives to every shape of the re-creation question.
//
// The row is here because it is the one that must not move: the fix for
// #4676 is in the core, on a list two other dialects read the other way, and
// a change that took the attribute off everywhere would pass every test
// written in the dialect that wanted it. Measured 2026-09-26 on GNU bash
// 5.3.20 under `--noprofile --norc`.
func TestAnArrayAssignmentKeepsTheExportAttribute(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a literal over an exported scalar", `export a=1; a=(x y); declare -p a`, "declare -ax a=([0]=\"x\" [1]=\"y\")\n"},
		{"an append over one", `export a=1; a+=(x y); declare -p a`, "declare -ax a=([0]=\"1\" [1]=\"x\" [2]=\"y\")\n"},
		{"a literal over an exported array", `export a=(p q); a=(x y); declare -p a`, "declare -ax a=([0]=\"x\" [1]=\"y\")\n"},
		{"and a declaration", `export a=1; declare -a a=(x y); declare -p a`, "declare -ax a=([0]=\"x\" [1]=\"y\")\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
