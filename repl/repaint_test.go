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

// A redraw that cannot account for the screen puts the whole line back, and
// the prompt with it. These are the ways the screen moves without this editor
// having written anything.
//
// The whole-line draw is recognized by the prompt appearing again: it is the
// one thing an incremental redraw never writes, which is most of why it is
// cheaper.
func TestAScreenThisCannotAccountForIsDrawnWhole(t *testing.T) {
	const prompt = "PROMPT> "
	t.Run("the terminal was resized", func(t *testing.T) {
		var out strings.Builder
		cols := 40
		e := &editor{out: &out, width: func() int { return cols }}
		p := drawPrompt(prompt)
		e.line, e.pos = []rune("echo hi"), 7
		e.redraw(p)

		// A key typed at the same width does not mention the prompt.
		out.Reset()
		e.line, e.pos = []rune("echo hit"), 8
		e.redraw(p)
		if strings.Contains(out.String(), prompt) {
			t.Errorf("drew the prompt again for a keystroke: %q", out.String())
		}

		// One typed after a resize does: the terminal reflowed everything
		// that was on it, and where the line now sits is not something the
		// last draw can be reasoned from.
		out.Reset()
		cols = 20
		e.line, e.pos = []rune("echo hits"), 9
		e.redraw(p)
		if !strings.Contains(out.String(), prompt) {
			t.Errorf("did not draw the whole line after a resize: %q", out.String())
		}
	})

	t.Run("the prompt changed", func(t *testing.T) {
		// Same width, different text — a prompt that recolored itself. The
		// width matching is not enough: the old text is still on the row.
		var out strings.Builder
		e := &editor{out: &out, width: func() int { return 40 }}
		e.line, e.pos = []rune("echo hi"), 7
		e.redraw(drawPrompt("\x1b[32mPROMPT> \x1b[0m"))
		out.Reset()
		e.redraw(drawPrompt("\x1b[31mPROMPT> \x1b[0m"))
		if !strings.Contains(out.String(), "PROMPT> ") {
			t.Errorf("did not draw the new prompt: %q", out.String())
		}
	})

	t.Run("the prompt is wider than the terminal", func(t *testing.T) {
		// The rest of this package counts the line's rows from the row the
		// prompt's last row begins on and takes that to be the row the cursor
		// is on, which a prompt that wraps on its own makes untrue.
		var out strings.Builder
		e := &editor{out: &out, width: func() int { return 10 }}
		wide := drawPrompt("a-very-long-prompt> ")
		e.write(wide.lead + wide.text)
		e.promptDrawn(wide)
		out.Reset()
		e.line, e.pos = []rune("x"), 1
		e.redraw(wide)
		if !strings.Contains(out.String(), wide.text) {
			t.Errorf("did not draw the whole line under a wrapping prompt: %q", out.String())
		}
	})
}

// The cost of a keystroke does not depend on the prompt.
//
// This is the property the change is for, and it is the one zsh has: a prompt
// is drawn when a line begins, and a key typed into the line has no reason to
// touch it. A theme of the kind people run is several hundred bytes of escape
// sequences, so a redraw that re-emitted it would make one keystroke cost more
// than the whole line does.
//
// Driven through readLine rather than by calling redraw, because the prompt is
// written there and the first key of a line is the one that would pay for it.
//
// A bound rather than a number. What a keystroke costs depends on how many
// runs the highlighter returned and how long their escape sequences are, and
// pinning the total would make this fail for a change to either that is
// nobody's regression.
func TestAKeystrokeCostsTheSameUnderAThemedPrompt(t *testing.T) {
	const themed = "\x1b[38;5;31m\x1b[48;5;238m \x1b[38;5;250m~/src/sh \x1b[0m" +
		"\x1b[38;5;238m\x1b[48;5;236m \x1b[38;5;114mmain \x1b[0m\x1b[38;5;236m\x1b[0m " +
		"\x1b[38;5;76m❯\x1b[0m "
	const keys = "echo hi"
	cost := func(prompt string) (drawn int, sessions string) {
		var out strings.Builder
		e := &editor{
			in: typing(keys + "\r"), out: &out,
			highlighter: tokenColors{},
			width:       func() int { return 80 },
		}
		p := drawPrompt(prompt)
		if _, err := e.readLine(p); err != nil {
			t.Fatalf("readLine: %v", err)
		}
		return out.Len() - len(p.lead) - len(p.text), out.String()
	}
	plain, _ := cost("$ ")
	fancy, session := cost(themed)
	if plain != fancy {
		t.Errorf("typing %q cost %d bytes under a plain prompt and %d under a themed one; "+
			"the prompt is being redrawn", keys, plain, fancy)
	}
	if n := strings.Count(session, "❯"); n != 1 {
		t.Errorf("the themed prompt was drawn %d times for one line", n)
	}
	// And the cost per keystroke is of the order of the change, not of the
	// line: the bound is generous, and a whole-line redraw of this line under
	// this prompt is several hundred bytes a key.
	if perKey := plain / len(keys); perKey > 40 {
		t.Errorf("a keystroke cost %d bytes; the line is being redrawn whole", perKey)
	}
}

