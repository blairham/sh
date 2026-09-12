// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// Whether a pattern component may match a name that begins with a period.
//
// The rule every shell states is that it may only if the pattern *explicitly*
// begins with one, and the whole of the question is what "begins with" means
// for a pattern that starts with a group. Measured on zsh 5.9.2 in a directory
// holding `.hidden` and `plain`, with `extendedglob` on:
//
//	.hidden            .hidden          the plain case
//	\.hidden           .hidden          an escaped period is still a period
//	.(hidden|x)        .hidden
//	(.hidden|plain)    .hidden plain    an alternative begins with one
//	(x|.hidden)        .hidden          and it need not be the first
//	(.|x)hidden        .hidden          nor the whole of it
//	(.hidden)          .hidden          a group of one
//	((.hidden))        .hidden          and one nested in another
//	(.h*)              .hidden
//	(|.hidden)         .hidden          past an empty alternative
//	(#i)(.HIDDEN|x)    .hidden          and past a pattern-flag group
//	(.hidden|plain)*   .hidden plain
//	[.]hidden          nothing          a bracket is not explicit
//	?hidden            nothing          nor is a metacharacter
//	*                  plain            which is the rule's whole point
//
// So the period has to be a *literal* one the pattern could match first, and
// the places a pattern can start are: the front of it, the front of every
// alternative of a group standing there, and whatever follows a `(#…)` flag
// group, which matches nothing itself.
//
// The bracket row is the one that says this is not "could this pattern match
// a leading period" — `[.]hidden` plainly could — but "is one written there".
//
// This is load-bearing well beyond dotfile listings. powerlevel10k builds one
// alternation out of every anchor file it knows — `.git`, `.tool-versions`,
// `go.mod`, `package.json` and the rest — and asks
// `[[ -n $dir/${~MARKER}(#qN) ]]` of each component of the working directory
// to decide which components are anchors. A reading that looks only at the
// pattern's first byte finds `(` there, refuses every dotfile in the
// alternation, and answers "no anchor" for every directory whose marker is a
// dotfile — which is most of them (#2119).

// patternBeginsWithPeriod reports whether a component pattern explicitly
// begins with a period.
//
// group says a parenthesized group is a group in this dialect rather than
// literal parentheses; where it is not, only the front of the pattern is a
// place a pattern can start, which is the reading this had before groups were
// looked into.
func patternBeginsWithPeriod(pattern string, group bool) bool {
	if !group {
		return strings.HasPrefix(globUnescape(pattern), ".")
	}
	return periodStartsAt(pattern, 0, 0)
}

// periodStartsAt is that question asked at one position, following groups
// down.
//
// depth bounds the recursion rather than the nesting: a pattern is a string a
// script wrote, and a thousand nested parentheses is not a shell behavior to
// model but a stack to run out of.
func periodStartsAt(pattern string, i, depth int) bool {
	if i >= len(pattern) || depth > 32 {
		return false
	}
	if escapedAt(pattern, i) {
		// `\.` is a period the pattern wrote, which is measured: the escape
		// says the character is literal and says nothing about where it is.
		return pattern[i] == '.'
	}
	if pattern[i] != '(' {
		return pattern[i] == '.'
	}
	end, ok := groupEndsAt(pattern, i)
	if !ok {
		return false
	}
	if i+1 < end && pattern[i+1] == '#' && !escapedAt(pattern, i+1) {
		// A pattern-flag group draws nothing, so the start of the pattern is
		// whatever stands after it.
		return periodStartsAt(pattern, end+1, depth+1)
	}
	for _, alt := range groupAlternatives(pattern, i+1, end) {
		if periodStartsAt(pattern, alt, depth+1) {
			return true
		}
	}
	return false
}

// groupEndsAt is the index of the parenthesis closing the group that opens at
// i, or false where nothing closes it — an unbalanced group is not a group,
// and the matcher will read those parentheses as text.
func groupEndsAt(pattern string, i int) (int, bool) {
	depth := 0
	for j := i; j < len(pattern); j++ {
		if escapedAt(pattern, j) {
			continue
		}
		switch pattern[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return j, true
			}
		}
	}
	return 0, false
}

// groupAlternatives lists where each alternative of a group begins: just
// inside it, and after every bar standing at its own top level.
func groupAlternatives(pattern string, start, end int) []int {
	out := []int{start}
	depth := 0
	for j := start; j < end; j++ {
		if escapedAt(pattern, j) {
			continue
		}
		switch pattern[j] {
		case '(':
			depth++
		case ')':
			depth--
		case '|':
			if depth == 0 {
				out = append(out, j+1)
			}
		}
	}
	return out
}
