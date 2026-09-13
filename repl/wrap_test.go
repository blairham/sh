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

// A line wider than the terminal occupies more than one screen row, and the
// redraw has to come back up to the prompt before it can draw over what is
// there.
//
// `\r` returns to the start of the row the cursor is on, not to the start of
// the line, and `\x1b[K` clears that row and no other. Together they were the
// whole of the old redraw, which is right up to the moment a line wraps and
// wrong for every keystroke after it: the earlier rows keep whatever was on
// them.
func TestARedrawComesBackUpToThePrompt(t *testing.T) {
	// Ten columns, so "$ " and eight characters fill the first row exactly
	// and the ninth is on the second.
	out := typedAt(t, 10, "123456789\r")
	if !strings.Contains(out, "\x1b[1A") {
		t.Errorf("no move back up to the prompt row in %q", out)
	}
	if !strings.Contains(out, "\x1b[J") {
		t.Errorf("no erase to the end of the screen in %q", out)
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
	// A prompt with no space in it, so nothing can be satisfied by a space
	// the prompt happened to contain. One column of prompt and nine
	// characters is exactly ten columns.
	const prompt = "#"

	// Asked of the screen rather than of the bytes. What matters is where the
	// *next* character lands: if the deferred wrap is mishandled the tenth
	// character goes on the row that is already full, or the row count after
	// it is out by one. Typing one more character is what asks the question,
	// and it cannot be answered by a shell that merely wrote a space.
	full := newScreen(10).feed(typedAtWith(t, 10, prompt, "123456789X\r"))
	if got := full.text(); got != "#123456789\nX" {
		t.Errorf("the character past the edge did not start a new row:\n got %q\nwant %q",
			got, "#123456789\nX")
	}

	// And a line one short of the edge keeps the next character on the row.
	short := newScreen(10).feed(typedAtWith(t, 10, prompt, "12345678X\r"))
	if got := short.text(); got != "#12345678X" {
		t.Errorf("wrapped a line that still had a column left:\n got %q\nwant %q",
			got, "#12345678X")
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
	// ^A and then a character. Where the cursor *is* is asked by putting
	// something there: land it at column 4 and the line reads "wxyz", land it
	// on top of the prompt and it does not. A row that read the escape bytes
	// instead asserted one spelling of the move — `\r` and a count from the
	// left-hand edge — and failed for a draw that stepped there relatively,
	// which is the same cell by a shorter road.
	out := typedAtWith(t, 20, "ab> ", "xyz\x01w\r")
	if got := newScreen(20).feed(out).text(); got != "ab> wxyz" {
		t.Errorf("^A did not put the cursor at the start of the line, past the prompt:\n got %q\nwant %q",
			got, "ab> wxyz")
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
