// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "testing"

// The width table is the one thing here settled against the standard rather
// than against a running shell, so these name the class each character is in
// and why, and not "what some terminal did".

func TestACharacterIsMeasuredInCells(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    rune
		want int
	}{
		{"latin", 'a', 1},
		{"a digit", '7', 1},
		{"a space", ' ', 1},
		{"cyrillic", 'ж', 1},
		{"greek", 'π', 1},
		{"CJK", '日', 2},
		{"hangul", 'ᄀ', 2},
		{"a fullwidth letter", 'ａ', 2},
		{"an emoji", '😀', 2},
		{"a combining acute", '́', 0},
		{"an enclosing circle", '⃝', 0},
		{"a control character", '\x01', 0},
		{"delete", '\x7f', 0},
		// Ambiguous width: one cell here, because two is right only in an
		// East Asian locale and that is the terminal's business.
		{"an ambiguous character", '§', 1},
		// Format characters are one cell: neither Mn nor Me, and the
		// implementations that give them zero disagree with each other.
		{"a zero-width space", '​', 1},
		{"a soft hyphen", '­', 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runeWidth(tc.r); got != tc.want {
				t.Errorf("runeWidth(%q) = %d, want %d", tc.r, got, tc.want)
			}
		})
	}
}

// TestALineIsMeasuredInCells is the whole point: the geometry the editor
// draws with counts columns, and a line of CJK is twice as wide as its rune
// count.
func TestALineIsMeasuredInCells(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"日本", 4},
		{"a日b", 4},
		{"é", 1},
		// An escape sequence occupies no cells, which was already true and
		// must stay true now that the counting changed.
		{"\x1b[31mred\x1b[0m", 3},
		{"\x1b[31m日\x1b[0m", 2},
	} {
		if got := displayWidth(tc.in); got != tc.want {
			t.Errorf("displayWidth(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
