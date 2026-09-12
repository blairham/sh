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
// Which variable or option says so is the dialect's, in HistoryStyle. What
// they mean is here, because the meanings are shared: measured on 2026-09-05
// and re-measured on 2026-09-06, bash 5.3.15 and zsh 5.9.2 agree that a
// leading space hides a line, that a "duplicate" is the line immediately
// before it and not any earlier one, and that a pattern is matched against the
// whole line. They disagree about one thing only — whether a line the
// *pattern* knob rejected is still recallable — and that is the axis.
//
// Not whether an ignored line is recallable, which is what this said before
// and is a knob narrower than it sounds. zsh answers that question both ways
// in one shell: a line `HISTORY_IGNORE` rejected is still in `fc -l`, and a
// line `HIST_IGNORE_SPACE` or `HIST_IGNORE_DUPS` rejected is not. So the space
// and dups rules have no axis — every shell in the panel that has them at all
// forgets the line outright — and keepIgnored below applies to patterns only.

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

	// keepIgnored leaves a line one of the patterns rejected in the list the
	// up arrow walks and takes it out of the file only. Patterns only: see
	// HistoryStyle.PatternIgnoredStaysInSession for the measurement that says
	// so, and for why the other two rules do not get the same choice.
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
// option reads one of this dialect's option names, and a session with no
// shell to ask has no options — which is a session where every option-spelled
// rule is off rather than one that guesses.
func historyRulesFrom(
	style HistoryStyle,
	get func(string) (string, bool),
	option func(string) (on, known bool),
	match func(pattern, line string) bool,
) historyRules {
	rules := historyRules{keepIgnored: style.PatternIgnoredStaysInSession, match: match}
	// The options first and the variable after, so that a dialect which
	// somehow had both would let the variable it documents win the last word.
	// No shell in the panel has both; the order is written down rather than
	// left to whichever branch happens to run second.
	if option != nil {
		// The empty name is not asked about. A dialect with no such option
		// leaves the field blank, and a namespace handed "" answers about
		// whatever it makes of it — so the name is checked here rather than
		// trusted to come back unknown.
		//
		// A name the namespace does not have leaves the rule off. That is the
		// difference DialectOption's second result exists for: a dialect
		// naming an option this build does not carry must not read as "on".
		rules.ignoreSpace = optionOn(option, style.IgnoreSpaceOption)
		rules.ignoreDups = optionOn(option, style.IgnoreDupsOption)
	}
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
// The second result is whether the line may still be recalled: false for
// every rule but the patterns, and for those only where the dialect said so.
// Meaningless when the first result is false, and false there so that a caller
// reading it alone cannot mistake "not ignored" for "kept out of the file".
// hidesRecord reports that the person asked for this line not to be written
// down anywhere.
//
// The space rule alone, and the distinction it draws is the point of this
// existing at all. A leading space is a *privacy* gesture — the comment on
// ignoreSpace calls it "run this but do not write it down" — and it is the
// same sentence an empty HISTFILE says about a whole session, scoped to one
// line. The other two rules are *tidiness*: ignoreDups and the patterns keep
// the list the up arrow walks worth walking, which is a statement about a
// recall list and not about what may be kept.
//
// So this is what a recorder beside the history file asks, and `ignored`
// above is what the file itself asks. They share this rather than each
// spelling it out, because a second copy of "does a leading space hide this"
// is a second place for the answer to drift — and the copy that drifts is the
// one that writes a line somebody asked not to be written.
//
// A block store is the recorder this was added for: it kept the line, and its
// output, for a command the session had already agreed to forget (#2273).
func (r historyRules) hidesRecord(line string) bool {
	return r.ignoreSpace && strings.HasPrefix(line, " ")
}

func (r historyRules) ignored(line, previous string) (ignored, recallable bool) {
	// A blank hides a line and a repeat hides a line, and in every shell that
	// has either rule the line is gone from the list too. So these two return
	// false for recallable regardless of what the dialect said about patterns.
	if r.hidesRecord(line) {
		return true, false
	}
	if r.ignoreDups && previous != "" && line == previous {
		return true, false
	}
	if r.match == nil {
		return false, false
	}
	for _, pattern := range r.patterns {
		// The whole line, anchored at both ends: measured, `HISTIGNORE=pwd`
		// drops `pwd` and keeps `pwd x`, so the pattern is not a search for
		// something inside the line. `ls*` is how the other one is written.
		//
		// An empty pattern needs no guard: it matches only the empty string,
		// and a blank line is never recorded by anything.
		if r.match(pattern, line) {
			return true, r.keepIgnored
		}
	}
	return false, false
}

// optionOn reports whether a named option is on, for a name a dialect may have
// left blank and a namespace may never have heard of.
func optionOn(option func(string) (on, known bool), name string) bool {
	if name == "" {
		return false
	}
	on, known := option(name)
	return known && on
}
