// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// The special widgets: names the line editor calls itself, at fixed moments,
// where the shell has defined a widget under them. zshzle(1) lists them;
// these are the three a startup file reaches through every highlighter and
// `add-zle-hook-widget`. Measured 2026-10-02 through a pseudo-terminal against
// zsh 5.9.2 (#5398), each widget appending `$WIDGET` and `$BUFFER` to a file,
// with `ab` and Return typed, then `xy` and ^C:
//
//	init zle-line-init []     once the prompt is drawn, before any key
//	pre [a]                   before the redraw each key makes
//	pre [ab]
//	pre [ab]                  and once more as the line is accepted
//	finish [ab]               then the line is finished
//	init zle-line-init []     the next prompt — and the ^C'd line has no
//	                          finish, only the next prompt's init
//
// A widget that changes the line changes what is drawn and what is run: the
// line is taken back from it as from any other widget.

// specialWidget runs the shell's widget for one of those names, if it has
// one, and reports whether it ran. Never from inside one: a redraw made while
// a special widget runs does not ask for another pre-redraw.
func (e *editor) specialWidget(name string, prompt drawnPrompt) bool {
	if !e.specials || e.runFunc == nil || e.inSpecial {
		return false
	}
	e.inSpecial = true
	defer func() { e.inSpecial = false }()
	out, ok := e.runFunc(name, e.give(), editorActions{e: e, prompt: prompt})
	if !ok {
		return false
	}
	e.line = []rune(out.Buffer)
	e.pos = min(max(out.Cursor, 0), len(e.line))
	e.postdisplay = out.Postdisplay
	return true
}

// accepted finishes a line that was accepted: the last pre-redraw, the
// finish widget, and then the ordinary ending. It answers with the line as
// the widgets left it, which is what runs.
func (e *editor) accepted(prompt drawnPrompt) string {
	e.specialWidget("zle-line-pre-redraw", prompt)
	e.specialWidget("zle-line-finish", prompt)
	// The line is finished, so the drawing that ends it asks for no more
	// pre-redraws: measured, nothing is called after the finish.
	e.inSpecial = true
	e.endLine(prompt, "")
	e.inSpecial = false
	return string(e.line)
}
