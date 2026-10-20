// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// zsh takes a plus-signed letter off before the line's value lands. Measured
// 2026-10-03 on zsh 5.9 under `-c` (#5663). See
// interp.Semantics.PlusLetterComesOffAfterTheValueLands.
func TestAPlusLetterComesOffBeforeTheValueLands(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -l s=A; typeset +l s=B; typeset -p s`, "typeset s=B\n"},
		{`typeset -u s=a; typeset +u s=b; typeset -p s`, "typeset s=b\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}
