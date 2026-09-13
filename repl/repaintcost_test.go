// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// What a keystroke costs on the wire.
//
// zsh repaints what changed; this editor redrew the whole line, prompt and
// all, for every character typed — which is invisible on a local terminal and
// is exactly what is felt over ssh or inside a multiplexer. The bar for this
// dialect is that it is *faster* than the shell it imitates, so the cost is a
// measurement with a number on it rather than a thing to be careful about.
//
// Counted the way #2546 counted it: the bytes the editor writes while a line
// is typed, divided by the keystrokes that typed it. The prompt carries an
// escape sequence because a real one does, and that is most of what a full
// redraw re-sends.

// A highlighter that colors the first word, which is what makes a run exist
// at all. Deliberately not UnclosedQuote: that one styles nothing on a line
// with no open quote, so a line typed through it would measure an editor with
// no highlighting at work.
type colorFirstWord struct{}

func (colorFirstWord) Highlight(line string) []Highlight {
	end := strings.IndexByte(line, ' ')
	if end <= 0 {
		end = len(line)
	}
	if end == 0 {
		return nil
	}
	return []Highlight{{Start: 0, End: end, Style: "\x1b[93m"}}
}

// typedCost returns the bytes written while text is typed at prompt.
func typedCost(t *testing.T, prompt, text string) int {
	t.Helper()
	var out strings.Builder
	e := &editor{
		in:          typing(text + "\r"),
		out:         &out,
		highlighter: colorFirstWord{},
		width:       func() int { return 200 },
	}
	line, err := e.readLine(drawPrompt(prompt))
	if err != nil {
		t.Fatalf("readLine: %v", err)
	}
	if line != text {
		t.Fatalf("line = %q, want %q", line, text)
	}
	return out.Len()
}

const shortPrompt = "$ "

// A prompt the length of a real one. Two rows of color and a path is what a
// theme draws, and #2546 was measured against exactly that rather than
// against `$ `.
const longPrompt = "\x1b[32muser@host\x1b[0m:\x1b[34m~/Developer/github.com/blairham/sh\x1b[0m \x1b[35m(main)\x1b[0m$ "

// **The prompt must not be re-sent for every character typed.**
//
// This is the discriminating question, and a per-keystroke byte ceiling is
// not: a ceiling passes or fails on how long the test's own prompt happens to
// be, so it measures the fixture. Typing one line behind two prompts of very
// different lengths measures the editor. If the draw re-sends the prompt, the
// long one costs an extra prompt for every keystroke; if it sends only what
// changed, the two runs differ by about one prompt, once.
func TestTypingDoesNotReSendThePrompt(t *testing.T) {
	const text = "echo hi"
	keys := len([]rune(text))

	short := typedCost(t, shortPrompt, text)
	long := typedCost(t, longPrompt, text)
	extra := long - short
	t.Logf("short prompt %d bytes, long prompt %d bytes, difference %d over %d keystrokes",
		short, long, extra, keys)

	// One prompt's worth of difference is expected and correct — the prompt
	// is drawn once. Anything approaching one per keystroke is the whole line
	// being repainted.
	if ceiling := 2 * len(longPrompt); extra > ceiling {
		t.Errorf("a longer prompt cost %d more bytes for %d keystrokes (ceiling %d):\n"+
			"the prompt is being re-sent on every keystroke", extra, keys, ceiling)
	}
}

