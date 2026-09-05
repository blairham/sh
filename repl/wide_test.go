// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// A character wider than one cell, and the arithmetic that has to know it.
//
// The width table has been here since the prompt learned to wrap, and until
// now the editor did not consult it: the redraw counted the line in characters
// and the cursor in characters, so one `日` put everything after it one column
// to the left of where the terminal drew it. Typing a path after a CJK
// argument left the cursor sitting inside the text rather than after it, and
// on a wrapped line the redraw came back up to the wrong row and painted the
// prompt into the middle of the command.

// place is where the screen arithmetic lives, so it is worth asking directly.
func TestWhereTheLineLandsOnTheScreen(t *testing.T) {
	for _, c := range []struct {
		name           string
		prompt         int
		line           string
		pos, cols      int
		curRow, curCol int
		endRow, endCol int
	}{
		{"a short line", 2, "abc", 3, 10, 0, 5, 0, 5},
		{"the cursor in the middle", 2, "abcd", 2, 10, 0, 4, 0, 6},
		// Two cells each, so three of them are six columns and not three.
		{"wide characters count double", 2, "日本語", 1, 20, 0, 4, 0, 8},
		// Exactly at the right-hand edge: the wrap has not happened yet.
		{"filling the row exactly", 2, "12345678", 8, 10, 0, 10, 0, 10},
		// And a wide character that will not fit in the last cell is put on
		// the next row whole, leaving that cell blank. Counting cells and
		// dividing would put it one column to the left for the rest of the
		// line.
		{"a wide character that will not fit", 2, "1日日日日", 5, 10, 1, 2, 1, 2},
		// And the cursor sitting in front of that character is where writing
		// the line up to it would leave it: the blank cell the wrap skipped,
		// on the row above, and not on the character itself.
		{"the cursor in front of one that will not fit", 2, "1日日日日", 4, 10, 0, 9, 1, 2},
		{"a combining mark takes no cell", 2, "éx", 3, 10, 0, 4, 0, 4},
		// A character wider than the whole terminal. Nothing sensible is on
		// the screen at that point, and the count is kept inside the row so
		// that the rows after it are not pushed further out still.
		{"a terminal narrower than the character", 0, "日", 1, 1, 1, 1, 1, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			curRow, curCol, endRow, endCol := place(c.prompt, []rune(c.line), c.pos, c.cols)
			if curRow != c.curRow || curCol != c.curCol {
				t.Errorf("cursor at row %d column %d, want row %d column %d", curRow, curCol, c.curRow, c.curCol)
			}
			if endRow != c.endRow || endCol != c.endCol {
				t.Errorf("line ends at row %d column %d, want row %d column %d", endRow, endCol, c.endRow, c.endCol)
			}
		})
	}
}

// The cursor is moved in columns, so the redraw has to ask for the columns and
// not for the characters.
func TestTheCursorIsPlacedInCellsAndNotCharacters(t *testing.T) {
	var out strings.Builder
	e := &editor{out: &out, width: func() int { return 20 }}
	e.line, e.pos = []rune("日本語"), 1
	e.redraw(drawPrompt("$ "))
	// Two columns of prompt and one wide character is four, not three.
	if !strings.Contains(out.String(), "\r\x1b[4C") {
		t.Errorf("drew %q, want the cursor put four columns in", out.String())
	}

	// And the same on a terminal that will not say how wide it is, where the
	// cursor is walked back from the end instead.
	out.Reset()
	e = &editor{out: &out}
	e.line, e.pos = []rune("日本語"), 1
	e.redraw(drawPrompt("$ "))
	if !strings.Contains(out.String(), "\x1b[4D") {
		t.Errorf("drew %q, want the cursor walked back four columns", out.String())
	}
}

