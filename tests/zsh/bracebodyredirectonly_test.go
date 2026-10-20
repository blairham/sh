// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestACloseBraceEndsARedirectionOnlyCommand pins `}` closing a brace body
// whose last statement is redirections alone. Measured 2026-10-02 on zsh
// 5.9.2 under `-f` (#5155).
func TestACloseBraceEndsARedirectionOnlyCommand(t *testing.T) {
	cases := []struct{ src, want string }{
		{"{ print hello | >foo }; /bin/cat foo", "hello\n"},
		{"{ >foo }; [[ -e foo ]] && print made", "made\n"},
		{"f(){ >foo }; f; [[ -e foo ]] && print made", "made\n"},
	}
	for _, c := range cases {
		if got, _ := runZshOnPath(t, t.TempDir(), c.src); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