// **What a keystroke costs must not grow with the line it is typed into.**
//
// The prompt row above catches a draw that re-sends the prompt. This catches
// the other half — a draw that re-sends the *line* — and the two are separate
// failures: sending the whole line without the prompt is cheaper than before
// and still quadratic, so typing into a long line gets steadily more
// expensive. Typing twice as much must cost about twice as much, not four
// times.
func TestTypingCostGrowsWithTheTypingAndNotWithTheLine(t *testing.T) {
	short := typedCost(t, shortPrompt, strings.Repeat("x", 40))
	long := typedCost(t, shortPrompt, strings.Repeat("x", 80))
	t.Logf("40 characters cost %d bytes, 80 cost %d — ratio %.2f", short, long, float64(long)/float64(short))

	// Linear is 2.0 and quadratic is 4.0, but neither is reached exactly: the
	// prompt is drawn once either way, and that fixed cost pulls both ratios
	// down. Measured on this fixture, sending the difference gives 1.9 and
	// sending the whole line every time gives 2.7 — so the bar sits between
	// the two rather than at either name, and was placed by mutating the
	// split to zero and reading what that scored.
	if ratio := float64(long) / float64(short); ratio > 2.3 {
		t.Errorf("twice the typing cost %.2f times the bytes: the line is being re-sent, not the keystroke", ratio)
	}
}

// The control, and the reason the cost rows above mean anything.
//
// A cost that only ever falls would also be achieved by an editor that had
// stopped drawing, so this asserts the screen: the line is there, in the right
// place, and it was colored. Asked of a terminal rather than of the bytes —
// the cheap draw reaches the same cells by a different road, and the road is
// not the claim. See screen_test.go.
func TestTheCheapDrawStillPutsTheLineAndItsColorOnTheScreen(t *testing.T) {
	var out strings.Builder
	e := &editor{
		in:          typing("echo hi\r"),
		out:         &out,
		highlighter: colorFirstWord{},
		width:       func() int { return 80 },
	}
	if _, err := e.readLine(drawPrompt("$ ")); err != nil {
		t.Fatalf("readLine: %v", err)
	}
	sc := newScreen(80).feed(out.String())
	if got := sc.text(); got != "$ echo hi" {
		t.Errorf("the screen reads %q, want %q", got, "$ echo hi")
	}
	if !sc.painted() {
		t.Error("nothing was colored: the highlighter never reached the screen")
	}
}

// The difference and the whole line must put the same thing on the screen.
//
// This is the invariant the fast path lives or dies by, and it is asked of
// editing that moves backwards as well as forwards: an insert in the middle, a
// delete, a kill to the end, a word that changes color behind the cursor, and
// a line long enough to wrap. Each is typed twice — once normally, and once
// through an editor whose belief is cleared before every draw, so the second
// run is the old whole-line redraw. The screens have to match.
func TestTheDifferenceDrawsWhatTheWholeLineDraws(t *testing.T) {
	for _, c := range []struct{ name, keys string }{
		{"typing", "echo hi"},
		{"insert in the middle", "echo hi\x01\x06\x06X"},
		{"backspace", "echo hix\x7f"},
		{"delete back a word", "echo one two\x17"},
		{"kill to the end", "echo one two\x01\x06\x06\x06\x06\x0b"},
		{"a color that changes behind the cursor", "echo\x01X"},
		{"a line that wraps", "echo 1234567890123456789012345678901234567890"},
		// The prompt is two cells and the terminal is 24, so twenty-two
		// characters end the line exactly at the right-hand edge — the cell a
		// terminal defers the wrap in. One either side of it, because a row
		// that only tries the edge cannot tell a wrong answer from an
		// arithmetic that is out by one everywhere.
		{"ends one short of the edge", "echo 123456789012345678901"},
		{"ends exactly at the edge", "echo 1234567890123456789012"},
		{"ends one past the edge", "echo 12345678901234567890123"},
		{"edge, then one more", "echo 1234567890123456789012X"},
		{"edge, then backspace", "echo 1234567890123456789012\x7f"},
		// The cursor moving *down* between two draws. Every whole-line draw
		// ends by coming up from the end of the line, so nothing else in this
		// table ever asks a draw to go the other way: ^A puts the cursor on
		// the first row of a wrapped line and ^E sends it back to the last.
		{"wrapped, home then end", "echo 1234567890123456789012345678901234567890\x01\x05"},
		{"wrapped, home then forward", "echo 1234567890123456789012345678901234567890\x01\x06\x06"},
		// Three rows, and a cursor walked back across a row boundary so that
		// the move up lands on a row that is **not** the first. A move to the
		// first row is the one shape that cannot catch an up-move that
		// overshoots: the terminal has nowhere above row 0 to go, so a count
		// one too large arrives in the same place as a correct one.
		{"three rows, walked back across a boundary", "echo 123456789012345678901234567890123456789012345678901234567890" + "\x02\x02\x02\x02\x02\x02\x02\x02\x02\x02\x02\x02\x02\x02\x02\x02\x02\x02\x02\x02"},
		{"three rows, home", "echo 123456789012345678901234567890123456789012345678901234567890\x01"},
		{"wrapped, then cut back", "echo 1234567890123456789012345678901234567890\x17\x17"},
		{"emptied", "abc\x7f\x7f\x7f"},
	} {
		t.Run(c.name, func(t *testing.T) {
			const cols = 24
			quick := newScreen(cols).feed(typedThrough(t, cols, c.keys, false))
			whole := newScreen(cols).feed(typedThrough(t, cols, c.keys, true))
			if quick.text() != whole.text() {
				t.Errorf("the screens differ:\n difference: %q\n whole line: %q",
					quick.text(), whole.text())
			}
			if quick.colors() != whole.colors() {
				t.Errorf("the color landed differently:\n difference: %q\n whole line: %q",
					quick.colors(), whole.colors())
			}
			qr, qc := quick.cursor()
			wr, wc := whole.cursor()
			if qr != wr || qc != wc {
				t.Errorf("the cursor differs: difference left it at (%d,%d), whole line at (%d,%d)",
					qr, qc, wr, wc)
			}
		})
	}
}

