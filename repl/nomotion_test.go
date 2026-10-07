// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"testing"

	"github.com/blairham/sh/internal/terminfofixture"
)

// A terminal that cannot move the cursor right is one with no description,
// or one whose description has neither `cuf1` nor `cuf` — `dumb`'s has
// neither. See nomotion.go (#6314).
func TestATerminalWithoutCursorRightIsDrawnWithoutMotion(t *testing.T) {
	right := index(t, terminfoStringNames, "cuf1")
	db := terminfofixture.Database(t,
		terminfofixture.Description{Name: "moves", StrCount: right + 1, Strs: map[int]string{right: "\x1b[C"}},
		terminfofixture.Description{Name: "stays"},
	)
	for _, c := range []struct {
		term string
		want bool
	}{
		{"moves", false},
		{"stays", true},
		{"unknownterm", true},
	} {
		t.Run(c.term, func(t *testing.T) {
			s := Shell{
				Runner: newTestRunner(map[string]string{"TERM": c.term, "TERMINFO": db}),
				Editor: EditorStyle{DrawsWithoutCursorMotion: true},
				counts: &counts{},
			}
			if got := s.cannotMoveTheCursor(); got != c.want {
				t.Errorf("cannot move = %v, want %v", got, c.want)
			}
		})
	}
	t.Run("a dialect that does not ask", func(t *testing.T) {
		s := Shell{Runner: newTestRunner(map[string]string{"TERM": "stays", "TERMINFO": db}), counts: &counts{}}
		if s.cannotMoveTheCursor() {
			t.Error("a dialect without the setting drew without motion")
		}
	})
}
