// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// typedAt runs a line through an editor that believes the terminal is cols
// wide.
func typedAt(t *testing.T, cols int, keys string) string {
	t.Helper()
	return typedAtWith(t, cols, "$ ", keys)
}

func typedAtWith(t *testing.T, cols int, prompt, keys string) string {
	t.Helper()
	var out strings.Builder
	e := &editor{in: typing(keys), out: &out, width: func() int { return cols }}
	if _, err := e.readLine(drawPrompt(prompt)); err != nil {
		t.Fatalf("readLine: %v", err)
	}
	return out.String()
}

// A line wider than the terminal occupies more than one screen row, and a
// redraw has to reach every row of it.
//
// `\r` returns to the start of the row the cursor is on, not to the start of
// the line, and `\x1b[K` clears that row and no other. Together they were the
// whole of the oldest redraw, which is right up to the moment a line wraps and
// wrong for every keystroke after it: the earlier rows keep whatever was on
// them.
//
// Asserted on the screen rather than on the escape sequence that gets there.
// A redraw that writes only the difference goes back up a row when the change
// is on the row above and does not when it is not, so looking for the move
// itself would be looking for one of several right answers — see
// screenmodel_test.go.
func TestAWrappedLineIsRightOnEveryRow(t *testing.T) {
	// Ten columns, so "$ " and eight characters fill the first row exactly
	// and the ninth is on the second. The insertion is at the *start*, which
	// shifts every character on both rows.
	s := drawnScreen(t, 10, "$ ", []edit{
		{"123456789", 9},
		{"123456789", 0},
		{"x123456789", 1},
	})
	if got, want := s.text(), "$ x1234567\n89"; got != want {
		t.Errorf("screen is\n%q\nwant\n%q", got, want)
	}
	if row, col := s.at(); row != 0 || col != 3 {
		t.Errorf("cursor at row %d column %d, want row 0 column 3", row, col)
	}
}

// And the newline at the end has to come from below the last row of it.
//
// The cursor is left wherever the line was being edited. Sending a newline
// from the middle of a wrapped line puts the command's output on top of the
// rows below.
func TestTheLineEndsBelowItsLastRow(t *testing.T) {
	// Cursor sent home with ^A, so it is on the first row while the line
	// runs onto the second.
	out := typedAt(t, 10, "123456789\x01\r")
	i := strings.LastIndex(out, "\x1b[1B")
	j := strings.LastIndex(out, "\r\n")
	if i < 0 || i > j {
		t.Errorf("want a move down before the final newline, got %q", out)
	}
}

// A line that fits needs none of it.
func TestAShortLineStaysOnItsRow(t *testing.T) {
	out := typedAt(t, 80, "hi\r")
	if strings.Contains(out, "A") && strings.Contains(out, "\x1b[1A") {
		t.Errorf("moved rows for a line that fits: %q", out)
	}
}

// A terminal defers the wrap until there is something to put on the next row,
// so a line ending exactly at the right-hand edge leaves the cursor on the
// row it filled. Everything counted from there would be one row out.
func TestALineEndingAtTheEdgeIsWrapped(t *testing.T) {
	// A prompt with no space in it, so the space being looked for can only be
	// the one that forces the wrap. With "$ " it would be found in the prompt
	// itself and the test would pass whether the code did this or not.
	const prompt = "#"
	// One column of prompt and nine characters is exactly ten columns, so the
	// cursor belongs at the start of the row below rather than on the row it
	// filled.
	full := drawnScreen(t, 10, prompt, []edit{{"12345678", 8}, {"123456789", 9}})
	if row, col := full.at(); row != 1 || col != 0 {
		t.Errorf("cursor at row %d column %d, want row 1 column 0", row, col)
	}
	// And a line one short of the edge stays on the row.
	short := drawnScreen(t, 10, prompt, []edit{{"1234567", 7}, {"12345678", 8}})
	if row, col := short.at(); row != 0 || col != 9 {
		t.Errorf("cursor at row %d column %d, want row 0 column 9", row, col)
	}
}

// The cursor is placed from the left-hand edge, so where it lands includes
// the prompt.
//
// ^A is the case that says so: the cursor goes to the start of the *line*,
// which is not the start of the row — the prompt is still to the left of it.
// A count that left the prompt out would put the cursor on top of it, and
// every key typed after that would insert in the wrong place.
func TestTheCursorIsPlacedPastThePrompt(t *testing.T) {
	s := drawnScreen(t, 20, "ab> ", []edit{{"xyz", 3}, {"xyz", 0}})
	if row, col := s.at(); row != 0 || col != 4 {
		t.Errorf("cursor at row %d column %d, want row 0 column 4, past the prompt", row, col)
	}
}

// Escape sequences instruct the terminal rather than filling a cell, so a
// colored prompt is not as wide as it is long.
func TestDisplayWidthSkipsEscapeSequences(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		want     int
	}{
		{"plain", "$ ", 2},
		{"color around it", "\x1b[32m$\x1b[0m ", 2},
		{"one rune is one cell", "héllo", 5},
		{"nothing but escapes", "\x1b[1;32m\x1b[0m", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := displayWidth(tc.in); got != tc.want {
				t.Errorf("displayWidth(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// With no width to be had — a terminal that will not say — the editor still
// has to draw, and draws the way it always did.
func TestWithoutAWidthTheOldDrawingStands(t *testing.T) {
	var out strings.Builder
	e := &editor{in: typing("hi\r"), out: &out}
	if _, err := e.readLine(drawPrompt("$ ")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\r\x1b[K") {
		t.Errorf("want the single-row redraw, got %q", out.String())
	}
}

// A line that ends exactly at the right-hand edge has a row below it, and the
// newline still has to come from there.
//
// The forced wrap put a space on that row, so it is drawn even though the
// arithmetic that counts the line stops at the edge. Counting the edge as the
// row above leaves the next thing printed on top of it.
func TestALineEndingAtTheEdgeEndsBelowThatRow(t *testing.T) {
	// Two columns of prompt and eight characters is exactly ten, and ^A puts
	// the cursor back onto the first row.
	out := typedAtWith(t, 10, "$ ", "12345678\x01\r")
	i := strings.LastIndex(out, "\x1b[1B")
	j := strings.LastIndex(out, "\r\n")
	if i < 0 || i > j {
		t.Errorf("want a move down before the final newline, got %q", out)
	}
}
