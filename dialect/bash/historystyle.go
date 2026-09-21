// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import "github.com/blairham/sh/repl"

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
		// reads such a file back — see EntriesMayCarryAHashTimestampLine for
		// what counts as one and why the decision is the file's rather than
		// each line's. Neither of the other two encodings is bash's, and the
		// backslash one was measured in the same probe as a control: bash
		// reading `cat <<EOF\`, `a\`, `EOF` hands back three entries, so a
		// trailing backslash is text here and not a join (#4013).
		EntriesMayCarryAHashTimestampLine: true,

		Control: "HISTCONTROL",
		Ignore:  "HISTIGNORE",
	}
}
