// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"
)

// This shell has the same letter and means the same option by it (#1856).
//
// Measured on ksh93 (AJM 93u+ 2012-08-01), 2026-09-11: `set +B; echo {a,b}`
// writes `{a,b}`, `set +o braceexpand` does the same, `set -B` puts the
// expansion back and `$-` loses the letter while it is off. Ours refused the
// letter as not implemented and expanded the braces anyway.
func TestBraceExpansionCanBeTurnedOff(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{"the letter", "set +B; echo {a,b}\n", "{a,b}\n"},
		{"the long name", "set +o braceexpand; echo {a,b}\n", "{a,b}\n"},
		{"and back again", "set +B; set -B; echo {a,b}\n", "a b\n"},
		{"the letter leaves `$-`", "set +B; echo $-", "h\n"},
		{"and comes back with it", "set +B; set -B; echo $-", "hB\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runKsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("%s gave %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}
