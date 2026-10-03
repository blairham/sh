// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestAScalarsSubscriptIsEvaluatedOnce pins that a subscript on a scalar
// runs its expression once, so a step inside it steps once (#5659). Measured
// 2026-10-03 on ksh93u+ 2012-08-01.
func TestAScalarsSubscriptIsEvaluatedOnce(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`i=0; s=abc; echo "${s[++i]}" $i`, " 1\n"},
		{`i=0; s=abc; echo "${s[i++]}${s[i++]}" $i`, "abc 2\n"},
		{`i=-1; s=abc; echo "${s[++i]}" $i`, "abc 0\n"},
	} {
		got, _ := runKsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
