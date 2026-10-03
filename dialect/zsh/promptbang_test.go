// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestPromptBangDrawsTheHistoryNumber pins `promptbang`. Measured 2026-10-02
// on zsh 5.9.2 under `-f`, where the history number is 0 (#5155). See
// interp.promptBangText.
func TestPromptBangDrawsTheHistoryNumber(t *testing.T) {
	cases := []struct{ src, want string }{
		{"setopt promptbang; print -P !", "0\n"},
		{"print -P !", "!\n"},
		{`setopt promptbang; print -P 'a!!b'`, "a!b\n"},
		{`setopt promptbang; print -P '%%!'`, "%0\n"},
		{`setopt promptbang; print -P '%(!.a.b)'`, "b\n"},
		{"setopt promptbang; print ${(%):-!}", "!\n"},
		{"setopt promptbang; print ${(%%):-!}", "0\n"},
		{`setopt promptbang promptsubst; x='!'; print -P '[$x]'`, "[0]\n"},
		{`setopt promptbang; PS4='!> '; set -x; : x`, "0> : x\n"},
	}
	for _, c := range cases {
		if got, _ := runZsh(t, t.TempDir(), c.src); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
