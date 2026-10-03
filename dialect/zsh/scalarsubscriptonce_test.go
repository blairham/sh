// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAScalarsSubscriptIsEvaluatedOnce pins that a subscript on a scalar
// runs its expression once, so a step inside it steps once (#5659). Measured
// 2026-10-03 on zsh 5.9.2 under -f.
func TestAScalarsSubscriptIsEvaluatedOnce(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`i=1; s=abc; print ${s[++i]} $i`, "b 2\n"},
		{`i=1; s=abc; print ${${s}[++i]} $i; x=${s[i++]}; print $x $i`, "b 2\nb 3\n"},
		{`i=0; s=abc; print ${s[++i]}${s[++i]} $i`, "ab 2\n"},
		// A range has two ends, and each is read once.
		{`i=0; s=abc; print ${s[++i,++i]} $i`, "ab 2\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
