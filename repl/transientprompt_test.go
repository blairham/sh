// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// What an accepted line leaves behind on the screen (#3787).
//
// Asserted against the screen model rather than against the bytes, for the
// reason the issue gives: a trim that emits the right sequences in the wrong
// order leaves the terminal wrong in a way a byte assertion cannot see. What
// is being claimed is "the upper row of the prompt is gone and the line is
// still there", and that is a statement about cells.
//
// The prompt is two rows, because a one-row prompt cannot fail the way this
// is most likely to: redraw returns to the row the cursor is on and rewrites
// from the prompt's *last* row down, so a trim written as an ordinary redraw
// leaves the upper row untouched and the screen ends up with the old frame's
// top and the new prompt under it. Only a two-row prompt can catch that.
func TestAnAcceptedLineCollapsesItsPrompt(t *testing.T) {
	const cols = 40

	// The rows of the screen after the line has been accepted, with trailing
	// blank rows dropped.
	run := func(t *testing.T, promptText string, transient TransientPrompt) []string {
		t.Helper()
		var out strings.Builder
		e := &editor{out: &out, width: func() int { return cols }, transient: transient}
		p := drawPrompt(promptText)
		e.write(p.lead + p.text)
		e.promptDrawn(p)
		e.line, e.pos = []rune("echo hi"), len("echo hi")
		e.write(string(e.line))

		sc := newScreen(cols)
		sc.feed(out.String())
		out.Reset()

		e.endLine(p, "")
		sc.feed(out.String())

		rows := strings.Split(sc.text(), "\n")
		for len(rows) > 0 && strings.TrimSpace(rows[len(rows)-1]) == "" {
			rows = rows[:len(rows)-1]
		}
		for i := range rows {
			rows[i] = strings.TrimRight(rows[i], " ")
		}
		return rows
	}

	bare := func() string { return "$ " }

	t.Run("a two-row prompt collapses to the bare one", func(t *testing.T) {
		rows := run(t, "TOP\r\n> ", bare)
		if len(rows) != 1 {
			t.Fatalf("screen is %d row(s), want 1:\n%q", len(rows), rows)
		}
		if rows[0] != "$ echo hi" {
			t.Errorf("row 0 = %q, want %q", rows[0], "$ echo hi")
		}
	})

	t.Run("a one-row prompt is replaced too", func(t *testing.T) {
		rows := run(t, "> ", bare)
		if len(rows) != 1 || rows[0] != "$ echo hi" {
			t.Errorf("rows = %q, want one row %q", rows, "$ echo hi")
		}
	})

	// CONTROL. Nothing wired is the prompt exactly as it was drawn, both rows
	// of it. Without this row the test above passes on an editor that erased
	// the screen and wrote nothing, which is not the feature.
	t.Run("no transient leaves both rows", func(t *testing.T) {
		rows := run(t, "TOP\r\n> ", nil)
		if len(rows) != 2 {
			t.Fatalf("screen is %d row(s), want 2 — the prompt should be untouched:\n%q", len(rows), rows)
		}
		if rows[0] != "TOP" || rows[1] != "> echo hi" {
			t.Errorf("rows = %q, want [TOP \"> echo hi\"]", rows)
		}
	})

	// CONTROL. An empty answer is "off", and it must be the same as nil
	// rather than a trim to nothing — a shell that turns the setting off must
	// not lose its prompt.
	t.Run("an empty answer is off, not a trim to nothing", func(t *testing.T) {
		rows := run(t, "TOP\r\n> ", func() string { return "" })
		if len(rows) != 2 || rows[0] != "TOP" || rows[1] != "> echo hi" {
			t.Errorf("rows = %q, want the untrimmed two-row prompt", rows)
		}
	})

	// CONTROL. The line itself is not the prompt and never goes away. A trim
	// that erased from too far up would take the line with it, and every row
	// above would still read as "collapsed".
	t.Run("the accepted line survives", func(t *testing.T) {
		rows := run(t, "TOP\r\n> ", bare)
		if len(rows) == 0 || !strings.Contains(rows[0], "echo hi") {
			t.Errorf("rows = %q, want the accepted line still on the screen", rows)
		}
	})
}

