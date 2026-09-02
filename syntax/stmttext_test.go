// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// textBetween is bounds-checked rather than trusting its arguments, and this
// is why the check is not dead code: a Pos is only as good as whatever
// produced it, and this runs on the keystroke path, where a panic is the one
// outcome the parser may never have. Nothing in the parser can reach these —
// the positions it passes are its own tokens over its own input — so a test
// is the only thing that can.
func TestTextBetweenRefusesPositionsItCannotUse(t *testing.T) {
	p := NewParser("echo hi", Core())
	for _, tc := range []struct {
		name     string
		from, to Pos
	}{
		{"past the end", Pos{Offset: 0}, Pos{Offset: 999}},
		{"negative", Pos{Offset: -1}, Pos{Offset: 4}},
		{"inverted", Pos{Offset: 5}, Pos{Offset: 2}},
		{"empty", Pos{Offset: 3}, Pos{Offset: 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.textBetween(tc.from, tc.to); got != "" {
				t.Errorf("textBetween = %q, want nothing", got)
			}
		})
	}
	if got := p.textBetween(Pos{Offset: 0}, Pos{Offset: 4}); got != "echo" {
		t.Errorf("textBetween = %q, want the text it can use", got)
	}
}
