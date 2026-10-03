// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// zsh writes `+p` exactly as it writes `-p`, value included. Measured
// 2026-10-03 on zsh 5.9 under `-c` (#5642). See
// interp.Semantics.PlusSignedPrintListsNoValues.
func TestAPlusPrintListsAsTheMinusDoes(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`x=/y; typeset +p x`, "typeset x=/y\n"},
		{`typeset -i n=1; typeset +p n`, "typeset -i n=1\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}
