// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// The redraw writes the difference and not the line, so what has to be
// asserted is the screen — see screenmodel_test.go for why the old
// substring-shaped assertions could not survive the change.
//
// The check below is the strong one: after every edit, what a terminal would
// be showing is exactly what it would show if the prompt and the line had been
// written to a blank screen, and the cursor is where the arithmetic says the
// cursor goes. It is run over the shapes an incremental redraw can get wrong —
// growing, shrinking, insertion away from the end, wrapping, unwrapping, wide
// characters, and a line replaced wholesale the way history replaces one.

// an edit is the line and the cursor as some keystroke left them.
type edit struct {
	line string
	pos  int
}

// typingOut is the line being typed one character at a time, cursor at the end.
func typingOut(line string) []edit {
	var steps []edit
	rs := []rune(line)
	for i := 0; i <= len(rs); i++ {
		steps = append(steps, edit{string(rs[:i]), i})
	}
	return steps
}

// tokenColors is a highlighter of the shape the ones in the wild have: a run
// per word, each ended by this package's reset. It is what makes a keystroke
// change bytes in the middle of the drawn line rather than only at the end.
type tokenColors struct{}

func (tokenColors) Highlight(line string) []Highlight {
	var runs []Highlight
	at := 0
	for at < len(line) {
		for at < len(line) && line[at] == ' ' {
			at++
		}
		end := at
		for end < len(line) && line[end] != ' ' {
			end++
		}
		if end > at {
			style := "\x1b[38;5;13m"
			if len(runs) == 0 {
				style = "\x1b[38;5;93m"
			}
			runs = append(runs, Highlight{Start: at, End: end, Style: style})
		}
		at = end
	}
	return runs
}

// checkEdits drives one editor through the edits and checks the screen after
// each of them.
func checkEdits(t *testing.T, cols int, prompt string, hl Highlighter, steps []edit) {
	t.Helper()
	var out strings.Builder
	e := &editor{out: &out, highlighter: hl, width: func() int { return cols }}
	p := drawPrompt(prompt)
	live := newScreen(cols)
	for i, step := range steps {
		out.Reset()
		e.line, e.pos = []rune(step.line), step.pos
		e.redraw(p)
		live.feed(out.String())

		want := newScreen(cols)
		want.feed(p.text + step.line)
		if got := live.text(); got != want.text() {
			t.Fatalf("step %d (%q at %d): screen is\n%q\nwant\n%q\n(last write %q)",
				i, step.line, step.pos, got, want.text(), out.String())
		}
		wantRow, wantCol, _, _ := place(p.cells, e.line, e.pos, cols)
		wantRow, wantCol = pastEdge(wantRow, wantCol, cols)
		if row, col := live.at(); row != wantRow || col != wantCol {
			t.Fatalf("step %d (%q at %d): cursor at row %d column %d, want row %d column %d (last write %q)",
				i, step.line, step.pos, row, col, wantRow, wantCol, out.String())
		}
	}
}

func TestTheScreenIsRightAfterEveryKindOfEdit(t *testing.T) {
	const long = "one two three four five six seven eight nine ten eleven twelve"
	for _, tc := range []struct {
		name   string
		cols   int
		prompt string
		steps  []edit
	}{
		{"typed to the end", 80, "$ ", typingOut("echo hi")},
		{"typed past the edge", 20, "$ ", typingOut("0123456789012345678901234567")},
		{"typed past two edges", 12, "$ ", typingOut(long[:40])},
		{"a themed prompt", 40, "\x1b[48;5;238m dir \x1b[0m\x1b[38;5;76m > \x1b[0m", typingOut("echo hi")},
		{"inserted in the middle", 40, "$ ", []edit{
			{"echo hi", 7}, {"echo hi", 4}, {"echo Xhi", 6}, {"echo XYhi", 7}, {"echo XYhi", 0},
		}},
		{"deleted from the end", 40, "$ ", []edit{
			{"echo hi", 7}, {"echo h", 6}, {"echo ", 5}, {"echo", 4}, {"", 0},
		}},
		{"deleted from the middle", 40, "$ ", []edit{
			{"echo one two", 12}, {"echo one two", 8}, {"echo on two", 7}, {"echo o two", 6},
		}},
		{"unwrapped back onto one row", 12, "$ ", []edit{
			{long[:30], 30}, {long[:20], 20}, {long[:5], 5}, {"", 0},
		}},
		{"replaced wholesale", 20, "$ ", []edit{
			{"echo hello there", 16}, {"ls", 2}, {"echo hello there again and again", 32}, {"x", 1},
		}},
		{"ending exactly at the edge", 10, "#", []edit{
			{"12345678", 8}, {"123456789", 9}, {"1234567890", 10}, {"123456789", 9}, {"12345678", 8},
		}},
		{"wide characters", 10, "$ ", typingOut("日本語日本語")},
		{"wide characters deleted", 10, "$ ", []edit{
			{"日本語日本語", 6}, {"日本語日本", 5}, {"日本語", 3}, {"日", 1}, {"", 0},
		}},
		{"the cursor alone moves", 20, "$ ", []edit{
			{"echo hi", 7}, {"echo hi", 6}, {"echo hi", 0}, {"echo hi", 3}, {"echo hi", 7}, {"echo hi", 7},
		}},
		{"the cursor alone moves across rows", 12, "$ ", []edit{
			{long[:30], 30}, {long[:30], 0}, {long[:30], 15}, {long[:30], 30},
		}},
	} {
		for _, hl := range []struct {
			name string
			h    Highlighter
		}{
			{"plain", nil},
			{"colored", tokenColors{}},
			{"unclosed quotes", UnclosedQuote{Style: "\x1b[31m"}},
		} {
			t.Run(tc.name+"/"+hl.name, func(t *testing.T) {
				checkEdits(t, tc.cols, tc.prompt, hl.h, tc.steps)
			})
		}
	}
}

// A quotation opened part-way through a line recolors text that was already
// drawn, which is the case an incremental redraw has to notice in the middle
// of the line rather than at the end of it.
func TestReopeningAQuoteRecolorsWhatWasAlreadyDrawn(t *testing.T) {
	checkEdits(t, 40, "$ ", UnclosedQuote{Style: "\x1b[31m"}, []edit{
		{`echo one`, 8},
		{`echo "one`, 9},
		{`echo "one"`, 10},
		{`echo "one" two`, 14},
		{`echo "one" "two`, 15},
		{`echo "one" two`, 14},
	})
}
