// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// `zle_highlight`'s paste context, against the table on PastedTextStyle
// (#6271).
func TestThePasteStyleIsZleHighlights(t *testing.T) {
	for _, c := range []struct{ set, on, off string }{
		{"", "\x1b[7m", "\x1b[27m"},
		{"zle_highlight=(region:bold)", "\x1b[7m", "\x1b[27m"},
		{"zle_highlight=(paste:none)", "", ""},
		{"zle_highlight=(paste:bold)", "\x1b[1m", "\x1b[0m"},
		{"zle_highlight=(paste:underline,standout)", "\x1b[7m\x1b[4m", "\x1b[27m\x1b[24m"},
		{"zle_highlight=(paste:fg=red,bold)", "\x1b[1m\x1b[31m", "\x1b[0m\x1b[39m"},
		{"zle_highlight=(paste:bg=blue)", "\x1b[44m", "\x1b[49m"},
		{"zle_highlight=(paste:fg=196)", "\x1b[38;5;196m", "\x1b[39m"},
		{"zle_highlight=(paste:fg=#ff0000)", "\x1b[38;2;255;0;0m", "\x1b[39m"},
		{"zle_highlight=(paste:none,bold)", "\x1b[1m", "\x1b[0m"},
		{"zle_highlight=(paste:standout,none)", "", ""},
		{"zle_highlight=(paste:bg=blue,fg=red,underline)", "\x1b[4m\x1b[31m\x1b[44m", "\x1b[24m\x1b[39m\x1b[49m"},
	} {
		r := bindkeyRunner(t, c.set+"\n")
		if on, off := zsh.PastedTextStyle(r); on != c.on || off != c.off {
			t.Errorf("%s: got %q %q, want %q %q", c.set, on, off, c.on, c.off)
		}
	}
}
