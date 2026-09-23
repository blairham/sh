// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// What the last `=~` captured.
//
// A successful match is worth more than its status: the whole match and every
// parenthesized group are what a script matched *for*, and reading them back
// is the idiom the operator exists to serve. Every shell that keeps them keeps
// the same values; what differs is the *name* — and the shape — so the core
// keeps the record and a dialect names it, the same seam the pipeline-status
// record uses. One shell calls this array BASH_REMATCH; the others that have
// `=~` keep their captures under names and shapes of their own and never
// touch this one, so with no name from the dialect nothing is recorded.

// SetRegexMatch exposes what `=~` captures under a name, as an ordinary
// stored array: element 0 is the whole match and the rest are the groups, in
// order. A dialect calls it from Apply, the same seam that names the
// pipeline-status record.
//
// Ordinary rather than produced, which is measured: a script can assign to
// the name and read its own value back, and `unset` removes it until the next
// `=~` fills it again — there is no producer to outlive the unset.
func (r *Runner) SetRegexMatch(name string) { r.regexMatchName = name }

// regexFold is the prefix that turns a `=~` expression case-insensitive, and
// the empty string where the option asking for that is off.
//
// A **prefix on the expression** rather than a fold applied at comparison
// time, because a regular expression is not a glob: what has to fold is the
// whole compiled thing — literals, bracket expressions, character classes,
// and the complement of a negated class — and only the engine knows which
// bytes of the text are which. Folding the subject and the pattern before
// compiling would fold `[^a]` the wrong way round and would turn `\.` into a
// pattern about the letter it is not.
//
// The prefix is exactly what the option means and no more: `(?i)` sets the
// engine's initial case flag, so an expression that turns it off again for
// part of itself still does. Measured to agree with bash 5.3.15 on every
// cell that distinguishes the readings, with `shopt -s nocasematch` on:
//
//	[[ ABC =~ ^abc$ ]]              yes — a literal folds
//	[[ abc =~ ^ABC$ ]]              yes — and in the other direction
//	[[ ABC =~ ^[a-c]+$ ]]           yes — a range folds
//	[[ ABC =~ ^[[:lower:]]+$ ]]     yes — so does a character class
//	[[ abc =~ ^[[:upper:]]+$ ]]     yes — and that one too
//	[[ A   =~ ^[^a]$ ]]             no  — the fold precedes the negation
//	[[ ABC =~ ^(a)(B)c$ ]]          yes, and captures ABC, A, B
//
// The last is what says the fold must not touch the captures: they are spans
// of the **subject** as the script wrote it, so `BASH_REMATCH` holds `A` and
// not `a`.
//
// Locale is the other half of what the prefix means, and `(?i)` has no locale
// to be told about: an explicit C or POSIX locale narrows the fold to ASCII in
// the panel — measured 2026-09-13, `LC_ALL=C` makes `[[ ÉTÉ =~ ^été$ ]]` fail
// in bash 5.3.15 and in zsh 5.9.2 where the same line under a UTF-8 locale
// matches — and the engine's flag folds Unicode either way. regexOperands
// below is where the narrowing happens, by taking the characters the engine
// must not fold out of its reach rather than by asking it for a fold it does
// not offer. #2644.
func (r *Runner) regexFold() string {
	if r.MatchOption(RegexFoldsCase) {
		return "(?i)"
	}
	return ""
}

