// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestShortRepeatKeepsRepeatsShortBody pins `shortrepeat`. Measured
// 2026-10-02 on zsh 5.9.2 under `-f` (#5155): with `shortloops` off, the
// option gives `repeat` alone its one-command body back.
func TestShortRepeatKeepsRepeatsShortBody(t *testing.T) {
	cases := []struct{ src, want string }{
		{"unsetopt shortloops; setopt shortrepeat; eval 'repeat 3 print n'", "n\nn\nn\n"},
		{"unsetopt shortloops shortrepeat; eval 'repeat 3 print n'", "(eval):1: parse error near `print'\n"},
		{"unsetopt shortloops; setopt shortrepeat; eval 'for f in a b; print $f'", "(eval):1: parse error near `print'\n"},
		{"unsetopt shortloops; setopt shortrepeat; [[ -o shortrepeat ]] && print on", "on\n"},
	}
	for _, c := range cases {
		if got, _ := runZshOnPath(t, t.TempDir(), c.src); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
