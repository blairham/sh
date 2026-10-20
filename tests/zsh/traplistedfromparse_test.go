// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestATrapIsListedFromItsParse pins that `trap` writes an action back from
// its parse, in the function listing's arrangement, and quotes it in the
// escaped style. Measured 2026-10-02 on zsh 5.9.2 under `-f` (#5409).
func TestATrapIsListedFromItsParse(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"trap 'print E; trap' USR1; trap", "trap -- $'print E\\ntrap' USR1\n"},
		{"trap 'if true; then print a; fi' USR1; trap", "trap -- $'if true\\nthen\\n\\tprint a\\nfi' USR1\n"},
		{"trap 'x=1   y=2' USR1; trap", "trap -- 'x=1 y=2 ' USR1\n"},
		{"trap '  print a  # c' USR1; trap", "trap -- 'print a' USR1\n"},
		{`trap 'print "it'\''s"' USR1; trap`, `trap -- 'print "it'\''s"' USR1` + "\n"},
		{"trap true USR1; trap", "trap -- true USR1\n"},
		{"trap '' USR1; trap", "trap -- '' USR1\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
