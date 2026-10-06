// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"slices"
	"strings"
)

// executeNamedCmd reads the name of a widget under the line and runs it:
// zsh's `execute-named-cmd`, on `M-x` (#6241).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, `zsh -f`,
// TERM=xterm, `echo abc def` with the cursor on the `c`:
//
//	keys after M-x             what happened
//	(nothing)                  `execute: _` drawn on the row under the line
//	up-case-word Return        the row taken away, the line `echo abC def`
//	up-ca Return               the same: a prefix one name begins with is it
//	up-ca Tab                  the row reads `execute: up-case-word_`
//	up- Tab, up- Return        the five names beginning `up-` listed under the
//	                           row, which stays, with a bell
//	up case Return             `echo abC def`: a space is typed as `-`
//	nosuch Return              a bell, and the row stays for more typing
//	Backspace, ^U, ^W          take back a character, everything, everything
//	^G                         a bell, and the row taken away
//	mine Return                a widget of the shell's own, run
//	.up-case-word Return       the dotted name too
//	ESC 2 M-x up-case-word     two words: the count reaches the widget
//
// and `$LASTWIDGET` afterwards is the widget that ran. Under TERM=dumb zsh
// draws none of the row; this editor draws it the one way whatever the
// terminal, as it does an incremental search's.
func (e *editor) executeNamedCmd(prompt drawnPrompt) {
	if e.namedWidgets == nil {
		e.ring()
		return
	}
	table := e.namedWidgets()
	var typed []rune
	// draw puts the row under the line, and under that a listing where
	// there is one to show: zsh lists the names under its own row rather
	// than under the line, so the two go down together.
	draw := func(listing ...string) {
		e.redraw(prompt)
		e.belowAll(prompt, append([]string{"execute: " + string(typed) + "_"}, listing...))
	}
	done := func() { e.redraw(prompt) }
	draw()
	for {
		b, got := e.readByte()
		switch got {
		case keyAbandoned:
			done()
			e.interruptRequested = true
			return
		case keyContinues:
		default:
			done()
			return
		}
		switch {
		case b == '\r' || b == '\n' || b == '\t':
			matches := namesBeginning(table, string(typed))
			if _, exact := table[string(typed)]; exact && b != '\t' {
				matches = []string{string(typed)}
			}
			switch {
			case len(matches) == 1 && b != '\t':
				done()
				e.runNamed(matches[0], table[matches[0]], prompt)
				return
			case len(matches) == 0:
				e.ring()
			default:
				if p := commonNamePrefix(matches); len(p) > len(string(typed)) {
					typed = []rune(p)
					draw()
				} else {
					e.ring()
					draw(layOutListing(candidatesNamed(matches), e.cols(), e.listLayout()).rows...)
				}
			}
		case b == ' ':
			typed = append(typed, '-')
			draw()
		case b == del || b == backspace:
			if len(typed) > 0 {
				typed = typed[:len(typed)-1]
			}
			draw()
		case b == ctrlU || b == ctrlW:
			typed = typed[:0]
			draw()
		case b == ctrlG:
			e.ring()
			done()
			return
		case b >= 0x20:
			r := rune(b)
			if b >= 0x80 {
				var err error
				if r, err = e.readTypedRune(b); err != nil {
					done()
					return
				}
			}
			typed = append(typed, r)
			draw()
		default:
			e.ring()
		}
	}
}

// runNamed runs the widget execute-named-cmd was given, as the keystroke's
// own: `$LASTWIDGET` names it afterwards, and the count goes with it.
func (e *editor) runNamed(name string, b Binding, prompt drawnPrompt) {
	e.keyBinding = &b
	if b.Function != "" {
		if _, accept := e.runShellWidget(b.Function, prompt); accept {
			e.acceptRequested = true
		}
		return
	}
	e.runWidget(b, prompt)
}

// namesBeginning is the names in the table that begin with prefix, sorted.
func namesBeginning(table map[string]Binding, prefix string) []string {
	var out []string
	for name := range table {
		if strings.HasPrefix(name, prefix) {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// commonNamePrefix is how much of the names agree.
func commonNamePrefix(names []string) string {
	p := names[0]
	for _, n := range names[1:] {
		for !strings.HasPrefix(n, p) {
			p = p[:len(p)-1]
		}
	}
	return p
}

func candidatesNamed(names []string) []Candidate {
	out := make([]Candidate, len(names))
	for i, n := range names {
		out[i] = Candidate{Word: n}
	}
	return out
}

// belowAll is below for several rows: each under the one before, and the
// cursor back where it was on the line.
func (e *editor) belowAll(prompt drawnPrompt, rows []string) {
	cols := e.cols()
	if cols <= 0 {
		return
	}
	end := (prompt.cells + len(e.line)) / cols
	down := end - e.row + 1
	col := (prompt.cells + e.pos) % cols
	var b strings.Builder
	for range down {
		b.WriteString("\r\n")
	}
	for i, row := range rows {
		if i > 0 {
			b.WriteString("\r\n")
		}
		b.WriteString("\x1b[K")
		b.WriteString(row)
	}
	b.WriteString("\r\x1b[")
	b.WriteString(itoa(down + len(rows) - 1))
	b.WriteString("A")
	if col > 0 {
		b.WriteString("\x1b[")
		b.WriteString(itoa(col))
		b.WriteString("C")
	}
	e.write(b.String())
}