// A redraw that changes nothing says nothing.
//
// Widgets ask for one whenever they have run, whether or not they touched the
// line — see runShellWidget, where the redraw is unconditional because an
// opaque action cannot be asked what it did. With a highlighter in front of
// `self-insert` that is every keystroke, so the do-nothing case is on the hot
// path rather than beside it.
func TestARedrawThatChangesNothingWritesNothing(t *testing.T) {
	var out strings.Builder
	e := &editor{out: &out, highlighter: tokenColors{}, width: func() int { return 80 }}
	p := drawPrompt("$ ")
	e.line, e.pos = []rune("echo hi"), 7
	e.redraw(p)
	out.Reset()
	e.redraw(p)
	e.redraw(p)
	if out.Len() != 0 {
		t.Errorf("redrawing an unchanged line wrote %q", out.String())
	}
}

// Getting from one cell to another, in the fewest bytes that say it.
//
// Exact sequences here, where everything else in this file asserts the screen:
// this function's whole job is the encoding, so the encoding is the behavior.
// Each one is checked against the terminal model as well, because the shortest
// way of saying something is worth nothing if it says the wrong thing.
func TestTheCursorIsMovedInTheFewestBytes(t *testing.T) {
	const cols = 40
	for _, tc := range []struct {
		name                           string
		fromRow, fromCol, toRow, toCol int
		want                           string
	}{
		{"nowhere", 0, 7, 0, 7, ""},
		{"one left is a backspace", 0, 7, 0, 6, "\b"},
		{"three left is three of them", 0, 7, 0, 4, "\b\b\b"},
		{"four left is the sequence", 0, 7, 0, 3, "\x1b[4D"},
		{"back to the start of the line", 0, 30, 0, 0, "\r"},
		{"back to a column near the start", 0, 30, 0, 2, "\r\x1b[2C"},
		{"right", 0, 4, 0, 9, "\x1b[5C"},
		{"up a row, same column", 2, 6, 1, 6, "\x1b[1A"},
		{"down a row, same column", 1, 6, 2, 6, "\x1b[1B"},
		{"up two rows and a little left", 3, 9, 1, 7, "\x1b[2A\b\b"},
		{"up two rows and a long way left", 3, 30, 1, 14, "\x1b[2A\x1b[16D"},
		{"down a row to the start", 1, 9, 2, 0, "\x1b[1B\r"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b strings.Builder
			moveCursor(&b, tc.fromRow, tc.fromCol, tc.toRow, tc.toCol)
			if got := b.String(); got != tc.want {
				t.Errorf("moved with %q, want %q", got, tc.want)
			}
			// And it lands where it was asked to. The filler puts the cursor
			// at the starting cell without any of the sequences under test.
			s := newScreen(cols)
			s.grow(tc.fromRow + 1)
			s.row, s.col = tc.fromRow, tc.fromCol
			s.feed(b.String())
			if row, col := s.at(); row != tc.toRow || col != tc.toCol {
				t.Errorf("landed at row %d column %d, want row %d column %d", row, col, tc.toRow, tc.toCol)
			}
		})
	}
}
