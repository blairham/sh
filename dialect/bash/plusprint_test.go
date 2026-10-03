// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// bash writes `+p` exactly as it writes `-p`, value included. Measured
// 2026-10-03 on bash 5.3.20 under `-c` (#5642). See
// interp.Semantics.PlusSignedPrintListsNoValues.
func TestAPlusPrintListsAsTheMinusDoes(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`x=/y; declare +p x`, "declare -- x=\"/y\"\n"},
		{`declare -i n=1; declare +p n`, "declare -i n=\"1\"\n"},
	} {
		if out, st := runBash(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}
