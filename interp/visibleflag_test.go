// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"testing"
)

// `${(V)x}`, against zsh 5.9.2 one control character at a time.
//
// The whole range rather than a few, because the rule is not the one the
// two escapes suggest: tab and newline get `\t` and `\n` and **every other**
// control character gets a caret, including the three that have a C escape
// of their own. A test naming only tab, newline and escape would pass for
// an implementation that wrote `\v`, `\f` and `\r`.
func TestTheVisibleFlagSpellsEveryControlCharacterAsZshDoes(t *testing.T) {
	for c := 1; c <= 31; c++ {
		want := fmt.Sprintf("a^%cb", c^0x40)
		switch c {
		case '\t':
			want = `a\tb`
		case '\n':
			want = `a\nb`
		}
		if got := visibleText(fmt.Sprintf("a%cb", c)); got != want {
			t.Errorf("visibleText(a\\x%02x b) = %q, want %q", c, got, want)
		}
	}
	if got, want := visibleText("a\x7fb"), "a^?b"; got != want {
		t.Errorf("delete = %q, want %q", got, want)
	}
}

// What it leaves alone, which is the half that separates it from a quoting
// flag: the backslash is **not** doubled, and a multi-byte character is not
// touched — every byte this rewrites is below \x80 and every byte of one of
// those is above it.
func TestTheVisibleFlagLeavesTextAlone(t *testing.T) {
	for _, s := range []string{"", "abc", `a\b`, `a\\b`, "héllo", "日本語", "a%F{red}b"} {
		if got := visibleText(s); got != s {
			t.Errorf("visibleText(%q) = %q, want it unchanged", s, got)
		}
	}
}
