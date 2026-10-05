// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// A diagnostic the shell writes while one of its widgets runs gets a row of
// its own (#6085).
//
// Measured 2026-10-05 through a pseudo-terminal against zsh 5.9.2, a widget on
// a key with `ab` typed: before the first diagnostic the shell itself writes
// to the terminal, the editor moves down past the line's last row and ends
// it — `ab\r\r\nw: read-only variable: KEYS` — and after the call the prompt
// is drawn again from scratch, the ground a fresh prompt is drawn on and every
// row of it. Once per call: `cd /nx1; print -n X; cd /nx2` writes the second
// message straight after the `X`. From the middle of a line wrapped over two
// rows it goes down to the second first. Whether the error is fatal does not
// enter into it, and what decides it is in interp.Runner.BeforeDiagnostic.
//
// Here the editor used to know nothing until the widget returned, so the
// message was written at the cursor, onto the line's own row, and with the
// cursor in the middle of the line it covered the rest of it until the redraw.

// DiagnosticActions is the handle's half for a shell about to write a
// diagnostic while a widget runs, asked for with a type assertion like
// ArgumentActions.
type DiagnosticActions interface {
	// EndTheRowForADiagnostic moves below the line and ends the row, so
	// what is written next starts on one of its own, and has the next draw
	// put the prompt back from scratch. Only the first call does anything
	// until the line is drawn again.
	EndTheRowForADiagnostic()
}

func (a editorActions) EndTheRowForADiagnostic() { a.e.endTheRowForADiagnostic(a.prompt) }

// endTheRowForADiagnostic is EndTheRowForADiagnostic.
func (e *editor) endTheRowForADiagnostic(prompt drawnPrompt) {
	if e.rowEnded {
		return
	}
	e.rowEnded = true
	e.toLastRow(e.live(prompt))
	e.write(e.newline())
	e.row = 0
}

// redrawAfterAnEndedRow is the half of the above a draw does: where a
// diagnostic took the row, the prompt goes back whole, on the ground every
// fresh prompt is drawn on — measured, zsh writes the reset and the erase,
// the prompt's leading rows and then the line. It reports nothing; the draw
// that called it goes on as a whole one.
func (e *editor) redrawAfterAnEndedRow(prompt drawnPrompt) {
	if !e.rowEnded {
		return
	}
	e.rowEnded = false
	e.write(e.clearBefore + prompt.lead)
	e.row = 0
}
