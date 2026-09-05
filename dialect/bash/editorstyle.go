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
		// Measured under a pty: a hundred matches is where it stops asking
		// and starts asking, it counts the matches and not the rows, it does
		// not echo the key that answered, and it rings the bell at anything
		// that is not `y` or `n` rather than taking it as an answer.
		ListQuery:                   "Display all %[1]d possibilities? (y or n)",
		ListQueryAcceptsOnlyYesOrNo: true,
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
		// Measured: a bare Tab in a directory holding a `.hidden` lists it
		// along with everything else. readline calls this
		// `match-hidden-files` and documents it as on by default, which is
		// what the run shows.
		CompletionMatchesHiddenFiles: true,
	}
}