// typedThrough types keys at a cols-wide terminal. With alwaysWhole set, the
// editor's belief about the screen is cleared before every keystroke, which is
// the whole-line redraw this change replaced.
func typedThrough(t *testing.T, cols int, keys string, alwaysWhole bool) string {
	t.Helper()
	var out strings.Builder
	e := &editor{
		in:          typing(keys),
		out:         &out,
		highlighter: colorFirstWord{},
		width:       func() int { return cols },
	}
	if alwaysWhole {
		e.neverRemember = true
	}
	// No Enter. Accepting the line moves the cursor to a fresh row, which
	// normalises away exactly the disagreement this is looking for — a draw
	// that left the cursor a row out is indistinguishable once both have gone
	// down to the next one. Running the input out instead ends readLine where
	// the last keystroke left it.
	if _, err := e.readLine(drawPrompt("$ ")); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("readLine: %v", err)
	}
	return out.String()
}

// A color that changes without the text changing.
//
// The difference is over characters *and* their style, and the style half had
// nothing asking about it: every other case here changes the text, so a diff
// that compared only characters would still have found the right split. This
// highlighter recolors the whole line on a character typed at the end, so the
// text before the cursor is identical and the color under it is not — and a
// draw that matched on text alone would leave the old color on the screen.
type colorByLength struct{}

func (colorByLength) Highlight(line string) []Highlight {
	if line == "" {
		return nil
	}
	style := "\x1b[31m"
	if len(line)%2 == 0 {
		style = "\x1b[32m"
	}
	return []Highlight{{Start: 0, End: len(line), Style: style}}
}

func TestAColorThatChangesBehindTheCursorIsRedrawn(t *testing.T) {
	const cols = 24
	run := func(whole bool) *screen {
		var out strings.Builder
		e := &editor{
			in:          typing("abcdef"),
			out:         &out,
			highlighter: colorByLength{},
			width:       func() int { return cols },
		}
		e.neverRemember = whole
		if _, err := e.readLine(drawPrompt("$ ")); err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("readLine: %v", err)
		}
		return newScreen(cols).feed(out.String())
	}
	quick, full := run(false), run(true)
	if quick.text() != full.text() {
		t.Errorf("text differs:\n difference: %q\n whole line: %q", quick.text(), full.text())
	}
	if quick.colors() != full.colors() {
		t.Errorf("the color was left behind:\n difference: %q\n whole line: %q",
			quick.colors(), full.colors())
	}
}

