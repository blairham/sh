// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// How much subject a pattern can possibly consume, which is what makes a
// substitution stop asking questions whose answer is already no.
//
// `${v//pat/X}` looks for the *longest* match at each position, and the way
// it finds one is to try every span from the whole of the rest of the
// subject downwards. That is quadratic in the subject's length, and the
// constant is a whole match attempt — so a 1750-byte subject is three
// million attempts before a byte is replaced.
//
// Almost all of them are impossible on their face. A pattern of four
// ordinary characters consumes exactly four bytes, so of the 1750 spans
// tried at each position, 1749 could not have matched whatever the subject
// held. Measured on this machine's own prompt theme, two such patterns —
// `" %{\b"` and `" \b"`, four calls each against a 1750-byte prompt — were
// **6.27 of the 6.33 seconds** the theme spent in substitution, at some 790
// milliseconds a call.
//
// # The bound only ever skips what cannot match
//
// This is not an optimisation the matcher can get wrong quietly: a bound
// that is too *wide* costs the attempts it failed to skip and changes no
// answer, and a bound that is too *narrow* would skip a span that could
// have matched and silently lose a replacement. So the analysis refuses
// everything it is not certain of. Every byte that any mode of this matcher
// could read as something other than one literal byte ends the analysis and
// the caller goes back to trying every span.
//
// That leaves it narrow on purpose. `?` and `[a-z]` consume exactly one
// *unit*, which is one to four bytes, and either could be admitted with a
// little more care; neither is here, because neither appeared in the
// measurement and an untested widening of this bound is the one change in
// this file that could lose a match. What is admitted is the case that was
// actually costing the time: a run of ordinary bytes, and an escape in
// front of one.

// spanStoppers is every byte this analysis refuses to reason past, in any
// dialect and under any option.
//
// A superset of both [patternMeta] and [extendedPatternMeta] on purpose, and
// the reason is the asymmetry above: reading one byte as literal when some
// mode would not is the error that loses a match, so anything arguable is in
// here. `]`, `)` and `|` are, though they mean nothing outside a construct
// this list has already stopped at; `>` and `@` and `+` and `!` are, though
// no path in this matcher reads them at all.
//
// `{`, `}` and `%` are deliberately *not*: braces are a phase that has
// finished before a pattern reaches here, and `%` is an ordinary character
// to every part of this matcher. Those are the two facts that let a prompt
// theme's `" %{\b"` be recognized as the four plain bytes it is.
const spanStoppers = `*?[]()|<>~^#@+!`

// spanBoundDisabled turns the analysis off, so that a test can run the same
// substitution both ways and compare. Nothing outside a test writes it, and
// the comparison it enables is the only reference for "the bound changed no
// answer" that cannot drift away from the code it is checking.
var spanBoundDisabled bool

// patternSpanBytes is the fewest and the most bytes of subject a pattern can
// consume, and whether the most is known at all.
//
// Bytes rather than units, because the caller has byte offsets and because
// the matcher's own literal path compares one pattern byte to one subject
// byte — a multi-byte character in a pattern is matched as its bytes, so the
// count is exact rather than approximate.
func patternSpanBytes(p string, o patternOpts) (lo, hi int, bounded bool) {
	if spanBoundDisabled {
		return 0, 0, false
	}
	for i := 0; i < len(p); {
		c := p[i]
		if c == '\\' {
			if i+1 >= len(p) || !o.escapeReaches(p[i+1]) {
				// A trailing backslash matches a backslash, and one the
				// dialect does not let reach the next character is a
				// character of its own with that character still to come.
				// Both are answerable and neither is measured, so both stop
				// the analysis rather than adding a case nothing exercises.
				return 0, 0, false
			}
			// An escaped metacharacter is an ordinary character: two bytes
			// of pattern for one byte of subject.
			lo, hi, i = lo+1, hi+1, i+2
			continue
		}
		if strings.IndexByte(spanStoppers, c) >= 0 {
			return 0, 0, false
		}
		lo, hi, i = lo+1, hi+1, i+1
	}
	return lo, hi, true
}

// spanCouldMatch reports whether a span of n bytes is one the pattern could
// fill, given bounds patternSpanBytes worked out.
func spanCouldMatch(n, lo, hi int, bounded bool) bool {
	if !bounded {
		return true
	}
	return n >= lo && n <= hi
}
