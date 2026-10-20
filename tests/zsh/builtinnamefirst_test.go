// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestBuiltinLooksItsNameUpBeforeMatching pins that `builtin` refuses a name
// naming no builtin before any word behind it is matched against the
// filesystem. Measured 2026-10-02 on zsh 5.9.2 under `-f` in an empty
// directory (#5377).
func TestBuiltinLooksItsNameUpBeforeMatching(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"builtin -a nomatch*; echo st=$?", "zsh:1: no such builtin: -a\nst=1\n"},
		{"builtin nosuch nomatch*; echo st=$?", "zsh:1: no such builtin: nosuch\nst=1\n"},
		{"c=(builtin nosuch); $c nomatch*; echo st=$?", "zsh:1: no such builtin: nosuch\nst=1\n"},
		// The controls: a real builtin's words are matched, and `command`
		// matches its words first.
		{"builtin echo nomatch*", "zsh:1: no matches found: nomatch*\n"},
		{"command -a nomatch*", "zsh:1: no matches found: nomatch*\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
