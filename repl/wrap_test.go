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
	e := &editor{in: strings.NewReader(keys), out: &out, width: func() int { return cols }}
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
	// A prompt with no space in it, so the space being looked for can only be
	// the one that forces the wrap. With "$ " it would be found in the prompt
	// itself and the test would pass whether the code did this or not.
	const prompt = "#"
	// One column of prompt and nine characters is exactly ten columns.
	out := typedAtWith(t, 10, prompt, "123456789\r")
	if !strings.Contains(out, "123456789 \r") {
		t.Errorf("want the wrap forced past the last character, got %q", out)
	}
	// And a line one short of the edge does not force it.
	short := typedAtWith(t, 10, prompt, "12345678\r")
	if strings.Contains(short, "12345678 \r") {
		t.Errorf("forced a wrap that was not needed: %q", short)
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
	out := typedAtWith(t, 20, "ab> ", "xyz\x01\r")
	if !strings.Contains(out, "\r\x1b[4C") {
		t.Errorf("want the cursor moved back to column 4, past the prompt, got %q", out)
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
	e := &editor{in: strings.NewReader("hi\r"), out: &out}
	if _, err := e.readLine(drawPrompt("$ ")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\r\x1b[K") {
		t.Errorf("want the single-row redraw, got %q", out.String())
	}
}
