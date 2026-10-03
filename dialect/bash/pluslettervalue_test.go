// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// bash takes a plus-signed letter off before the line's value lands. Measured
// 2026-10-03 on bash 5.3.20 under `-c` (#5663). See
// interp.Semantics.PlusLetterComesOffAfterTheValueLands.
func TestAPlusLetterComesOffBeforeTheValueLands(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`declare -l s=A; declare +l s=B; declare -p s`, "declare -- s=\"B\"\n"},
		{`declare -u s=a; declare +u s=b; declare -p s`, "declare -- s=\"b\"\n"},
	} {
		if out, st := runBash(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}
