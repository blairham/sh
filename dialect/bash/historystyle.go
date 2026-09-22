// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// HistoryStyle is how bash searches its history and what it declines to keep.
//
// Measured under a pseudo-terminal on 2026-09-05, bash 5.3.15, with four lines
// of history and the rendered screen reconstructed from the bytes rather than
// read by eye. `C-r` replaces the prompt, keeps the matched line after it, and
// puts the cursor at the start of the match:
//
//	(reverse-i-search)`echo': echo two
//
// and once nothing older matches, the wording changes and the line that did
// match stays where it is:
//
//	(failed reverse-i-search)`echo': echo one
//
// The knobs are two variables. `HISTCONTROL` is a colon-separated list —
// `ignorespace`, `ignoredups`, `ignoreboth` — and `HISTIGNORE` a
// colon-separated list of patterns matched against the whole line: measured,
// `HISTIGNORE=ls*:pwd` drops `ls -la` and `pwd` and keeps `pwd x`.
//
// An **empty line** in a history file is a gap rather than a command, and
// EmptyLinesAreEntries is left off for it. Measured 2026-09-21 on bash
// 5.3.20 with `env -i` and a scratch HOME: a blank first, last, between two
// commands, two in a row, and among `#` time lines is dropped every time, on
// all three read routes — `-r`, `-n`, and the read at the first `set -o
// history`. Empty and not blank: a line of spaces and a line of one tab come
// back as entries of their own, which is the same line bash draws for the
// lines a script *runs*. zsh is the shell that keeps the blank, so it is zsh
// that states something and bash that leaves the substrate's answer alone
// (#4024).
//
// And an ignored line is *gone*, which is the half zsh does not agree with.
// Measured with `HISTCONTROL=ignorespace`: after ` echo hidden`, the up arrow
// at the next prompt recalls the line before it, and bash's own `history`
// cannot see the hidden one either. Recorded or not recorded, with nothing in
// between — so IgnoredStaysInSession is left off.
func HistoryStyle() repl.HistoryStyle {
	return repl.HistoryStyle{
		SearchPrompt:       "(reverse-i-search)`%s': ",
		SearchFailedPrompt: "(failed reverse-i-search)`%s': ",
		// The file's own encoding, measured 2026-09-21 on bash 5.3.20 from
		// script files with no terminal and a scratch HOME. bash writes a
		// `#<epoch>` line in front of each entry when it was told to record
		// when a line ran, and leaves those lines out of the list when it
		// reads such a file back — see [repl.HashTimestampLines] for what
		// counts as one and why the decision is the file's rather than each
		// line's. This is the answer for a shell that was **not** told to
		// record when each line ran; historyStyle is where the other one is.
		// Neither of the other two encodings is bash's, and the backslash one
		// was measured in the same probe as a control: bash reading `cat
		// <<EOF\`, `a\`, `EOF` hands back three entries, so a trailing
		// backslash is text here and not a join (#4013).
		HashTimestampLines: repl.HashTimestampLinesWhenTheFileOpensWithOne,
		// And the variable that moves it to the third answer, which the
		// session's own reader consults as well — see historyStyle, where
		// the measurement is, and repl.HistoryStyle.InForce, which is the
		// one place either reader asks.
		HashTimestampsVariable: "HISTTIMEFORMAT",

		Control: "HISTCONTROL",
		Ignore:  "HISTIGNORE",
	}
}

// historyStyle is [HistoryStyle] for a read or a write this shell is doing
// *now*, which is not the same thing: how a history file spells an entry
// depends on whether the shell was told to record when each line ran, and
// that is a variable a script sets and unsets rather than a property of the
// dialect.
//
// HISTTIMEFORMAT is the variable, and **set** is the whole of the test —
// measured 2026-09-22 on bash 5.3.20, the empty string puts a read into the
// mode exactly as `%F %T ` does, and `unset HISTTIMEFORMAT` takes it back
// out. Its *value* decides only what a listing draws in front of an entry.
//
// Every reader and writer of a history file in this dialect goes through
// here rather than through [HistoryStyle], so a file this shell wrote a
// moment ago is one it reads back the same way. [HistoryStyle] is what the
// front end is handed, once, when the session is built — and it puts the
// same question at each of its own reads and writes, through
// [repl.HistoryStyle.InForce], which is the one place either side asks.
func historyStyle(r *interp.Runner) repl.HistoryStyle {
	return HistoryStyle().InForce(r.GetVar)
}