// A line of wide characters wraps at half as many of them, and the redraw has
// to come back up over every row it drew.
func TestAWrappedLineOfWideCharacters(t *testing.T) {
	// Ten columns, two of prompt: four wide characters fill the first row and
	// the fifth is on the second.
	out := typedAtWith(t, 10, "$ ", "日本語日本\r")
	if !strings.Contains(out, "\x1b[1A") {
		t.Errorf("no move back up to the prompt row in %q", out)
	}
	// Four of them is eight columns and fits; nothing should move rows.
	short := typedAtWith(t, 10, "$ ", "日本語日\r")
	if strings.Contains(short, "\x1b[1A") {
		t.Errorf("moved rows for a line that fits: %q", short)
	}
}

// And the newline at the end still comes from below the last row of it.
func TestAWideLineEndsBelowItsLastRow(t *testing.T) {
	// The cursor is sent home while the line runs onto a second row.
	out := typedAtWith(t, 10, "$ ", "日本語日本\x01\r")
	i := strings.LastIndex(out, "\x1b[1B")
	j := strings.LastIndex(out, "\r\n")
	if i < 0 || i > j {
		t.Errorf("want a move down before the final newline, got %q", out)
	}
}

// cells is the line's counterpart to displayWidth, and the two have to agree.
//
// They are separate because the prompt may carry escape sequences and the line
// never can — the editor drops a control character rather than inserting it —
// so the line is counted without the check for them, on a path that runs at
// every keystroke. Two counts of one thing is two things to get wrong, and
// this is what says they have not.
func TestCountingCellsAgreesWithMeasuringAString(t *testing.T) {
	for _, s := range []string{"", "abc", "日本語", "héllo", "e\u0301", "a日b", "👍"} {
		if got, want := cells([]rune(s)), displayWidth(s); got != want {
			t.Errorf("cells(%q) = %d, displayWidth(%q) = %d", s, got, s, want)
		}
	}
}

// A line of wide characters is drawn exactly where a line of the same width in
// ASCII is drawn.
//
// The strongest form of the claim, and the one that needs no arithmetic in the
// test: five `日` and ten `x` fill the same ten cells, so every byte the editor
// sends to move the cursor has to be the same for both. Asserting a row and a
// column instead lets the test and the code agree on the same wrong answer.
func TestAWideLineIsDrawnWhereAnAsciiLineOfTheSameWidthIs(t *testing.T) {
	// Twelve columns and two of prompt, so ten cells of line run onto a
	// second row.
	draw := func(line string, pos int) string {
		var out strings.Builder
		e := &editor{out: &out, width: func() int { return 12 }}
		e.line, e.pos = []rune(line), pos
		e.redraw(drawPrompt("$ "))
		return movements(out.String())
	}
	for _, c := range []struct {
		name              string
		widePos, asciiPos int
	}{
		{"the cursor at the start", 0, 0},
		{"the cursor in the middle", 2, 4},
		{"the cursor at the edge of the first row", 5, 10},
	} {
		t.Run(c.name, func(t *testing.T) {
			wide := draw("日本語日本", c.widePos)
			plain := draw("1234567890", c.asciiPos)
			if !strings.Contains(plain, "\x1b[1A") && c.asciiPos < 10 {
				t.Fatal("the drawing did not move rows, so this test proves nothing")
			}
			if wide != plain {
				t.Errorf("the wide line was moved about with\n%q\nand the plain one with\n%q", wide, plain)
			}
		})
	}
}

// movements is the drawing with the printable text taken out, so that two
// lines of the same width can be compared by where they put the cursor.
func movements(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == esc || s[i] == '\r' || s[i] == '\n' {
			if s[i] != esc {
				b.WriteByte(s[i])
				i++
				continue
			}
			start := i
			i++
			if i < len(s) && s[i] == '[' {
				i++
				for i < len(s) && (s[i] < '@' || s[i] > '~') {
					i++
				}
			}
			if i < len(s) {
				i++
			}
			b.WriteString(s[start:i])
			continue
		}
		// A printable run counts as its width, so that the space that forces
		// a wrap is not mistaken for a character of the line.
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		b.WriteString(strings.Repeat(".", runeWidth(r)))
	}
	return b.String()
}
