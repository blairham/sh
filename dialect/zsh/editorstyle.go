// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/repl"

// EditorStyle is what zsh draws while a line is being typed.
//
// // Measured: real zsh leaves the abandoned line as it is and draws no mark
// after it.
func EditorStyle() repl.EditorStyle {
	return repl.EditorStyle{
		Interrupt: "",
		// Measured under a pty. The threshold is the same hundred, and
		// everything about the question is different: it names the shell,
		// counts the rows the matches would take as well as the matches,
		// echoes the key it read, and treats the first key it is given as
		// the answer — `n`, `q` and `x` all decline alike, with no bell and
		// no second asking.
		ListQuery:             "zsh: do you wish to see all %[1]d possibilities (%[2]d lines)? ",
		ListQueryEchoesTheKey: true,
		// Measured under a pty against zsh 5.9 started with no startup files,
		// one keystroke at a time. These are the four places where the same
		// key does something different from bash, and every one of them is on
		// the daily path: see EditorStyle for the line each was measured on.
		//
		// The characters are zsh's own `WORDCHARS` default, confirmed a
		// character at a time by pressing the key rather than by reading the
		// variable.
		WordCharacters:                         "*?_-.[]~=/&;!#$%^(){}<>",
		KillToStartOfLineTakesTheWholeLine:     true,
		KillWordBeforeCursorUsesWordCharacters: true,
		ForwardWordStopsBeforeTheNextWord:      true,
		TransposeAtTheStartSwapsTheFirstTwo:    true,
	}
}