// A highlighter whose offsets fall inside a character.
//
// Highlight works in bytes and the draw works in characters, so a run whose
// end lands in the middle of a multi-byte character would put every column
// after it out by one — and a line drawn at the wrong column is worse than a
// line drawn twice. The draw checks that the runs account for exactly the
// characters in the line and declines when they do not; this is the row that
// makes the check mean something.
type splitsARune struct{}

func (splitsARune) Highlight(line string) []Highlight {
	if len(line) < 2 {
		return nil
	}
	// One byte in, which for the line below is inside the first character.
	return []Highlight{{Start: 0, End: 1, Style: "\x1b[31m"}}
}

func TestARunSplittingACharacterFallsBackToTheWholeLine(t *testing.T) {
	const cols = 24
	run := func(whole bool) *screen {
		var out strings.Builder
		e := &editor{
			in:          typing("日本語"),
			out:         &out,
			highlighter: splitsARune{},
			width:       func() int { return cols },
		}
		e.neverRemember = whole
		if _, err := e.readLine(drawPrompt("$ ")); err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("readLine: %v", err)
		}
		return newScreen(cols).feed(out.String())
	}
	// What such a highlighter *should* produce is a question for whoever
	// writes one — a style sequence inside a character has always drawn
	// rubbish here, and that is not this change's to fix. What is this
	// change's is that the cheap draw must not make it **worse**: it declines
	// the line rather than counting characters it cannot count, so the screen
	// is the one the whole-line draw produces, whatever that is.
	quick, whole := run(false), run(true)
	if quick.text() != whole.text() {
		t.Errorf("the difference diverged from the whole-line draw:\n difference: %q\n whole line: %q",
			quick.text(), whole.text())
	}
}

// styled() and the runs are two spellings of one drawing, and the difference
// path trusts that they agree: it diffs the runs and the whole-line path
// writes the string. A highlighter the guard admits must give the same bytes
// either way.
func TestTheRunsAndTheStringAgree(t *testing.T) {
	for _, c := range []struct {
		name string
		line string
		runs []Highlight
	}{
		{"no runs", "echo one", nil},
		{"one run in the middle", "echo one two", []Highlight{{Start: 5, End: 8, Style: "\x1b[31m"}}},
		{"a run at the start", "echo one", []Highlight{{Start: 0, End: 4, Style: "\x1b[31m"}}},
		{"a run to the end", "echo one", []Highlight{{Start: 5, End: 8, Style: "\x1b[32m"}}},
		{"two runs", "echo one two", []Highlight{{Start: 0, End: 4, Style: "\x1b[31m"}, {Start: 9, End: 12, Style: "\x1b[32m"}}},
		{"over wide characters", "echo 日本", []Highlight{{Start: 5, End: 11, Style: "\x1b[31m"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := &editor{line: []rune(c.line), highlighter: fixedRuns(c.runs)}
			var b strings.Builder
			writeRunsFrom(&b, e.styledRuns(), 0)
			if got, want := b.String(), e.styled(); got != want {
				t.Errorf("the runs drew %q, the string drew %q", got, want)
			}
		})
	}
}

// fixedRuns is a highlighter that answers the same runs whatever it is given.
type fixedRuns []Highlight

func (f fixedRuns) Highlight(string) []Highlight { return f }

// A prompt that changes under the draw.
//
// Reverse search replaces the prompt with `(reverse-i-search)` and its query,
// so the prompt is a different width on every keystroke. The difference is
// computed in columns measured from the prompt's width, so a belief formed
// against one prompt says nothing about a screen drawn under another — the
// draw has to notice and write the line whole.
//
// Nothing else in this package changes the prompt mid-line, which is why this
// is its own row: the guard was there, and dropping it failed nothing.
func TestASearchChangingThePromptIsDrawnWhole(t *testing.T) {
	const cols = 40
	run := func(whole bool) *screen {
		var out strings.Builder
		e := &editor{
			in:          typing("\x12one"),
			out:         &out,
			history:     []string{"echo one", "echo two", "echo one more"},
			highlighter: colorFirstWord{},
			width:       func() int { return cols },
		}
		e.neverRemember = whole
		if _, err := e.readLine(drawPrompt("$ ")); err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("readLine: %v", err)
		}
		return newScreen(cols).feed(out.String())
	}
	quick, whole := run(false), run(true)
	if quick.text() != whole.text() {
		t.Errorf("the search drew differently:\n difference: %q\n whole line: %q",
			quick.text(), whole.text())
	}
	if quick.colors() != whole.colors() {
		t.Errorf("the search colored differently:\n difference: %q\n whole line: %q",
			quick.colors(), whole.colors())
	}
	qr, qc := quick.cursor()
	wr, wc := whole.cursor()
	if qr != wr || qc != wc {
		t.Errorf("the cursor differs: difference (%d,%d), whole line (%d,%d)", qr, qc, wr, wc)
	}
}

