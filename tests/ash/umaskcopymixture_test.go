// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// A permission copy is taken **alone** here: beside a permission letter, in
// either order, the whole operand is `illegal mode` at 2 and the mask is left
// where it was (#3074). bash throws the letters away and dash ORs the copy in;
// this column refuses the mixture outright.
//
// Measured 2026-09-18 inside the pinned image
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b,
// BusyBox v1.37.0, a script file under `env -i LC_ALL=C`.
func TestUmaskRefusesACopyBesideLetters(t *testing.T) {
	for _, tc := range []struct {
		src     string
		refused bool
	}{
		{"umask 222\numask -S g=uw", true},
		{"umask 222\numask -S g=wu", true},
		// The portable spelling on its own is taken, which is what says the
		// refusal is about the mixture and not about the copy.
		{"umask 222\numask -S g=u", false},
		// And letters on their own are untouched.
		{"umask 222\numask -S g=w", false},
	} {
		out, st := runUmask(t, tc.src)
		if tc.refused != (st != 0) {
			t.Errorf("%s: out %q status %d, refused=%v want %v", tc.src, out, st, st != 0, tc.refused)
		}
		if tc.refused && !strings.Contains(out, "illegal mode: ") {
			t.Errorf("%s: out %q, want the whole operand named", tc.src, out)
		}
	}
}
