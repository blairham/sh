// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// A refused `set` option is echoed back with the sign it was asked with, for
// the letter and for the long name alike.
//
// Measured 2026-10-04 in the digest-pinned alpine image, BusyBox v1.37.0:
// `set +B` is `illegal option +B`, `set +o zzz` is `illegal option +o zzz`,
// and `set -o emacs` beside `set +o emacs` writes one of each. Until #5723
// both wordings wrote `-` whatever was asked, which is dash's answer and not
// this shell's.
func TestASetRefusalEchoesTheSign(t *testing.T) {
	for _, tc := range []struct{ script, want string }{
		{"set +q\n", "illegal option +q"},
		{"set -q\n", "illegal option -q"},
		{"set +o zzznosuch\n", "illegal option +o zzznosuch"},
		{"set -o zzznosuch\n", "illegal option -o zzznosuch"},
	} {
		t.Run(tc.script, func(t *testing.T) {
			out, _ := run(t, tc.script)
			if !strings.Contains(out, tc.want) {
				t.Errorf("%q said %q, want %q in it", tc.script, out, tc.want)
			}
		})
	}
}
