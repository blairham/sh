// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import "github.com/blairham/sh/repl"

// EditorStyle is what bash draws while a line is being typed.
//
// // Measured: real bash draws `^C` after the abandoned line and starts the next
// prompt below it.
func EditorStyle() repl.EditorStyle {
	return repl.EditorStyle{
		Interrupt: "^C",
		// bash keeps the terminal settings a command leaves, `-echo`
		// included, with line buffering back on. See
		// repl.EditorStyle.KeptCanonical (#6105).
		KeptCanonical: true,
		// And `^D` on an empty line is refused while IGNOREEOF is set, as
		// many times as it says or ten. See
		// repl.EditorStyle.IgnoreEndOfInputOption (#6239).
		IgnoreEndOfInputParameter: "IGNOREEOF",
		EndOfInputRefusals:        10,
		// Measured under a pty: a hundred matches is where it stops asking
		// and starts asking, it counts the matches and not the rows, it does
		// not echo the key that answered, and it rings the bell at anything
		// that is not `y` or `n` rather than taking it as an answer.
		ListQuery:                   "Display all %[1]d possibilities? (y or n)",
		ListQueryAcceptsOnlyYesOrNo: true,
		// Measured 2026-09-14 through a pseudo-terminal: this shell writes
		// `\e[?2004h` before the prompt and `\e[?2004l\r` after the line it
		// read, and draws a paste that arrives in reverse video until the
		// next keystroke. ksh93 does neither, which is what makes these
		// questions a dialect answers (#2775).
		BracketedPaste:     true,
		PastedTextStyle:    "\x1b[7m",
		PastedTextStyleEnd: "\x1b[27m",
		// And `bind 'set enable-bracketed-paste off'` turns it off. See
		// repl.EditorStyle.BracketedPasteSetting (#6264).
		BracketedPasteSetting: bracketedPasteSetting,
		// And sets it off on a terminal that cannot take the markers. See
		// repl.EditorStyle.BracketedPasteOffForTerminals (#6310).
		BracketedPasteOffForTerminals:        []string{"dumb", "vt52", "emacs"},
		BracketedPasteOffWithoutADescription: true,
		// And ^D at a continuation prompt leaves the row as it is when
		// there are no markers to take back. See
		// repl.EditorStyle.EndOfInputWithNoWordStaysOnTheRow (#6310).
		EndOfInputWithNoWordStaysOnTheRow: true,
		// Every field about words is left at its zero value on purpose, and
		// they are bash's measured answers rather than an absence of one: a
		// word is letters and digits, `^U` kills only what is before the
		// cursor, `^W` is delimited by whitespace, `M-f` stops at the end of
		// the word rather than before the next, and `^T` at the start of the
		// line does nothing. zsh disagrees with all five.
		//
		// So are the three about undo and `M-.`: one `^_` takes back the
		// whole run of typing rather than one character of it, it leaves the
		// cursor after the text it put back rather than where the change was
		// made, and `M-.` past the oldest line takes the word it inserted
		// back off. zsh disagrees with all three of those too, and both bash
		// 5.3.15 and bash 3.2.57 give these answers.
		//
		// And the one field vi command mode adds is left at its zero value
		// for the same reason: measured on `   ab` with Escape, `$` and `I`,
		// bash inserts at column 0 where zsh skips the indent. The rest of
		// the command mode agrees between the two shells and between bash
		// 5.3.15 and bash 3.2.57.
		//
		// Measured: a bare Tab in a directory holding a `.hidden` lists it
		// along with everything else. readline calls this
		// `match-hidden-files` and documents it as on by default, which is
		// what the run shows.
		CompletionMatchesHiddenFiles: true,
		// readline marks a symlinked directory only once the word names it
		// whole. See repl.EditorStyle.SymlinkedDirectoryMarkedWhenNamedWhole.
		SymlinkedDirectoryMarkedWhenNamedWhole: true,
		// readline runs under the editing modes: `set +o emacs +o vi`
		// turns it off and the terminal gathers the line (#5922). See
		// repl.EditorStyle.RunsUnderTheOptions.
		RunsUnderTheOptions: []string{"emacs", "vi"},
		// And the bell, which this shell rings for an ambiguous completion
		// whether or not it fills a prefix in. Measured 2026-09-19 through a
		// pseudo-terminal on twelve directories agreeing on `aa`: one Tab
		// writes `\a` and then the `a`, where zsh writes the `a` alone. A
		// unique match is silent in both, so what differs is the middle
		// state — see repl's EditorStyle field for the three rows, and note
		// that bash 3.2.57 answers identically (#3735).
		BellRingsOnAnAmbiguousCompletionThatInserts: true,
		// And for a key with nothing to act on: `^F` at the end of the
		// line, Backspace at its start, Down on the newest line. Measured
		// 2026-10-06 through a pseudo-terminal; zsh rings for none of the
		// motions and deletes — see the repl field for the table (#6240).
		BellRingsWhenAnEditHasNothingToActOn: true,
		// The case keys, transpose-words and `^V`, which readline binds in
		// the emacs keymap, and `^V` in both vi modes as well (#6259) — and
		// where its words differ from the other dialect's: `M-c` raises a word's
		// first character even when it is a digit, and `M-t` after the last
		// word takes the blanks after it along. Measured 2026-10-06 through a
		// pseudo-terminal; see the repl fields for the rows (#6250).
		WordKeys:                         true,
		QuotedInsertInViInsert:           true,
		QuotedInsertInViCommand:          true,
		ViInsertTypesTheseKeys:           viInsertTypedKeys,
		CapitalizeTakesTheFirstCharacter: true,
		TransposeWordsReachesTheLineEnd:  true,
		// And ESC with a digit or a minus is a count, drawn as `(arg: N)` in
		// place of the prompt's last row and read and spent readline's way.
		// Measured 2026-10-06 through a pseudo-terminal; see the repl fields
		// for the rows (#6248), and for how `M-t`, `M-.`, `^K` and `^T` read
		// one (#6265).
		PrefixArgument:                     true,
		CountPrompt:                        "(arg: %d) ",
		CountReadAsReadline:                true,
		NegativeCountTypesNothing:          true,
		CountStopsWhereItCannotAct:         true,
		NegativeCaseCountGoesBackward:      true,
		TransposeWordsCountAsReadline:      true,
		YankLastArgCountAsReadline:         true,
		KillLineReadsOnlyTheSign:           true,
		TransposeCharsTakesNoNegativeCount: true,
	}
}

// viInsertTypedKeys are the control keys bash's vi insert mode puts in the
// line as they are: readline's vi-insert keymap has self-insert on each.
// Measured 2026-10-06 against bash 5.3.20 (#6301); see
// repl.EditorStyle.ViInsertTypesTheseKeys.
const viInsertTypedKeys = "\x01\x02\x05\x06\x07\x0b\x0c\x0f\x18\x1c\x1d\x1e"
