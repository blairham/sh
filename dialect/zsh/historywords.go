// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// `$historywords` is every word of the history list, most recently typed
// first.
//
// The same list `$history` publishes, cut into words — which is the sentence
// the name promises and is *not* what a probe that can only reach a
// non-interactive shell would have said. A script's history list is empty in
// both shells, so `${#historywords}` is `0` here and there whatever this
// function does, and an implementation graded on that alone would have passed
// with the words in any order or with no words at all.
//
// So the order and the cut were measured on a shell with a history list in
// it: zsh 5.9.2, 2026-09-27, `zsh -f -i` reading its commands from a pipe
// under `env -i PATH=/usr/bin:/bin TERM=dumb` with a scratch `HOME`, after
//
//	print one two
//	ls -l "a b" c
//	zmodload zsh/parameter
//
// the array reads `zsh/parameter zmodload c "a b" -l ls two one print` —
// which is **the whole word sequence in reverse**: the newest line first, and
// inside each line the last word first. Not "the newest line's words in
// order", which is the reading that agrees with it for a one-word line and
// parts from it at the second.
//
// The cut is the lexer's and the quoting stays on: `"a b"` comes back as one
// word with its quotes, and `;` and `|` are words of their own. That is
// [interp.Runner.ShellWords], which is the split behind `${(z)…}` — the same
// function rather than a second one, because "what are this line's words" has
// one answer and a second splitter beside it would be where the two came to
// disagree.
//
// # The reference has three cuts and this shell has one, which is a choice
//
// That shell keeps a word list **per entry**, filled by whatever put the
// entry there, so the answer depends on the route in. Measured the same day
// and in the same way, three routes over the same two lines:
//
//	route                     `ls -l "a b" c` becomes
//	the line editor           ls  -l  "a b"  c          the lexer's words
//	fc -R of a history file   ls  -l  "a  b"  c         split on whitespace
//	print -s 'ls -l "a b" c'  ls -l "a b" c             the whole line, one word
//
// Three answers to one question, and the differences are that shell's
// bookkeeping rather than a rule about words: a line it lexed has real words,
// a line it read out of a file has never been lexed, and `print -s` sets a
// word list of one outright.
//
// This shell keeps one list of lines and no per-entry word list, so it cuts
// them one way, and the way it cuts them is **the line editor's** — the route
// the parameter exists for. A completer reading `$historywords` is reading
// what a person typed, and `"a b"` is one word there in the shell being
// modeled. The two routes that disagree are the two nothing types: a history
// file and a builtin that pushes a line.
//
// Where the words are unquoted the three cuts coincide, which is what the
// test grades — `fc -R` of a file of plain commands is byte-identical between
// the two shells.
//
// `$history` is what supplies the lines, and deliberately through the same
// function rather than through [interp.Runner.HistoryEntries] directly: that
// list holds the command now running, which the table drops outside the line
// editor — so reading the raw list here put the reading line's own words into
// the answer, one entry ahead of `${#history}`. See zshHistoryEvents.
func zshHistoryWordsView(r *interp.Runner) []string {
	entries := zshHistoryEvents(r)
	var words []string
	for _, entry := range entries {
		words = append(words, r.ShellWords(entry)...)
	}
	// Reversed whole, which is the measurement above and not a loop written
	// backwards over the lines: a per-line reversal that kept the lines in
	// order would put the oldest line's words first.
	for i, j := 0, len(words)-1; i < j; i, j = i+1, j-1 {
		words[i], words[j] = words[j], words[i]
	}
	return words
}

// registerHistoryWords installs `$historywords`.
//
// Readonly and hidden, measured with the rest: `${(t)historywords}` is
// `array-readonly-hide-hideval-special`.
func registerHistoryWords(r *interp.Runner) {
	r.SetDynamicArray("historywords", zshHistoryWordsView)
	r.MarkReadonly("historywords")
	hideModuleParameter(r, "historywords")
}
