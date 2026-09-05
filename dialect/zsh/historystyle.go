// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/repl"

// HistoryStyle is how zsh searches its history and what it declines to keep.
//
// Measured under a pseudo-terminal on 2026-09-05, zsh 5.9.2, alongside the
// same probe run against bash — and the two differ in where the search goes as
// well as in what it says. zsh leaves the prompt and the line alone and draws
// the search on a row of its own underneath, with a trailing underscore where
// the query is being typed:
//
//	P> echo two
//	bck-i-search: echo_
//
// and once nothing older matches:
//
//	failing bck-i-search: echo_
//
// The knob is `HISTORY_IGNORE`, and it is a *single* pattern rather than a
// colon-separated list of them — the colon is not a separator here, and
// reading it as one would break a pattern that contains a path.
//
// An ignored line stays in the session, which is the conflict with bash on
// identical intent. Measured: with `HISTORY_IGNORE='ls*'` set, `ls -d .` runs,
// is absent from the file afterwards, and is what the up arrow recalls at the
// very next prompt. bash's answer to the same request is to forget it
// entirely.
//
// `HIST_IGNORE_SPACE` and `HIST_IGNORE_DUPS` are zsh's spelling of the two
// rules bash keeps in HISTCONTROL, and they are `setopt` names rather than
// variables — a different startup surface, and one this shell does not yet
// have a place to hold. They are measured and recorded in
// docs/spec/history.md, and left unimplemented rather than approximated with a
// variable zsh does not have.
func HistoryStyle() repl.HistoryStyle {
	return repl.HistoryStyle{
		SearchPrompt:          "bck-i-search: %s_",
		SearchFailedPrompt:    "failing bck-i-search: %s_",
		SearchBelowTheLine:    true,
		Ignore:                "HISTORY_IGNORE",
		IgnoreIsOnePattern:    true,
		IgnoredStaysInSession: true,
	}
}