// regexDotAll is the prefix every expression this shell hands to the engine
// carries, and it is what POSIX ERE means rather than a preference.
//
// A shell compiles with `regcomp` and **without** `REG_NEWLINE`, and that is
// the flag that would make a newline special: without it a newline is an
// ordinary character, so `.` and a negated bracket both pass over one, and
// `^` and `$` stay at the ends of the *text*. Go's default is half of that
// pair — the anchors are already at the text boundary and `.` alone excludes
// newline — so the one flag that closes the gap is `(?s)`. `(?m)` is the
// wrong half and would open a second gap in the other direction, since it
// moves the anchors to line boundaries.
//
// Measured 2026-09-20 with `s=$'a\nb'`, in bash 5.3.20, bash 3.2.57, zsh
// 5.9.2 and ksh93u+ alike:
//
//	[[ $s =~ a.b ]]        matches   — a newline is ordinary ground
//	[[ $s =~ ^a.b$ ]]      matches   — and the anchors still reach over it
//	[[ $s =~ ^b ]]         no        — `^` is the start of the text
//	[[ $s =~ a$ ]]         no        — and `$` the end of it
//	[[ $s =~ a[^x]+b ]]    matches   — a negated class already spanned here
//
// The two that answer `no` are the control: they are what says the flag is
// `(?s)` and not `(?m)`, since they agree with the panel today and `(?m)`
// would turn both of them into matches.
//
// It is one constant because there are two compile sites: this operator, and
// ksh93's `~(E)` pattern flavor, which goes to the same engine and answers
// the same three ways — measured the same day, `[[ $s == ~(E)a.b ]]` matches
// there and `[[ $s == ~(E)^b ]]` does not. A second helper that omitted the
// flag would be a divergence nobody looked for. #3892.
const regexDotAll = "(?s)"

// The block of characters the narrowing borrows to stand in for the ones the
// engine must not fold. Plane 15 is private use throughout, so nothing in it
// is a letter, nothing in it has a case, and `(?i)` leaves every one of them
// alone.
const (
	caselessStandInFirst = 0xF0000
	caselessStandInLast  = 0xFFFFD
)

// standIns hands out the characters a narrowed fold writes in place of the
// ones the engine must not fold.
//
// present is every character of the operands that falls in the block, and it
// is why the block is not simply counted through: a private-use character an
// operand actually holds must not be handed out as a stand-in for a different
// character, or the two would match each other. Skipping what is there rather
// than rewriting it as well is what keeps the number of stand-ins bounded by
// the operands' *cased* characters.
type standIns struct {
	of      map[string]rune
	present map[rune]bool
	next    rune
}

func newStandIns(operands ...string) *standIns {
	m := &standIns{of: map[string]rune{}, present: map[rune]bool{}, next: caselessStandInFirst}
	for _, s := range operands {
		for _, c := range s {
			if c >= caselessStandInFirst && c <= caselessStandInLast {
				m.present[c] = true
			}
		}
	}
	return m
}

// pick is the stand-in for one character, the same one every time it is
// asked, so that an expression and a subject rewritten one after the other
// are still about each other.
//
// It reports false when the block has no character left that the operands do
// not already hold, and the caller then leaves the character as it stands —
// the un-narrowed fold, which is where this began. That needs an operand
// holding all 65,534 characters of plane 15, so it is a degradation and not
// an error: there is nothing here for a script to have done wrong.
func (m *standIns) pick(unit string) (rune, bool) {
	if c, ok := m.of[unit]; ok {
		return c, true
	}
	for m.next <= caselessStandInLast {
		c := m.next
		m.next++
		if m.present[c] {
			continue
		}
		m.of[unit] = c
		return c, true
	}
	return 0, false
}

