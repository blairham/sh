// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// HistoryStyle is what a dialect does with the session's history: how it draws
// an incremental search, and what it declines to remember.
//
// Separate from EditorStyle, which is about the line being typed. This is
// about the list behind it, and the two halves are here together because they
// are one surface: the knobs decide what is in the list, and the search is how
// a person reaches it.
//
// Every field's zero value is the substrate's own answer — a search drawn
// inline in wording that names no shell, and nothing filtered — which is what
// a front end that has said nothing gets.
type HistoryStyle struct {
	// SearchPrompt is what replaces the prompt, or is drawn under the line,
	// while a reverse incremental search is running. One verb: the query
	// typed so far.
	//
	// Measured under a pty on 2026-09-05, bash 5.3.15 and zsh 5.9.2, each
	// with four lines of history and `C-r e c h o` typed at the prompt:
	//
	//	bash   (reverse-i-search)`echo': echo two
	//	zsh    P> echo two
	//	       bck-i-search: echo_
	//
	// which is two differences and not one. The wording differs, and so does
	// where it goes: bash puts it where the prompt was and zsh keeps the
	// prompt, drawing the search on a row of its own below the line. See
	// SearchBelowTheLine.
	SearchPrompt string

	// SearchFailedPrompt is the same thing once nothing older matches.
	//
	//	bash   (failed reverse-i-search)`echo': echo one
	//	zsh    failing bck-i-search: echo_
	//
	// Both keep the last line that did match on the screen, and both ring the
	// bell. Neither undoes the query, so the next backspace goes back to a
	// search that works.
	SearchFailedPrompt string

	// SearchBelowTheLine draws the search on its own row under the command
	// line, leaving the prompt and the line where they were. zsh does; bash
	// replaces the prompt instead.
	//
	// It is a field rather than a consequence of the wording because it is a
	// separate decision: a shell could word it either way in either place,
	// and reading it off the presence of a `%s` would be inferring a layout
	// from a sentence.
	SearchBelowTheLine bool

	// Control names the variable holding a colon-separated list of what not
	// to record — `ignorespace`, `ignoredups`, `ignoreboth`. bash calls it
	// HISTCONTROL; zsh spells the same two rules as `setopt` names, which is
	// a different startup surface and is not read here.
	//
	// Empty means this dialect has no such variable, and setting one by that
	// name changes nothing. Measured: ksh93 with HISTIGNORE set records the
	// lines anyway, and a variable a shell does not have is not a variable
	// that half works.
	Control string

	// Ignore names the variable holding the patterns a recorded line must not
	// match — HISTIGNORE in bash, HISTORY_IGNORE in zsh. Empty is a dialect
	// with none.
	Ignore string

	// IgnoreIsOnePattern reads that variable as a single pattern rather than
	// as a colon-separated list of them. zsh's HISTORY_IGNORE is one;
	// bash's HISTIGNORE is a list.
	IgnoreIsOnePattern bool

	// IgnoredStaysInSession keeps an ignored line in the list the up arrow
	// walks, and leaves it out of the file only.
	//
	// This is the conflict the two shells have on identical intent, and it is
	// an axis rather than a shared answer with two variable names. Measured
	// on 2026-09-05: bash 5.3.15 given `HISTCONTROL=ignorespace` drops the
	// line entirely — the up arrow at the next prompt recalls the line
	// *before* it — while zsh 5.9.2 given `setopt HIST_IGNORE_SPACE` recalls
	// the ignored line itself and simply does not write it down. One is
	// "never recorded"; the other is "recorded, not saved".
	IgnoredStaysInSession bool
}

// The substrate's own search wording, for a front end that has not said. It
// names no shell on purpose: the core does not know its successors, and a
// default borrowed from one of them would make the others look like
// deviations from it.
const (
	defaultSearchPrompt = "(reverse-search)`%s': "
	defaultSearchFailed = "(failed reverse-search)`%s': "
)
