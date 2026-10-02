// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestABraceClassReadsADollarSingleSpanAsItsValue: under `braceccl` a `$'…'`
// span in the body is the characters it stands for, so `{$'\0'-$'\5'}` is the
// six characters NUL to 5. Measured 2026-10-02 on zsh 5.9.2, E01options'
// `BRACE_CCL option starting from NUL` (#5155). It was the backslash, the
// digits and everything between `0` and `\`.
func TestABraceClassReadsADollarSingleSpanAsItsValue(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`setopt braceccl; for c in {$'\0'-$'\5'}; do print -n $(( #c )),; done`, "0,1,2,3,4,5,"},
		{`setopt braceccl; print -r -- {$'\t'b}`, "\t b\n"},
		{`setopt braceccl; print -r -- {a$'\x2d'c}`, "a b c\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}
