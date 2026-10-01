// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "testing"

// **`read` drops a NUL and keeps the bytes around it** — see
// interp.Semantics.NulInAValue. Measured against BusyBox ash 1.37.0.
func TestReadDropsANul(t *testing.T) {
	out, st := runIn(t, `printf 'A=1\0B=2\0c\0d\n' > f
read -r v < f; echo "raw=$v len=${#v}"
read v < f; echo "cooked=${#v}"`)
	if want := "raw=A=1B=2cd len=8\ncooked=8\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
