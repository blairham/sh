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

// A trim's half of the same bound: the characters a match has to end with.
//
// `${v##pat}` is the other quadratic loop in this file's family.
// [spanByLength] asks the matcher about every prefix of the subject, longest
// first, and takes the first that the whole pattern matches — so a subject of
// n units is n match attempts, each of which may read the whole prefix. Under
// the searching flag it asks about every *span* rather than every prefix, so
// the same analysis is what keeps that from being quadratic as well.
//
// Measured on this machine's powerlevel10k instant-prompt cache, which is
// where it was found: `${content##*$rs$key$us}`, one expansion against a
// 13,574-byte subject, is **2139ms here and 0.125ms in zsh 5.9.2** — some
// seventeen thousand times. That single expansion is most of a 12-second
// startup, because the instant prompt runs it before the shell draws
// anything (#2124).
//
// The pattern ends in 52 ordinary characters. Every one of those 13.5
// thousand prefixes that does not end in exactly those bytes is a question
// whose answer is already no, and there is one that does.

// bypassOperators are the operators that let a match end somewhere other
// than where the pattern's last literal characters say it does — which is
// what would make an edge literal unsound rather than merely narrow.
//
// `|` is an alternation, so a match of `foo|bar` ends in `foo` as readily as
// in `bar`. `^` and `~` are the two negations, and a match of `^abc` is one
// that deliberately is not those bytes. A `(#…)` flag group is the fourth
// and is refused by its opening two characters: `(#i)ABC` matches `abc`,
// whose bytes are not the pattern's.
//
// Any of them anywhere in the pattern ends the analysis. Working out how far
// each one reaches is exactly the care this file refuses to take, and none
// of them appeared in what was measured.
const bypassOperators = "|^~"

// patternEdgeLiterals is the run of ordinary characters a pattern begins and
// ends with: bytes that every piece the *whole* pattern matches must begin
// and end with, whatever the pattern did between them.
//
// Both are returned from one scan because a trim wants them both — a piece
// is matched from end to end, so it has to satisfy each — and either may be
// empty, which asks nothing of the subject. The same asymmetry as the bound
// above governs it: a literal that is too *short* costs the attempts it
// failed to skip and changes no answer, and one that claimed bytes the
// pattern does not require would skip a piece that could have matched. So
// every byte that any mode of this matcher could read as something other
// than one literal byte resets the run, and the four operators that can
// bypass a run outright refuse the question.
func patternEdgeLiterals(p string, o patternOpts) (head, tail string, ok bool) {
	if spanBoundDisabled || o.fold || o.litFold != caseExact {
		return "", "", false
	}
	if strings.ContainsAny(p, bypassOperators) || strings.Contains(p, "(#") {
		return "", "", false
	}
	// lit is the run in hand: the head while nothing has stopped it yet, and
	// from the last stopper onwards the tail. Written out rather than sliced
	// off the pattern because an escape is two bytes of pattern for one of
	// subject, so the run is not always a substring of what it came from.
	var lit []byte
	headDone := false
	for i := 0; i < len(p); {
		c := p[i]
		if c == '\\' {
			if i+1 >= len(p) || !o.escapeReaches(p[i+1]) {
				// Answerable, unmeasured, and the same call the bound above
				// makes: both spellings stop the analysis rather than adding
				// a case nothing exercises.
				return "", "", false
			}
			lit = append(lit, p[i+1])
			i += 2
			continue
		}
		if strings.IndexByte(spanStoppers, c) >= 0 {
			if c == '#' && o.extended {
				// The one operator that reaches *backwards*. `a#` is zero or
				// more `a`, so the run in hand is not required after all —
				// found by the property test below, which matched `a#`
				// against the empty string while the run still claimed `a`.
				//
				// The whole run goes rather than its last character, because
				// the quantifier takes a unit and a unit is one to four bytes:
				// dropping one byte of `é#` would leave half a character
				// standing as a requirement. Giving up the run costs the
				// attempts it would have skipped and can lose no match.
				lit = lit[:0]
			}
			if !headDone {
				head, headDone = string(lit), true
			}
			lit = lit[:0]
			i++
			continue
		}
		lit = append(lit, c)
		i++
	}
	if !headDone {
		// Nothing stopped the scan, so the pattern is one literal run and it
		// is both edges at once. `abc` requires a piece to start with `abc`
		// and to end with it, which two separate checks on one three-byte
		// piece both answer yes to.
		head = string(lit)
	}
	return head, string(lit), true
}

// edgeLiteralsFit reports whether a piece could be one the pattern matches
// whole, judged only by the characters each end of the pattern requires.
//
// Necessary and never sufficient: a piece that fits is still asked of the
// matcher, and only a piece that cannot fit is skipped.
func edgeLiteralsFit(piece, head, tail string, known bool) bool {
	if !known {
		return true
	}
	return strings.HasPrefix(piece, head) && strings.HasSuffix(piece, tail)
}