// regexOperands resolves the pair a `=~` evaluation hands to the engine: the
// expression to compile, the subject to match it against, and — where the two
// are not the script's own text — the map from an offset in that subject back
// to an offset in the script's.
//
// Everything below exists for one cell of the fold. With `nocasematch` on and
// an explicit C or POSIX locale, the fold reaches ASCII and no further, so
// `[[ ÉTÉ =~ ^été$ ]]` is a miss and `[[ ABCé =~ ^abcé$ ]]` is still a match —
// the ASCII letters beside the accented one go on folding. That rules out
// both of the cheap answers: dropping the prefix loses the second cell, and
// keeping it loses the first.
//
// It also cannot be done after the parse. `regexp/syntax` folds a bracket
// expression **before** it complements one, which is the behavior
// `[[ A =~ ^[^a]$ ]]` measures and the reason the prefix is a prefix; by the
// time a parsed class is in hand, `[^a]` and `[^é]` are both plain sets of
// ranges and nothing says which runes a fold put there or took away. So the
// narrowing has to be in force *at* the parse, and the only lever the engine
// leaves is which characters the expression is written in.
//
// Hence the stand-ins: every character above ASCII that has a case at all is
// swapped, in the expression and in the subject alike, for a private-use
// character that has none. `(?i)` then folds exactly the ASCII letters, the
// parse sees a bracket expression whose members are caseless and complements
// it correctly, and two characters that were distinct stay distinct. Only
// cased characters are swapped, so a subject of CJK or punctuation is handed
// over untouched and keeps its offsets; and an undecodable byte is untouched
// too, since U+FFFD has no case and every bad byte would otherwise collapse
// onto the same stand-in.
//
// What it does not reach either is *which* characters the engine calls the
// same letter where the fold is wide. `(?i)` folds a full simple-fold orbit,
// and the panel folds with the C library's towlower: measured under
// `en_US.UTF-8`, bash 5.3.15 misses `[[ ſ =~ ^s$ ]]` where this matches,
// because U+017F is in Go's fold orbit for `s` and is not the lower case of
// anything. The glob side folds by the lower-case map for that reason — see
// eqRuneFolded — and the operator that hands its fold to an engine cannot.
// That is the engine's reading rather than the locale's, so it is #2643's
// question and not this one's.
//
// What this does **not** change is how much of the subject a character is.
// A stand-in is one character where the character it stands in for was one,
// so `.` passes over the same ground either way — which is the byte-or-
// character question the glob side answers with patternOpts.chars and this
// operator has never asked. Leaving it unasked is deliberate: it would make
// the operator read the subject by one measure with the fold on and another
// with it off, and it is a wider question than the fold. It is also visible.
// Measured under `LC_ALL=C`, bash 5.3.15 misses `[[ ÉTÉ =~ ^...$ ]]` and
// `[[ É =~ ^[^é]$ ]]` because `É` is two bytes there and neither `.` nor a
// bracket matches either of them, where this counts one character and answers
// the second the other way. Both are that axis rather than this one — under a
// UTF-8 locale, where the fold is the only thing in play, the bracket agrees.
//
// The expression comes back under regexDotAll whichever way the fold went,
// because that flag is not an option a script turned on: it is what a POSIX
// ERE is, and the fold's prefix composes with it rather than replacing it.
func (r *Runner) regexOperands(pat, subject string) (expr, subj string, back []int) {
	fold := r.regexFold()
	if fold == "" || r.caseFoldReachesBeyondASCII(pat, subject) {
		return regexDotAll + fold + pat, subject, nil
	}
	stand := newStandIns(pat, subject)
	subj, back = standInFor(subject, stand)
	expr, _ = standInFor(pat, stand)
	return regexDotAll + fold + expr, subj, back
}

// standInFor rewrites the cased characters above ASCII of s into private-use
// characters that have no case, giving the same stand-in to the same
// character every time it is asked — which is what lets an expression and a
// subject be rewritten one after the other and still be about each other.
//
// back is the offset of each byte of the result in s, with the length on the
// end so that the end of a match maps as well as its start. It is nil when
// nothing was rewritten, which is the answer for every subject that holds no
// cased character above ASCII.
func standInFor(s string, stand *standIns) (string, []int) {
	if isASCII(s) {
		return s, nil
	}
	var b strings.Builder
	b.Grow(len(s))
	back := make([]int, 0, len(s)+1)
	rewrote := false
	for i := 0; i < len(s); {
		w := characterWidth(s[i:], "")
		unit := s[i : i+w]
		out := unit
		// A character with no case of its own is written through, and that
		// is where an undecodable byte lands as well as CJK and punctuation:
		// it decodes to U+FFFD, which folds to itself, so it keeps its bytes
		// and its offsets and every bad byte stays distinct from every other.
		if c, _ := utf8.DecodeRuneInString(unit); c >= utf8.RuneSelf && unicode.SimpleFold(c) != c {
			if in, ok := stand.pick(unit); ok {
				out, rewrote = string(in), true
			}
		}
		n := b.Len()
		b.WriteString(out)
		for j := n; j < b.Len(); j++ {
			back = append(back, i)
		}
		i += w
	}
	if !rewrote {
		return s, nil
	}
	back = append(back, len(s))
	return b.String(), back
}

