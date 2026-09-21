// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "strings"

// Drawing the prompt again, in place, while a line is being typed.
//
// This is the second of the three things an asynchronous segment needs, and
// it is the one this editor genuinely did not have. The prompt is rendered
// once per line and handed to readLine, which writes the leading rows exactly
// once — "the leading rows first and once, every redraw after this rewrites
// only the last row" — because a keystroke can only change the row the line
// is on, and re-emitting the rows above it on every keystroke is what left a
// ladder of prompts behind an Up arrow (#2467).
//
// A segment that arrives late is the case those rows are not left alone in,
// and it is also the case that will be most common: a right-aligned or
// upper-row repository segment is what people write. So this goes up past
// them, exactly as trimPrompt does for the transient prompt, and for the same
// reason — the two are the same arithmetic in the same direction.
//
// # What it must not do
//
// Re-run the hooks. `precmd` and its neighbors fire once per prompt line,
// and a prompt redrawn because a segment arrived must not fire a person's
// hooks a second time — so the re-render is the *prompt half* of
// beforeReading and nothing else. See Shell.rerender.

// reprompt renders the prompt afresh and puts it on the screen under the line
// being typed, answering the prompt that is now there.
//
// It answers the old prompt unchanged wherever it cannot or need not draw:
// nothing to re-render, nothing different to draw, or no width to do the
// arithmetic with. Returning the prompt rather than storing it is the same
// interface trimPrompt has, and for the same reason — everything after this
// counts rows from the prompt's width, and it can only count correctly
// against the width the screen is actually wearing.
func (e *editor) reprompt(prompt drawnPrompt) drawnPrompt {
	if e.rerender == nil {
		return prompt
	}
	fresh, ok := e.rerender(e.cols())
	if !ok {
		// The theme is not drawing. Whatever is on the screen is the
		// person's own parameter and is not this file's to replace.
		return prompt
	}
	if fresh.lead == prompt.lead && fresh.text == prompt.text && fresh.right == prompt.right {
		// A publisher says a redraw *would* differ; it is not required to be
		// right. Writing nothing when it is wrong is what makes a publisher
		// that cannot tell a cheap thing to be rather than a flickering one.
		return prompt
	}
	cols := e.cols()
	if cols <= 0 {
		// None of the arithmetic below can be done without a width, and a
		// partial attempt leaves the screen worse than a prompt that is one
		// segment short. The same refusal trimPrompt makes.
		return prompt
	}

	var b strings.Builder
	b.WriteString("\r")
	if up := e.row + leadRows(prompt.lead); up > 0 {
		b.WriteString("\x1b[")
		b.WriteString(itoa(up))
		b.WriteString("A")
	}
	// To the end of the screen rather than the end of the row: what is being
	// replaced is every row the old prompt and the line occupied, and a
	// prompt that has just lost a row would otherwise leave it behind.
	b.WriteString("\x1b[J")
	b.WriteString(fresh.lead)
	e.write(b.String())

	// The whole line goes back under the new prompt. The incremental repaint
	// cannot account for a screen it did not write, and does not try to: the
	// write above took what it knew with it, because every write through this
	// editor invalidates it. Nothing here has to remember to.
	e.row = 0
	e.redraw(fresh)
	return fresh
}
