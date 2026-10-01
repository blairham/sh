// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// **`read` drops a NUL and keeps the bytes around it** — see
// interp.Semantics.NulInAValue. Measured against ksh93u+; the input has no
// blank after a NUL, which ksh93 would drop as well (not modeled, noted on
// the axis).
func TestReadDropsANul(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `printf 'A=1\0B=2\0c\0d\n' > f
read -r v < f; echo "raw=$v len=${#v}"
read v < f; echo "cooked=${#v}"`)
	if want := "raw=A=1B=2cd len=8\ncooked=8\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
