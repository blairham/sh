// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// A pasted line is drawn once, not once per character.
//
// A paste arrives as a single write, and the editor draws the *whole* line every
// time it draws — which is deliberate, because a delta breaks the first time a
// character is wider than one cell or the line wraps (see redraw). Drawing it
// once per byte therefore costs the line squared: measured through a pty before
// this was fixed, a 405-byte paste put **88,695** bytes on the terminal against
// bash 5.3.15's 410, and a 2000-byte one was heading for megabytes (#1742).
//
// The fix is to wait rather than to draw less: input already in hand will be
// read before anybody could look at the screen, so the draw is held until the
// input runs out. What reaches the terminal is then one drawing of the line, and
// the screen is byte-for-byte what it always was.
//
// Bytes written is the assertion, not the sequences. That is the whole subject —
// the screen was correct throughout, which is why this was invisible to every
// other test in this package, and to `make smoke` at 29 of 29.
func TestAPastedLineIsDrawnOnce(t *testing.T) {
	const n = 400
	line := strings.Repeat("x", n)

	var out strings.Builder
	// strings.NewReader on purpose: one Read answers the whole line, which is
	// what a paste is. The typing case is typing(), and it is what every other
	// test in this package uses.
	e := &editor{in: strings.NewReader(line + "\r"), out: &out}
	got, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("readLine: %v", err)
	}
	if got != line {
		t.Fatalf("line came back as %d characters, want %d", len(got), n)
	}
	// One drawing of the line, plus the prompt and the cursor arithmetic. The
	// bound is generous — what it excludes is the old behavior, which was two
	// orders of magnitude over it, not a few bytes of escape sequence either
	// way.
	if written := out.Len(); written > 3*n {
		t.Errorf("a %d-byte paste wrote %d bytes to the terminal, want no more than %d — the line is being drawn more than once",
			n, written, 3*n)
	}
	// And it really was drawn: a bound alone would be satisfied by a shell that
	// drew nothing at all.
	if !strings.Contains(out.String(), line) {
		t.Errorf("the pasted line never reached the terminal")
	}
}

// The same line typed a character at a time is still drawn a character at a
// time, which is the half the coalescing must not take away: the highlighter
// colors the line as it is written, and a person watching sees each keystroke.
//
// So this asserts the opposite bound to the test above, on the same input. The
// pair is what says the editor is choosing by *when the input arrived* rather
// than doing one thing always.
func TestATypedLineIsDrawnAsItIsTyped(t *testing.T) {
	const n = 40
	line := strings.Repeat("x", n)

	var out strings.Builder
	e := &editor{in: typing(line + "\r"), out: &out}
	if _, err := e.readLine(drawPrompt("$ ")); err != nil {
		t.Fatalf("readLine: %v", err)
	}
	// Each keystroke redraws the line, so the total grows with the square. The
	// bound is a floor rather than a ceiling: anything near the paste figure
	// would mean the draws had been coalesced away and nobody would see the
	// line grow.
	if written := out.Len(); written < 4*n {
		t.Errorf("typing %d characters wrote only %d bytes, want at least %d — the draws have been coalesced away",
			n, written, 4*n)
	}
}

// A line pasted with its newline in the same write is still drawn before it
// runs.
//
// The case the obvious implementation gets wrong: the input never runs out — the
// `\r` is in the same write as the text — so nothing would ever trigger the held
// draw and the command would run having never appeared. endLine flushes it,
// which it has to anyway: the row it counts back from is only accurate once the
// line has been drawn.
func TestAPasteEndingInItsNewlineIsStillDrawn(t *testing.T) {
	var out strings.Builder
	e := &editor{in: strings.NewReader("echo pasted\r"), out: &out}
	if _, err := e.readLine(drawPrompt("$ ")); err != nil {
		t.Fatalf("readLine: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "echo pasted") {
		t.Errorf("the pasted line was never drawn: %q", got)
	}
}

// A question the editor asks mid-line is answered from the buffer, not from the
// descriptor underneath it.
//
// The listing query is the case that caught this: `confirmList` read `e.in`
// directly, so with the whole line in one write it saw end-of-input while the
// answer sat in the buffer — it declined, and the `n` was then typed into the
// line, which came back as `an`.
//
// It has to be a paste. The same case typed a byte at a time never fills the
// buffer, so a reader that goes straight to the descriptor works and the bug is
// invisible — which is exactly what happened when the existing listing test was
// moved onto typing(): it had been catching this, and stopped. The two shapes
// test different things and both are wanted.
func TestAPastedAnswerToTheListingQueryIsConsumed(t *testing.T) {
	for _, c := range []struct {
		name, answer string
		listed       bool
	}{
		{"declined", "n", false},
		{"accepted", "y", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out strings.Builder
			e := &editor{
				// One Read answers all of it: the line, both Tabs, the answer
				// and the newline.
				in:              strings.NewReader("a\t\t" + c.answer + "\r"),
				out:             &out,
				comp:            fakeCompleter{cmds: shortNames(120)},
				listQuery:       "ask %[1]d %[2]d",
				listQueryStrict: true,
			}
			line, err := e.readLine(drawPrompt("P> "))
			if err != nil {
				t.Fatal(err)
			}
			if line != "a" {
				t.Fatalf("line = %q, want %q — the answer was typed into the line instead of being read as an answer", line, "a")
			}
			if listed := strings.Contains(out.String(), "a119"); listed != c.listed {
				t.Errorf("listed = %v, want %v — the answer did not decide the listing", listed, c.listed)
			}
		})
	}
}
