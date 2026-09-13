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
		// What this shell calls typing, so a widget put in front of it actually
		// intercepts a printable key. See repl's EditorStyle.SelfInsertWidget
		// and #2485.
		SelfInsertWidget: "self-insert",
		// Output that never ended its line. Measured 2026-09-12 through a
		// pseudo-terminal with an rc file ending `printf 'LEFTOVER'`: this
		// shell writes a bold, inverse `%` where the output stopped, pads to
		// the end of the row so the terminal wraps, and puts the prompt on the
		// row below — where bash draws its prompt straight onto the output.
		//
		// The two options one at a time, which is what says they are not
		// independent: `nopromptsp` leaves the return and drops the mark, and
		// `nopromptcr` drops **both**, though `promptsp` is still set. So the
		// return is the outer of the two and both names are given here.
		//
		// This is what keeps powerlevel10k's `fetching gitstatusd ..` progress
		// line from having the prompt drawn against it (#2477).
		MarkUnfinishedOutputOption:  "PROMPT_SP",
		ReturnBeforeThePromptOption: "PROMPT_CR",
		// And the erase, which is neither option's doing: measured, this shell
		// writes it with both of them turned off, and bash writes it in no
		// case at all.
		ClearsBelowThePrompt: true,
		UnfinishedOutputMark: "\x1b[1m\x1b[7m%\x1b[27m\x1b[1m\x1b[0m",
		// Measured under a pty against zsh 5.9 started with no startup files,
		// one keystroke at a time. These are the four places where the same
		// key does something different from bash, and every one of them is on
		// the daily path: see EditorStyle for the line each was measured on.
		//
		// The characters are zsh's own `WORDCHARS` default, confirmed a
		// character at a time by pressing the key rather than by reading the
		// variable.
		WordCharacters:                         wordCharacters,
		KillToStartOfLineTakesTheWholeLine:     true,
		KillWordBeforeCursorUsesWordCharacters: true,
		ForwardWordStopsBeforeTheNextWord:      true,
		TransposeAtTheStartSwapsTheFirstTwo:    true,
		// And three more about taking a change back and about `M-.`, measured
		// the same way. One `^_` after `echo abcdef` leaves `echo abcde`
		// here and an empty line in bash; `^A`, `^K`, `^_` leaves the cursor
		// at the start of the line here and at the end of it in bash; and a
		// press of `M-.` past the oldest line keeps that line's last word
		// here where bash takes the word back off the line.
		UndoTakesBackOneKeystrokeAtATime:  true,
		UndoRestoresTheCursorToWhereItWas: true,
		LastArgumentStaysOnTheOldestLine:  true,
		// And one about vi command mode: measured on `   ab` with Escape,
		// `$` and `I`, zsh puts the cursor at the first character that is not
		// a blank and bash puts it at column 0. It is the only place the two
		// shells' command modes disagree about a key this editor offers.
		ViInsertAtStartOfLineSkipsLeadingBlanks: true,
		// Measured: a bare Tab in a directory holding a `.hidden` lists
		// everything except it, and `.` completes it outright because it is
		// then the only match. Left false rather than written out, so that
		// what a dialect *says* is what it differs about.
	}
}

// wordCharacters is this shell's `WORDCHARS` default: what joins letters and
// digits into one word, for the line editor and for the `[[:WORD:]]` pattern
// class alike. One constant because they are one measurement — the prelude
// gives the variable this value, and a script that reassigns it moves the
// class with it.
const wordCharacters = "*?_-.[]~=/&;!#$%^(){}<>"
