// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **`read` keeps a NUL as a byte of the value** — see
// interp.Semantics.NulInAValue. Measured against zsh 5.9.2.
func TestReadKeepsANul(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `printf 'A=1\0B=2\0c\0d\n' > f
read -r v < f; echo "raw=$v len=${#v}"
read v < f; echo "cooked=${#v}"`)
	if want := "raw=A=1\x00B=2\x00c\x00d len=11\ncooked=11\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
