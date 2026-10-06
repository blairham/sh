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
		// `^X r` and `^X s` are the plain searches too, in this keymap
		// alone. See repl.EditorStyle.SearchOnControlX (#5904).
		SearchOnControlX: true,
		// And `^G` gives the line up. See
		// repl.EditorStyle.SendBreakOnControlG (#5913).
		SendBreakOnControlG: true,
		// Measured under a pty. The threshold is the same hundred, and
		// everything about the question is different: it names the shell,
		// counts the rows the matches would take as well as the matches,
		// echoes the key it read, and treats the first key it is given as
		// the answer — `n`, `q` and `x` all decline alike, with no bell and
		// no second asking.
		ListQuery: "zsh: do you wish to see all %[1]d possibilities (%[2]d lines)? ",
		// And how many matches it takes, which in this shell is a parameter
		// a person sets at the prompt rather than a number built in.
		// Measured 2026-09-28 through a pseudo-terminal against zsh 5.9.2 —
		// five matches in one row and `LISTMAX=5` asks, `LISTMAX=6` does
		// not, `LISTMAX=0` asks only about a listing that will not fit, and
		// every negative asks whatever the size. See
		// repl.editor.listQueryAsks, where the grid is (#4993).
		ListQueryThresholdParameter: "LISTMAX",
		KeySequenceWaitParameter:    "KEYTIMEOUT",
		ListQueryEchoesTheKey:       true,
		// And the answer takes the question's row. See
		// repl.EditorStyle.ListQueryAnswerTakesTheQuestionsRow (#6119).
		ListQueryAnswerTakesTheQuestionsRow: true,
		// Measured 2026-09-14 through a pseudo-terminal: this shell writes
		// `\e[?2004h` after the prompt and `\e[?2004l\r` after the line it
		// read, and draws a paste that arrives in reverse video until the
		// next keystroke. ksh93 does neither, which is what makes these
		// questions a dialect answers (#2775).
		BracketedPaste: true,
		// And the two sequences are a parameter's, which zle creates when it
		// loads and a person unsets to turn the bracketing off. See
		// zleboot.go.
		BracketedPasteParameter: bracketedPasteParameter,
		// And ESC with a digit or a minus is a count for the next key.
		// Measured; see repl's prefixarg.go (#5498).
		PrefixArgument:     true,
		PastedTextStyle:    "\x1b[7m",
		PastedTextStyleEnd: "\x1b[27m",
		// And a control character in the line is a caret in standout, which
		// is this shell's `zle_highlight` default for `special`. Measured;
		// see repl.EditorStyle.ControlCharacterStyle (#5972).
		ControlCharacterStyle:    "\x1b[7m",
		ControlCharacterStyleEnd: "\x1b[27m",
		// And a Tab with nothing but blanks before the cursor is a tab, when
		// this editor's own completion is the one asked. Measured; see
		// repl.EditorStyle.TabOnABlankLineTypesItself (#6119).
		TabOnABlankLineTypesItself: true,
		// What this shell calls typing, so a widget put in front of it actually
		// intercepts a printable key. See repl's EditorStyle.SelfInsertWidget
		// and #2485.
		SelfInsertWidget: "self-insert",
		// And the editor calls zle-line-init, zle-line-pre-redraw and
		// zle-line-finish where they are defined. Measured 2026-10-02
		// through a pseudo-terminal (#5398).
		SpecialWidgets: true,
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
		// And the terminal's flow control, which the editor leaves on while
		// this option is set — the default — so `C-s` and `C-q` are XON/XOFF
		// and never reach a widget; `unsetopt flowcontrol` hands both keys to
		// the editor from the next prompt. See repl.EditorStyle.FlowControlOption
		// and internal/tty's Raw for the measurement (#5943).
		FlowControlOption: "FLOW_CONTROL",
		// And the bell: a widget that returns non-zero rings it, and
		// `unsetopt beep` silences every bell the editor rings. See
		// repl.EditorStyle.BeepOption (#6108).
		BeepOption:            "BEEP",
		RingsWhenAWidgetFails: true,
		// What zsh keeps of the settings a command leaves: all of them, with
		// line buffering and echo back on, and even after a signal ended the
		// command. See repl.EditorStyle.KeptCanonical (#6105).
		KeptCanonical:        true,
		KeptEcho:             true,
		KeepsWhatASignalLeft: true,
		// And the option the whole editor runs under, which this shell alone
		// in the panel has: `unsetopt zle` is an interactive shell with no
		// line editor, and `-o interactive +o zle` — `-fiV +Z` — is how zsh's
		// own `zpty`-driven test files start the shell they drive.
		//
		// Measured 2026-09-25 on zsh 5.9.2 (aarch64-apple-darwin25.4.0),
		// `-fiV +Z` on a pseudo-terminal with `TERM=dumb` and `PS1=`/`PS2=`
		// exported, one command and then `exit`:
		//
		//	b': &\r\n[1] 6064\r\n[1]  + done       :\r\nexit\r\n'
		//
		// Nothing but what the terminal echoed and what the commands printed.
		// The same session without `+Z` writes ` \r`, `\e[?2004h` and
		// `\e[?2004l\r` around every line, so it is not that this shell emits
		// no escapes — it is that with the option off there is no editor to
		// emit them (#4472).
		RunsUnderTheOptions: []string{"ZLE"},
		// And a ^C at a prompt read with it off gives up the line typed
		// after it too. See repl.EditorStyle.InterruptWithoutTheEditorTakesTheNextLine.
		InterruptWithoutTheEditorTakesTheNextLine: true,
		// And the ground under the prompt, which is neither option's doing:
		// measured, this shell writes all four sequences with both options
		// turned off, and bash writes nothing in any case. The three resets
		// are the attributes a run of output is most likely to have left on;
		// the erase is what stops a shorter prompt leaving the tail of a
		// longer one behind it.
		ClearBeforeThePrompt: "\x1b[0m\x1b[27m\x1b[24m\x1b[J",
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
		InsertLastWordTakesArguments:      true,
		// And one about vi command mode: measured on `   ab` with Escape,
		// `$` and `I`, zsh puts the cursor at the first character that is not
		// a blank and bash puts it at column 0. It is the only place the two
		// shells' command modes disagree about a key this editor offers.
		ViInsertAtStartOfLineSkipsLeadingBlanks: true,
		// The matches are drawn on the keystroke that found them ambiguous
		// rather than on a second one. Measured 2026-09-19 through a
		// pseudo-terminal on `: big/aa0` against ten `aa0N/` directories:
		// this shell writes the bell and the listing on the first Tab where
		// bash writes the bell alone and waits for a second. Named for the
		// option because `unsetopt autolist` is a person asking for bash's
		// answer, and measured, it gets it.
		ListMatchesWithoutASecondKeyOption: "AUTO_LIST",
		// And whether a second key lists where the first did not, which in
		// this shell is an option of its own and off: with AUTO_LIST off no
		// key lists. See repl.EditorStyle.ListMatchesOnASecondKeyOption.
		ListMatchesOnASecondKeyOption: "BASH_AUTO_LIST",
		// And whether a key that fills in what the matches agree on counts
		// as one of those keys: not while LIST_AMBIGUOUS and a listing
		// option are on, and as the first otherwise (#6220). See
		// repl.EditorStyle.FillStandsAsideOption.
		FillStandsAsideOption: "LIST_AMBIGUOUS",
		// And the menu completion a Tab starts: on the press after a
		// listing by default, on the first press under MENU_COMPLETE, with
		// the bell as it starts (#6197). See
		// repl.EditorStyle.MenuOnARepeatedCompletionOption.
		MenuOnARepeatedCompletionOption: "AUTO_MENU",
		MenuOnTheFirstCompletionOption:  "MENU_COMPLETE",
		BellRingsWhenAMenuStarts:        true,
		// And how the listing is arranged, which zsh's two options decide.
		// See repl.EditorStyle.ListPackedOption (#6157).
		ListPackedOption:    "LIST_PACKED",
		ListRowsFirstOption: "LIST_ROWS_FIRST",
		// And whether a listing of files marks each one as `ls -F` does,
		// which is the completion system's `compadd -f` answer too. See
		// repl.EditorStyle.ListTypesOption (#6179).
		ListTypesOption: "LIST_TYPES",
		// And whether the cursor goes back up to the line after a listing.
		// See repl.EditorStyle.ListReturnsToTheLineOption (#6129).
		ListReturnsToTheLineOption: "ALWAYS_LAST_PROMPT",
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

// MailStyle is the mailbox check this shell makes between prompts.
//
// Measured 2026-09-28 through a pseudo-terminal against zsh 5.9.2 — `zsh -f
// -i` under `env -i PATH=/usr/bin:/bin TERM=xterm` with a scratch `HOME` —
// driving a real session and growing the file between commands. See
// repl.MailStyle, where the rows are and where the `cat` that stops the
// reports is (#4994).
func MailStyle() repl.MailStyle {
	return repl.MailStyle{
		File:     "MAIL",
		Path:     "MAILPATH",
		Interval: "MAILCHECK",
		Message:  "You have new mail.",
	}
}
