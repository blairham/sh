// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestACountedReadStillSplits is the No side of
// interp.Semantics.ReadCountFillsTheFirstNameWhole: a counted read splits what
// it took across the names, clears the ones it has no field for and trims the
// one it fills. Measured 2026-10-16 on bash 5.3.20, `-c`.
func TestACountedReadStillSplits(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"-n two names", `y=keep; read -n 3 x y <<< 'p q rs'; echo "[$x] [$y]"`, "[p] [q]\n"},
		{"-N two names", `y=keep; read -N 3 x y <<< 'p q rs'; echo "[$x] [$y]"`, "[p q] []\n"},
		{"-n -a", `read -n 3 -a x <<< 'p q rs'; declare -p x`, "declare -a x=([0]=\"p\" [1]=\"q\")\n"},
		{"-n trimmed", `read -n 4 x <<< '  pq'; echo "[$x]"`, "[pq]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBash(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
