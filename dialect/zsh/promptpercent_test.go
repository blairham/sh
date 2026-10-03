// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestPromptPercentOffLeavesTheEscapesAsText pins `promptpercent`. Measured
// 2026-10-02 on zsh 5.9.2 under `-f` (#5155). See
// interp.Runner.SetPromptPercent.
func TestPromptPercentOffLeavesTheEscapesAsText(t *testing.T) {
	dir := t.TempDir()
	cases := []struct{ src, want string }{
		{`unsetopt promptpercent; print -P '%/ %% %!'`, "%/ %% %!\n"},
		{`print -P '%%'`, "%\n"},
		{"unsetopt promptpercent; print ${(%%):-%%}", "%%\n"},
		{"unsetopt promptpercent; print ${(%):-%%}", "%\n"},
		{`unsetopt promptpercent; PS4='%N+ '; set -x; : x`, "%N+ : x\n"},
		{"unsetopt promptpercent; setopt promptbang; print -P '!'", "0\n"},
		{"unsetopt promptpercent; [[ -o promptpercent ]] || print off", "off\n"},
	}
	for _, c := range cases {
		if got, _ := runZsh(t, dir, c.src); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