// A prompt that changes without changing width.
//
// The difference never writes the prompt — it starts from the first character
// that moved, which is in the line — so a prompt that changed under it stays
// on the screen as it was. Two checks guard that, and only one of them is
// exercised by a real session: reverse search changes the prompt's *width*,
// so the width check alone would catch it and the text check reads as
// redundant.
//
// It is not redundant. A prompt that reports something of a fixed width — an
// exit status, a clock, a branch — changes its text and keeps its cells, and
// then only the text check is between the person and a prompt showing the
// previous command's status forever. Driven directly, because nothing in this
// package changes a prompt that way yet and a guard nothing exercises is a
// guard nobody knows is broken.
func TestAPromptThatChangesWithoutChangingWidthIsDrawnAgain(t *testing.T) {
	const cols = 40
	var out strings.Builder
	e := &editor{out: &out, width: func() int { return cols }}
	e.line, e.pos = []rune("echo hi"), 7

	e.redraw(drawPrompt("[0]$ "))
	// Same width, different text — the status changed.
	e.redraw(drawPrompt("[1]$ "))

	got := newScreen(cols).feed(out.String()).text()
	if want := "[1]$ echo hi"; got != want {
		t.Errorf("the screen still shows the old prompt:\n got %q\nwant %q", got, want)
	}
}

// Something else printed, so the belief is gone.
//
// The difference is only safe while the editor knows what is on the screen,
// and plenty of things write to the terminal without going through a draw: a
// completion listing, a job notice, the ground under a fresh prompt. Each
// leaves the screen somewhere the belief does not describe, and a draw that
// trusted it would send a difference against a picture that is no longer
// there.
//
// Listing completions is the one a person hits every day, so it is the one
// with a row. write() is what clears the belief, which is why this passes for
// anything that prints rather than only for this.
func TestAListingClearsWhatTheEditorBelievesIsOnTheScreen(t *testing.T) {
	const cols = 40
	comp := CompleterFunc(func(Completion) []string {
		return []string{"apple", "apricot"}
	})
	run := func(whole bool) *screen {
		var out strings.Builder
		e := &editor{
			in:          typing("echo ap\t\tX"),
			out:         &out,
			comp:        comp,
			highlighter: colorFirstWord{},
			width:       func() int { return cols },
		}
		e.neverRemember = whole
		if _, err := e.readLine(drawPrompt("$ ")); err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("readLine: %v", err)
		}
		return newScreen(cols).feed(out.String())
	}
	quick, whole := run(false), run(true)
	if quick.text() != whole.text() {
		t.Errorf("the screen after a listing differs:\n difference: %q\n whole line: %q",
			quick.text(), whole.text())
	}
	qr, qc := quick.cursor()
	wr, wc := whole.cursor()
	if qr != wr || qc != wc {
		t.Errorf("the cursor differs: difference (%d,%d), whole line (%d,%d)", qr, qc, wr, wc)
	}
}
