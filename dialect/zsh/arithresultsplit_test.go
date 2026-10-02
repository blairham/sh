// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnArithmeticResultIsNeverSplit is #5157's E03posix row `IFS applies to
// math results`, which the reference's own suite marks as an expected
// failure: zsh does not field-split an arithmetic expansion's result. We split
// it whenever shwordsplit was on, and so the row passed when it should have
// failed. See interp.Semantics.ArithExpansionIsSplit.
//
// Every row measured 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`).
func TestAnArithmeticResultIsNeverSplit(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`IFS=2; printf '<%s>\n' $((11*11))`, "<121>\n"},
		{`IFS=2; setopt shwordsplit; printf '<%s>\n' $((11*11)) $[11*11] $((11*11))x`, "<121>\n<121>\n<121x>\n"},
		{`emulate sh; IFS=2; printf '<%s>\n' $((11*11))`, "<121>\n"},
		// The control: a parameter holding the same text does split under
		// the option, so the option is on and the split is the axis's.
		{`IFS=2; setopt shwordsplit; x=121; printf '<%s>\n' $x`, "<1>\n<1>\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), c.src+"\n")
		if out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}