// Where the cursor is left once a wrapped line has been trimmed (#3787).
//
// Separate from the screen test above because it is the one property that
// test cannot see: it strips trailing blank rows, and getting this wrong
// produces exactly one extra blank row.
//
// The trim writes the whole line, so the cursor ends at the *end* of it,
// which for a wrapped line is not the row it was on while being edited. The
// step after the trim, toLastRow, moves down by the difference between the
// line's last row and the row it believes the cursor is on — so if the trim
// does not record where it actually left the cursor, that move is counted
// from a stale row and the newline lands one row too low.
//
// The editing position is put in the middle of the first row on purpose:
// with the cursor and the line's end on the same row, a stale row and a
// fresh one are the same number and nothing can fail.
func TestATrimmedWrappedLineLeavesTheCursorOnTheRowBelowIt(t *testing.T) {
	const cols = 40
	// 50 columns of line under a two-cell prompt: 52 cells, so two rows.
	line := strings.Repeat("x", 50)

	var out strings.Builder
	e := &editor{out: &out, width: func() int { return cols }, transient: func() string { return "$ " }}
	p := drawPrompt("TOP\r\n> ")
	e.write(p.lead + p.text)
	e.promptDrawn(p)
	e.line, e.pos = []rune(line), 5
	e.write(line)
	// Back to the editing position, which is on the prompt's own row — where
	// a person who pressed Home would be. The cursor is *moved* rather than
	// the row simply assigned: e.row is a record of where the cursor is, and
	// a test that sets one without the other is describing a screen the
	// editor never produces.
	e.write("\x1b[1A\r\x1b[7C")
	e.row = 0

	sc := newScreen(cols)
	sc.feed(out.String())
	out.Reset()

	e.endLine(p, "")
	sc.feed(out.String())

	// The collapsed line is rows 0 and 1, so the next thing written belongs
	// on row 2.
	row, col := sc.at()
	if row != 2 || col != 0 {
		t.Errorf("cursor left at row %d col %d, want row 2 col 0 — "+
			"the newline after a trimmed wrapped line is on the wrong row:\n%q",
			row, col, strings.Split(sc.text(), "\n"))
	}
}

// A terminal whose width is unknown keeps the prompt it drew (#3787).
//
// Every move the trim makes is counted in rows and columns, so without a
// width there is no arithmetic to do. Erasing anyway and writing the short
// prompt would leave the old rows on the screen under the new one — worse
// than not trimming, and worse in a way the person cannot undo. A missing
// feature is the right failure here.
func TestNoTerminalWidthLeavesThePromptAlone(t *testing.T) {
	var out strings.Builder
	e := &editor{out: &out, transient: func() string { return "$ " }}
	p := drawPrompt("TOP\r\n> ")
	e.line, e.pos = []rune("echo hi"), len("echo hi")

	e.endLine(p, "")

	if got := out.String(); strings.Contains(got, "$ ") || strings.Contains(got, "\x1b[J") {
		t.Errorf("trimmed with no width: %q", got)
	}
}

// A line that ends exactly at the right-hand edge (#3787).
//
// The terminal has filled the row but has not moved off it, so the cursor is
// still on the old row and every count from there is one out. The trim writes
// a space to force the wrap and a carriage return to undo the space, which is
// the same bargain redraw and toLastRow both make.
func TestATrimmedLineEndingAtTheEdgeLandsOnTheNextRow(t *testing.T) {
	const cols = 40
	// Two cells of prompt and 38 of line is exactly one full row.
	line := strings.Repeat("x", 38)

	var out strings.Builder
	e := &editor{out: &out, width: func() int { return cols }, transient: func() string { return "$ " }}
	p := drawPrompt("TOP\r\n> ")
	e.write(p.lead + p.text)
	e.promptDrawn(p)
	e.line, e.pos = []rune(line), len(line)
	e.write(line)

	sc := newScreen(cols)
	sc.feed(out.String())
	out.Reset()

	e.endLine(p, "")
	sc.feed(out.String())

	rows := strings.Split(sc.text(), "\n")
	if len(rows) == 0 || strings.TrimRight(rows[0], " ") != "$ "+line {
		t.Errorf("row 0 = %q, want the collapsed full row", rows[0])
	}
	if row, col := sc.at(); row != 2 || col != 0 {
		t.Errorf("cursor at row %d col %d, want row 2 col 0 — the full row is row 1 "+
			"and the newline belongs below it:\n%q", row, col, rows)
	}
}

// leadRows counts the rows a prompt's leading text occupies, which is what
// says how far above the line the trim has to reach.
func TestLeadRowsCountsTheRowsAboveTheLine(t *testing.T) {
	for _, tc := range []struct {
		name, lead string
		want       int
	}{
		{"a one-row prompt has none", "", 0},
		{"one newline is one row above", "TOP\r\n", 1},
		{"two newlines are two", "A\r\nB\r\n", 2},
		// An escape sequence puts nothing in a cell and ends no row, so it
		// must not be counted. This is the row that fails if the count is
		// ever rewritten as a width measurement.
		{"a title sequence is not a row", "\x1b]0;title\a", 0},
		{"a colored row is still one row", "\x1b[32mTOP\x1b[0m\r\n", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := leadRows(tc.lead); got != tc.want {
				t.Errorf("leadRows(%q) = %d, want %d", tc.lead, got, tc.want)
			}
		})
	}
}
