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
// one, and reports whether it ran.
//
// **Never from inside any of the shell's own actions**, special or not. A
// redraw made while a special widget runs does not ask for another
// pre-redraw, which would never end; and a redraw made while an ordinary
// widget or a descriptor handler runs does not ask for one either, which is
// measured. zsh 5.9.2 on 2026-10-04, through a pseudo-terminal, a pre-redraw
// widget logging each call: a `self-insert` replacement that runs `zle
// .self-insert` and then `zle -R` gets **no** pre-redraw from either — one
// comes after the widget returns, before the redraw the key makes — and a
// `zle -F` handler's `zle -R` gets none at all. Here both asked for one in
// the middle of the widget, and the shell's half of that call took the
// calling widget's whole state with it (#5864).
func (e *editor) specialWidget(name string, prompt drawnPrompt) bool {
	if !e.specials || e.runFunc == nil || e.inShell {
		return false
	}
	out, ok := e.runShell(name, prompt)
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
	// The key that ends the line is the next line's last widget, whatever
	// spelled it. See LastWidget.
	e.last, e.keyBytes, e.keyBinding = LastWidget{Accepted: true, Known: true}, nil, nil
	e.specialWidget("zle-line-pre-redraw", prompt)
	e.specialWidget("zle-line-finish", prompt)
	// The line is finished, so the drawing that ends it asks for no more
	// pre-redraws: measured, nothing is called after the finish.
	e.inShell = true
	e.endLine(prompt, "")
	e.inShell = false
	return string(e.line)
}

// runShell runs one of the shell's widgets with inShell held for the length
// of the call, so nothing the widget makes the editor do asks for a special
// widget. Put back rather than cleared, because the editor can be inside one
// action already when it starts another.
func (e *editor) runShell(name string, prompt drawnPrompt) (Line, bool) {
	was := e.inShell
	e.inShell = true
	defer func() { e.inShell = was }()
	return e.runFunc(name, e.give(), editorActions{e: e, prompt: prompt})
}