// scriptOffsets maps what the engine reported about a rewritten subject onto
// the script's own text. A group that did not participate is -1 and stays
// there.
func scriptOffsets(loc, back []int) []int {
	if loc == nil || back == nil {
		return loc
	}
	out := make([]int, len(loc))
	for i, off := range loc {
		if off < 0 {
			out[i] = off
			continue
		}
		out[i] = back[off]
	}
	return out
}

// recordRegexMatch stores what a `=~` evaluation captured.
//
// It is the *evaluation* that records, before `!` or `&&` see the result — a
// negated match still fills the record, exactly as the pipeline-status record
// is taken before `!` inverts anything.
//
// took says, per element, whether that group took part in the match; element
// 0 is the whole match and took part whenever there was one. It is a slice of
// its own because the texts cannot carry the distinction: a group that
// matched the empty string and a group the match never reached are both the
// empty string, and one column leaves the second out of the record entirely.
//
// Two axes and both are one column's, asked only where a dialect named the
// record at all — see Semantics.RegexMatchSurvivesAFailedMatch and
// RegexMatchOmitsGroupsThatDidNotMatch.
func (r *Runner) recordRegexMatch(m []string, took []bool) {
	if r.regexMatchName == "" {
		// No dialect named the record, so there is nothing to keep and
		// nobody to ask.
		return
	}
	if m == nil {
		// A match that failed. One column stores an empty array, so a script
		// that forgets to check the status reads nothing rather than the
		// match before last; the other leaves the record where it was.
		if r.ask(r.sem().RegexMatchSurvivesAFailedMatch, "a failed `=~` leaving the record of the last match alone") ||
			r.unspecified {
			return
		}
		r.setArray(r.regexMatchName, nil)
		return
	}
	if r.ask(r.sem().RegexMatchOmitsGroupsThatDidNotMatch, "a group that took no part being left out of the `=~` record") {
		kept := make([]string, 0, len(m))
		for i := range m {
			if took[i] {
				kept = append(kept, m[i])
			}
		}
		m = kept
	}
	if r.unspecified {
		return
	}
	r.setArray(r.regexMatchName, m)
}

// SetRegexCaptureReport makes a `=~` evaluation publish what it matched
// through the same parameters a *reporting pattern* fills — the whole match,
// the groups, and the character positions of each. A dialect calls it from
// Apply, beside SetRegexMatch.
//
// Two shapes rather than one, because the shells that keep captures disagree
// about more than the name. One keeps a single dense array whose element 0 is
// the whole match; the other splits the whole match from the groups and
// reports where each began and ended, which is the same record a pattern flag
// already writes — so the second shape needs no parameters of its own, only
// permission to use the ones patterncapture.go already knows.
//
// It is a switch and not a name for that reason: the names are the reporting
// pattern's, and a dialect that has one has the other.
func (r *Runner) SetRegexCaptureReport() { r.regexCaptureReport = true }

// publishRegexCapture writes a successful `=~` into the reporting parameters.
//
// Only a successful one, which is measured: a match that fails leaves the
// parameters holding what the match before it put there, exactly as a
// reporting *pattern* that fails does. That is the opposite of the dense
// record beside it, which is emptied on every evaluation — and both are the
// shell they belong to, which is why they are two calls rather than one.
//
// loc is the byte offsets `regexp` reports: a pair per group, the first pair
// being the whole match, and -1 for a group that did not participate.
func (r *Runner) publishRegexCapture(subject string, loc []int) {
	if !r.regexCaptureReport || loc == nil {
		return
	}
	report := matchReport{
		subject:  subject,
		whole:    capSpan{begin: loc[0], end: loc[1], set: true},
		wantsAll: true,
	}
	for i := 2; i+1 < len(loc); i += 2 {
		var span capSpan
		if loc[i] >= 0 {
			span = capSpan{begin: loc[i], end: loc[i+1], set: true}
		}
		report.groups = append(report.groups, span)
	}
	r.publishMatch(report)
}
