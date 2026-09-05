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
// And an ignored line is *gone*, which is the half zsh does not agree with.
// Measured with `HISTCONTROL=ignorespace`: after ` echo hidden`, the up arrow
// at the next prompt recalls the line before it, and bash's own `history`
// cannot see the hidden one either. Recorded or not recorded, with nothing in
// between — so IgnoredStaysInSession is left off.
func HistoryStyle() repl.HistoryStyle {
	return repl.HistoryStyle{
		SearchPrompt:       "(reverse-i-search)`%s': ",
		SearchFailedPrompt: "(failed reverse-i-search)`%s': ",
		Control:            "HISTCONTROL",
		Ignore:             "HISTIGNORE",
	}
}
