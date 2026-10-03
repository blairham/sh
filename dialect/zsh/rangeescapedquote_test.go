// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestARangeSegmentHoldingAnEscapedQuoteIsAModifier pins that a substring
// range segment that opens on a nested `${` holding an escaped double quote
// is read as a modifier list, and refused naming its `$` (#5151, a chunk of
// D04parameter.ztst). Measured 2026-10-03 on zsh 5.9.2 under `-f`.
func TestARangeSegmentHoldingAnEscapedQuoteIsAModifier(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`: "${foo:0:${\"}}"; echo after`, "zsh:1: unrecognized modifier `$'\n"},
		{`: ${foo:${\"}}`, "zsh:1: unrecognized modifier `$'\n"},
		{`: ${foo:0:${x:-\"}}`, "zsh:1: unrecognized modifier `$'\n"},
		{`: ${(U)foo:0:${\"}}`, "zsh:1: unrecognized modifier `$'\n"},
		{`a=(x y); : ${(U)a:${\"}}`, "zsh:1: unrecognized modifier `$'\n"},
		{`a=(x y); : ${(U)a:0:${\"}}`, "zsh:1: unrecognized modifier `$'\n"},
		// The controls: other broken spellings, and ordinary ranges.
		{`: ${foo:0:${\\}}`, "zsh:1: bad substitution\n"},
		{`foo=abc; x=2; print ${foo:0:${x}} ${foo:1:$(echo 1)}`, "ab b\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
