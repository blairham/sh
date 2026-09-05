// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "strings"

// What a session declines to remember.
//
// The point of these is that the list stays worth walking. A history that
// records every `ls` is one where the up arrow is a way of scrolling past
// `ls`, and the search is a way of finding one — so the knobs are part of the
// navigation rather than beside it.
//
// Which variables say so is the dialect's, in HistoryStyle. What they mean is
// here, because the meanings are shared: measured on 2026-09-05, bash 5.3.15
// and zsh 5.9.2 agree that a leading space hides a line, that a "duplicate" is
// the line immediately before it and not any earlier one, and that a pattern
// is matched against the whole line. They disagree about one thing only —
// whether an ignored line is still recallable — and that is the axis.

// historyRules is what one session was told to leave out.
type historyRules struct {
	// ignoreSpace drops a line that begins with a blank, which is the
	// gesture for "run this but do not write it down".
	ignoreSpace bool

	// ignoreDups drops a line identical to the one immediately before it.
	//
	// Immediately before, and measured rather than assumed: given `echo a`,
	// `echo a`, `echo b`, `echo a`, both shells with the option on record
	// three lines and not two. The second `echo a` is a duplicate and the
	// fourth is not, because something else happened in between.
	ignoreDups bool

	// patterns are the globs a line must not match, already split out of
	// whichever variable this dialect keeps them in.
	patterns []string

	// keepIgnored leaves an ignored line in the list the up arrow walks and
	// takes it out of the file only. See HistoryStyle.IgnoredStaysInSession.
	keepIgnored bool

	// match is the dialect's own pattern matcher, so `HISTIGNORE='@(ls|pwd)'`
	// means in the history what it means in a `case`. Nil matches nothing,
	// which is a session with no interpreter to ask.
	match func(pattern, line string) bool
}

// historyRulesFrom reads what this session was told.
//
// Through the shell's own variables rather than the process environment, for
// the reason HISTFILE is read that way: a person sets HISTCONTROL at the
// prompt and means it from the next line on.
func historyRulesFrom(style HistoryStyle, get func(string) (string, bool), match func(pattern, line string) bool) historyRules {
	rules := historyRules{keepIgnored: style.IgnoredStaysInSession, match: match}
	if style.Control != "" {
		if v, ok := get(style.Control); ok {
			// Colon-separated, and an unknown word is simply not one of the
			// three rather than an error: bash accepts `HISTCONTROL=erasedups`
			// and this shell does not erase, so the value stands and the rule
			// it does not name is off.
			for _, word := range strings.Split(v, ":") {
				switch strings.TrimSpace(word) {
				case "ignorespace":
					rules.ignoreSpace = true
				case "ignoredups":
					rules.ignoreDups = true
				case "ignoreboth":
					rules.ignoreSpace, rules.ignoreDups = true, true
				}
			}
		}
	}
	if style.Ignore != "" {
		if v, ok := get(style.Ignore); ok && v != "" {
			if style.IgnoreIsOnePattern {
				rules.patterns = []string{v}
			} else {
				rules.patterns = strings.Split(v, ":")
			}
		}
	}
	return rules
}

// ignored reports whether this line is one the session was told to leave out.
//
// previous is the newest line already in the list, which is what a duplicate
// is measured against. Empty where there is nothing before it, and the first
// line of a session is therefore never a duplicate — which is right even when
// the file's last line happens to be the same text, because the two are
// different sessions and the earlier one is already written.
func (r historyRules) ignored(line, previous string) bool {
	if r.ignoreSpace && strings.HasPrefix(line, " ") {
		return true
	}
	if r.ignoreDups && previous != "" && line == previous {
		return true
	}
	if r.match == nil {
		return false
	}
	for _, pattern := range r.patterns {
		// The whole line, anchored at both ends: measured, `HISTIGNORE=pwd`
		// drops `pwd` and keeps `pwd x`, so the pattern is not a search for
		// something inside the line. `ls*` is how the other one is written.
		if pattern != "" && r.match(pattern, line) {
			return true
		}
	}
	return false
}
