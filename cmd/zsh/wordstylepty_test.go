// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// select-word-style in a startup file puts the -match widgets in place of the
// editor's own, and the keys a person presses then move, kill and change by
// the style: two backward kills in a row join into one kill that a yank
// brings back whole, the bash style stops at the `-` in `foo-bar`, and a
// forward move ends at the end of the word.
//
// Measured 2026-10-04 through a pseudo-terminal against /opt/homebrew/bin/zsh
// (zsh 5.9.2) with its own copies, the same startup file and the same keys:
// every recorded line below is what zsh recorded.
func TestTheWordStyleWidgetsAreTheKeys(t *testing.T) {
	control, screen, home := contribSession(t, `autoload -Uz select-word-style
select-word-style bash
bindkey '^Xh' backward-kill-word
bindkey '^Xb' backward-word
bindkey '^Xf' forward-word
bindkey '^Xu' up-case-word
`)
	contribSend(t, control, "foo-bar baz")
	contribAwaitRow(t, screen, "foo-bar baz")
	for i, step := range []struct{ keys, want string }{
		{"\x18h\x18h", `$'foo-'|4`},
		{"\x19", `$'foo-bar baz'|11`},
		{"\x18b", `$'foo-bar baz'|8`},
		{"\x18b", `$'foo-bar baz'|4`},
		{"\x18f", `$'foo-bar baz'|7`},
		{"\x18u", `$'foo-bar BAZ'|11`},
	} {
		contribSend(t, control, step.keys)
		if got := contribRecorded(t, control, screen, home, i+1); got != step.want {
			t.Errorf("after %q: %q, want %q", step.keys, got, step.want)
		}
	}
}
