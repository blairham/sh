// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "testing"

// How a control character in the line reaches the terminal, row by row of
// the measurements in controlglyph.go (#5972). Every expectation is the
// whole spelled string, so an added prefix or a stray reset cannot pass.
func TestAControlCharacterIsSpelledForTheTerminal(t *testing.T) {
	const on, off = "\x1b[7m", "\x1b[27m"
	for _, c := range []struct {
		name, line string
		col, cols  int
		want       string
	}{
		{"a caret in standout", "ab\x14cd", 3, 120, "ab" + on + "^T" + off + "cd"},
		{"delete and escape", "\x7f\x1b", 3, 120, on + "^?" + off + on + "^[" + off},
		{"a tab runs to the next stop of the row", "\t", 3, 120, "     "},
		{"from a later column, fewer", "x\t", 3, 120, "x    "},
		{"a tab stops at the edge", "\t", 117, 120, "   "},
		{"after a wrap the stops count from the new row", "xxx\tZ", 118, 120, "xxx       Z"},
		{"a newline is still a carriage return and a line feed", "a\nb\tc", 3, 120, "a\r\nb       c"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := spell(hideControls(c.line), c.col, c.cols, on, off); got != c.want {
				t.Errorf("spelled %q, want %q", got, c.want)
			}
		})
	}
}

// A caret inside a run the paste or a highlighter has open turns its own
// attribute off and the run's back on, so the run goes on after it — which is
// what zsh writes for a caret inside a pasted run.
func TestACaretInsideARunPutsTheRunBack(t *testing.T) {
	styled := "\x1b[7m" + hideControls("b\x01c") + "\x1b[27m"
	want := "\x1b[7mb\x1b[7m^A\x1b[27m\x1b[7mc\x1b[27m"
	if got := spell(styled, 0, 120, "\x1b[7m", "\x1b[27m"); got != want {
		t.Errorf("spelled %q, want %q", got, want)
	}
}

// And where things land: a caret is two cells that wrap between them, and a
// tab runs to its stop without wrapping. Measured against zsh 5.9.2 on a
// 120-column terminal after a three-cell prompt — see controlglyph.go.
func TestAControlCharacterIsPlacedInCells(t *testing.T) {
	x := func(n int) string {
		b := make([]rune, n)
		for i := range b {
			b[i] = 'x'
		}
		return string(b)
	}
	for _, c := range []struct {
		name           string
		line           string
		pos            int
		endRow, endCol int
		curRow, curCol int
	}{
		{"a caret is two cells", "ab\x14cd", 3, 0, 9, 0, 7},
		{"a caret wraps between its cells", x(116) + "\x14Z", 116, 1, 2, 0, 119},
		{"a tab to the edge leaves the wrap pending", x(114) + "\t", 115, 0, 120, 0, 120},
		{"a tab after a wrap counts from the new row", x(118) + "\tZ", 119, 1, 9, 1, 8},
	} {
		t.Run(c.name, func(t *testing.T) {
			curRow, curCol, endRow, endCol := place(3, []rune(c.line), c.pos, 120)
			if endRow != c.endRow || endCol != c.endCol || curRow != c.curRow || curCol != c.curCol {
				t.Errorf("cursor %d,%d end %d,%d; want cursor %d,%d end %d,%d",
					curRow, curCol, endRow, endCol, c.curRow, c.curCol, c.endRow, c.endCol)
			}
		})
	}
}
