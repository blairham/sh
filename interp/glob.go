// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"slices"
	"strings"
)

// globEscape marks a metacharacter as literal while a field is carried around.
//
// A field has to remember which of its metacharacters were quoted, because
// quoting is what decides whether text is a pattern at all — `$p` globs and
// `"$p"` does not. The fields expansion produces are therefore in this escaped
// form and are unescaped once globbing has had its look.
// `<` is in the set for the dialect that has numeric ranges, and `(` and `)`
// for the one that has pattern groups and glob qualifiers. Marking them where
// nothing reads one costs nothing: an escaped ordinary character is that
// character, so `\<` and `<` match the same text everywhere else.
//
// The parentheses are measured rather than added for symmetry. A `(` that
// arrives from a *value* is never a group in the shell that has them:
// `p="f(1|2)"; echo $p` prints `f(1|2)` and `p="*(.)"; echo $p` prints
// `*(.)` without globbing at all, where the same text written literally is
// an alternation and a qualifier list. Quoting says the same from the other
// side — `echo "( x )"` is five characters.
//
// **So is the `|`, and the discriminating case needs a group the value did
// not bring.** The rows above cannot see it: with the `(` already marked
// there is no alternation for a `|` to divide, so a value carrying both
// answers the same either way. The case that tells them apart is a group
// written *literally* around an expansion — which nothing could reach until
// an expansion inside a group was read as one at all (#1331). Measured on
// zsh 5.9.2, 2026-09-08, in a directory holding `ice.zsh`, `other.zsh` and
// one file literally named `ice|x.zsh`:
//
//	L="ice|other"; print -r -- ($L).zsh   no matches found: (ice|other).zsh
//	L="ice|x";     print -r -- ($L).zsh   ice|x.zsh
//	L="ice|other"; print -r -- (${~L}).zsh   ice.zsh other.zsh
//
// The middle row is the one that says it: the `|` is a character the name
// has to contain. The third is the same value with the flag that asks for
// the other reading, which is where an alternation from a value does come
// from.
// valueBackslashMark stands where a backslash that arrived in a **value**
// stood, under the reading that has it quote the character behind it and not
// be matched itself.
//
// It is a byte of its own because the escaped form has one meaning per byte
// and this needs two at once: for the *match* the backslash is a quote and
// contributes nothing, and for the text a failed match restores it is a
// backslash. A marked backslash followed by a marked character cannot say
// that -- it is a literal backslash in front of a quoted one, which is a
// different reading that another column holds -- so a third symbol is the
// whole of the difference (#1370).
//
// Always written immediately in front of an ordinary escape, so a reader
// that has not learned it sees a stray character and a *correctly quoted*
// one behind it. That failure is a pattern matching nothing and a word
// restored by globUnescape, which is far cheaper than the other
// arrangement's: a metacharacter quietly going live.
//
// NUL is available because globEscape marks one, so a NUL that was *data*
// carries a mark and a bare one can only be this. A shell value is not
// supposed to hold one at all, and this implementation lets one through in
// places the panel does not -- "not supposed to" is not a guarantee to build
// an alphabet on.
const valueBackslashMark = '\x00'

// valueBackslashRanOutOfValue is valueBackslashMark written twice, and stands
// where a backslash that arrived in a **value** stood with nothing behind it
// *in that value*.
//
// A value's end is not the field's end. `bs='\'; echo ./tmp${bs}/a/b/*` is one
// word whose second span is `/a/b/` and whose third is a live `*`, and the
// shells that have the backslash quote what follows it quote the **field's**
// next character rather than the value's — so this cannot be written as the
// ordinary mark, which names the character it quotes, and it cannot be written
// as a plain marked backslash either, which is what it was and is why
// `./tmp\/a/b/*` never found `./tmp/a/b/c` (#4234).
//
// Doubled rather than given a byte of its own, and that is a measurement rather
// than taste. `\x01` was the first attempt and it is a byte a value legitimately
// holds: `$'a\001b'` is in the tree's own minimal-quoting round trip, and with
// the mark on that byte the value came back as `a\b`. NUL is the only byte the
// escaped form can spend, for the reason valueBackslashMark gives — and it is
// already spent, so a *second* one of it is free: a NUL that is data is always
// written with a mark in front of it, so a bare mark can only be one this file
// wrote, and two bare marks in a row can only be this. Spelled out rather than
// built from the constant because a Go constant cannot be, and the two are held
// together by TestTheTwoValueBackslashMarksAgree.
const valueBackslashRanOutOfValue = "\x00\x00"

// markedByGlobEscape is the alphabet above, named because two readers need
// it: globEscape, which puts the marks on, and the value-backslash escaping,
// which asks whether a mark on one of these could change what a field means.
// A second spelling of the set is how the two would come apart.
//
// NUL is in it for valueBackslashMark's sake and for nothing else: marking a
// byte no pattern reads costs nothing, and it is what makes a bare mark
// unambiguous — and a doubled one, which is the other mark. See
// valueBackslashRanOutOfValue.
// A `/` is in it for one reader and means something weaker than the others do.
// Every other byte in this set is marked to say "this was quoted, so it is a
// character rather than an operator"; a `/` cannot be made a character, because
// it separates the components a pattern is matched a piece at a time against
// however it was written. So a mark on one records only that it **was quoted**
// and commits to nothing — the shape valueBackslashMark already uses — and
// splitFieldParts drops it while splitting on it like any other separator.
//
// The one reader is bracketHoldsASlash, and through it
// Semantics.BracketHoldingASlashIsStillABracket: the column that reads a
// bracket written across a separator as *not a bracket* reads one written
// across a quoted separator as a bracket, so the two spellings have to arrive
// here distinguishable. Before this they did not: `[qwe/]` and `[qwe\/]` were
// the same field, and an axis answered from that field alone moved two of
// glob.tests' lines to agreeing and two the other way (#4158).
const markedByGlobEscape = "*?[\\<()|/\x00" + extendedPatternMeta

func globEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(markedByGlobEscape, s[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// escapeValueBackslashes puts an expansion result into the escaped form
// without touching its live metacharacters.
//
// The escaped form spells "this character was quoted" as a backslash in front
// of it, so a backslash is the one byte a value cannot carry unmarked: one
// that was *in the value* is otherwise read as the mark for whatever follows
// it and removed with the marks, which is how `v='a\\b'; w=$v` assigned `ab`
// and a doubled one assigned a single backslash where every shell in the
// panel keeps both (#1222). Only globEscape's caller knew to mark them, and
// it is the caller that runs when the result is *not* a pattern -- so the
// loss was exactly on the path where the value stays live.
//
// What such a backslash does to the character behind it is three readings and
// not one, and none of them is decided here: the answer depends on whether
// the *field* is globbed, and a field is a word rather than one expansion --
// `v='a\\b'; echo $v*` puts a live star next to this result from a span that
// is not this one. So the backslash is written as valueBackslashMark, which
// records that a value put one there and commits to nothing, and
// resolveValueBackslashes reads it once the whole field exists. The mark is
// what globUnescape restores from, so the *text* is right under every
// reading whether the field is ever globbed or not.
//
// A backslash at the end of a value has nothing behind it and is an ordinary
// marked backslash: all three readings agree about it.
func escapeValueBackslashes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == valueBackslashMark {
			// A NUL that is *data*, marked so that a bare mark can only be one
			// this function wrote — and a bare pair only the other mark. See
			// valueBackslashMark.
			b.WriteByte('\\')
			b.WriteByte(s[i])
			continue
		}
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) {
			// The value ran out, and the character this backslash quotes is
			// the next one in the *field*. Marked rather than written as a
			// literal backslash, which is what it was: see
			// valueBackslashRanOutOfValue.
			b.WriteString(valueBackslashRanOutOfValue)
			continue
		}
		b.WriteByte(valueBackslashMark)
		i++
		b.WriteByte('\\')
		b.WriteByte(s[i])
	}
	return b.String()
}

// rewriteValueBackslashes replaces every valueBackslashMark with the reading
// p gives it, leaving a field the matcher can read.
//
// The mark always stands in front of a marked character, and that character
// is re-marked here rather than carried over: a mark is only *needed* on a
// metacharacter, and one on an ordinary character is not free. A marked
// letter is a backslash the matcher may not read as an escape at all --
// Semantics.PatternEscapeReaches decides which characters an escape reaches,
// and where it does not reach a letter the two bytes are a backslash and a
// letter, which matches nothing. globEscape is the one place that knows
// which characters need a mark, so it is the one that puts them on.
//
// A backslash behind the mark is the doubled case and is marked under every
// reading, because a lone one would escape whatever came after it.
func (r *Runner) rewriteValueBackslashes(field string, p ValueBackslashPolicy) string {
	if !holdsAValueBackslash(field) {
		return field
	}
	var b strings.Builder
	for i := 0; i < len(field); i++ {
		if strings.HasPrefix(field[i:], valueBackslashRanOutOfValue) {
			i++
			// The value ran out here, so what this backslash quotes is the
			// next unit of the *field* — and that unit's escaping is the
			// script's rather than the value's, which is what keeps a
			// metacharacter the script quoted quoted under every reading.
			unit, live, width := fieldUnitAt(field, i+1)
			i += width
			switch {
			case width == 0:
				// Nothing follows it in the field either, and there all three
				// readings agree: it is a backslash.
				b.WriteString(`\\`)
			case p == ValueBackslashQuotesWhatFollows && live && unit == "/" &&
				r.valueBackslashSurvivesAPatternPiece(componentSoFar(b.String())):
				// The one character a quote cannot take the meaning off: a
				// `/` still separates however it was quoted, so the backslash
				// stays where it is — at the end of the piece in front of it —
				// and that piece decides what becomes of it. A piece that
				// *describes* a name keeps it as a character and matches a
				// name with a backslash on the end; one that spells a name has
				// it removed with the rest of its quoting, which is the branch
				// below. See Semantics.ValueBackslashSurvivesAPatternPiece.
				b.WriteString(`\\`)
				b.WriteString(unit)
			case p == ValueBackslashQuotesWhatFollows && !live:
				// Nothing for it to quote. The script had already quoted what
				// follows, so a second quoting takes nothing off anything and
				// the backslash is a character of the pattern like any other.
				//
				// Unanimous, and this shell was alone: `v='a\'; printf '[%s]'
				// $v\\*b` in a directory holding `a*b`, `a\*b` and `a\\*b` is
				// `[a\\*b]` in dash, bash 5.3, bash 3.2, the same bash as `sh`,
				// ksh93 and zsh, and was `[a\*b] [a\\*b]` here — two fields where
				// every column produces one, from a backslash this branch spent
				// on a character that had nothing live about it (#4158).
				b.WriteString(`\\`)
				b.WriteString(globEscape(unit))
			case p == ValueBackslashQuotesWhatFollows:
				b.WriteString(globEscape(unit))
			case p == ValueBackslashIsData && live:
				b.WriteString(`\\`)
				b.WriteString(unit)
			default:
				b.WriteString(`\\`)
				b.WriteString(globEscape(unit))
			}
			continue
		}
		if field[i] != valueBackslashMark {
			b.WriteByte(field[i])
			if field[i] == '\\' && i+1 < len(field) {
				i++
				b.WriteByte(field[i])
			}
			continue
		}
		// The mark, and the marked character it stands in front of.
		quoted, has := byte(0), false
		if i+2 < len(field) {
			quoted, has = field[i+2], true
			i += 2
		}
		switch p {
		case ValueBackslashQuotesWhatFollows:
			// The backslash quotes and is gone; only what follows is left,
			// marked if it needs to be.
		case ValueBackslashIsData:
			// The backslash is a character and what follows it stays live.
			b.WriteString(`\\`)
			if has && quoted != '\\' {
				b.WriteByte(quoted)
				continue
			}
		default:
			// The backslash is a character and what follows it is not live,
			// which is the same field the text written literally produces.
			b.WriteString(`\\`)
		}
		if has {
			b.WriteString(globEscape(string(quoted)))
		}
	}
	return b.String()
}

// resolveValueBackslashes turns the marks a field carries into the reading
// this dialect has, and is where Semantics.ValueBackslashInAPattern is asked.
//
// Here rather than where the escaping happened, because here is the first
// point the *whole field* exists: a value is one span of a word and the
// metacharacter that makes the field a pattern may come from another --
// `v='a\\b'; echo $v*` is `ab` in bash and `a\\bc` in ksh93, and the value
// alone has nothing live in it at all.
//
// Asked only where the readings put different patterns on the wire. A field
// none of them makes a pattern is one nothing will glob, and the word it
// restores is globUnescape of the *unresolved* field, which is the same text
// under all three -- so the common shape, a value carrying a backslash in an
// ordinary word, demands no dialect.
//
// The second result is false only for an unanswered axis, which has already
// been reported: the field is not globbed and the caller restores it.
func (r *Runner) resolveValueBackslashes(field string) (string, bool) {
	if !holdsAValueBackslash(field) {
		return field, true
	}
	quotes := r.rewriteValueBackslashes(field, ValueBackslashQuotesWhatFollows)
	disarms := r.rewriteValueBackslashes(field, ValueBackslashDisarmsWhatFollows)
	data := r.rewriteValueBackslashes(field, ValueBackslashIsData)
	if !r.resultReadsAsPattern(quotes) && !r.resultReadsAsPattern(disarms) &&
		!r.resultReadsAsPattern(data) {
		return disarms, true
	}
	switch r.valueBackslashInAPattern() {
	case ValueBackslashQuotesWhatFollows:
		return quotes, true
	case ValueBackslashDisarmsWhatFollows:
		return disarms, true
	case ValueBackslashIsData:
		return data, true
	}
	return field, false
}

// escapedMarks says which bytes of a field in the escaped form are marks
// rather than data: marks[i] is true when s[i] is the backslash that marks
// s[i+1], so s[i] is not a byte of the field at all and s[i+1] is data
// whatever it looks like.
//
// It is the one reader of the escaped form, and it exists because there were
// two. The form's rule is not "a backslash is a mark" but "a backslash and
// the byte behind it are one unit", which is the only way to tell the mark in
// `\\:` — a value's colon, marked — from the value's own backslash in `\\\\`,
// where the second backslash is data and the colon behind *it* is not marked
// at all. A walk that looks at one byte at a time cannot tell those apart,
// and splitFieldsAt walked the escaped form exactly that way: it cut at a
// marked separator and left the mark on the end of the field in front of it,
// where the unescape then read it as a marked backslash and handed back a
// field one character longer than the value (#2212).
//
// nil for a field holding no backslash, which is nearly every field: there is
// nothing to mark, and the callers read nil as "every byte is data".
func escapedMarks(s string) []bool {
	if strings.IndexByte(s, '\\') < 0 {
		return nil
	}
	marks := make([]bool, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			marks[i] = true
			i++
		}
	}
	return marks
}

// globUnescape removes the marks, giving the literal field.
//
// valueBackslashMark is the one mark that leaves something behind: it stands
// where a value's backslash stood and is a backslash in the text, which is
// the half of #1370 the pattern side cannot also carry. The escape behind it
// is then read as any other mark is.
func globUnescape(s string) string {
	// Nothing to resolve is the ordinary case — a field with no backslash in
	// it and no mark standing in for one comes back as it went in. Asked
	// before building anything because the answer is almost always this one:
	// every field of every command reaches here, and a `strings.Builder` per
	// field was 19% of the allocations in the gate's workload (#1403), which
	// has no backslash anywhere in it.
	if !holdsAValueBackslash(s) && strings.IndexByte(s, '\\') < 0 {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == valueBackslashMark {
			// One backslash for a doubled mark as well as for a single one:
			// the pair is one backslash that ran out of value, not two. See
			// valueBackslashRanOutOfValue.
			if strings.HasPrefix(s[i:], valueBackslashRanOutOfValue) {
				i++
			}
			b.WriteByte('\\')
			continue
		}
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// componentSoFar is the piece of a path a rewrite has built up to here: the
// text after the last separator. A `/` is never marked — globEscape has no
// reason to mark one — so every `/` byte in the escaped form is a separator.
func componentSoFar(s string) string {
	return s[strings.LastIndexByte(s, '/')+1:]
}

// valueBackslashSurvivesAPatternPiece answers whether a value's trailing
// backslash stays in the piece it ends when a quoted `/` closes that piece, and
// it is asked only of a piece that describes a name rather than spelling one:
// where the piece spells one, every column removes the backslash with the rest
// of its quoting, so there is nothing to decide.
func (r *Runner) valueBackslashSurvivesAPatternPiece(piece string) bool {
	if !r.describesRatherThanSpells(piece) {
		return false
	}
	return r.ask(r.sem().ValueBackslashSurvivesAPatternPiece,
		"a value's trailing backslash staying in the pattern piece a quoted separator closed")
}

// slashLeavesABracket is Semantics.BracketHoldingASlashIsStillABracket, asked
// where a bracket that closes holds a live `/` and nowhere else.
//
// A bracket carrying a separator can never match, so the question is only about
// what becomes of the *word*: a pattern that matched nothing, or a word that was
// never a pattern. Nothing downstream tells those apart until an option deletes
// an unmatched pattern or refuses one, which is why this is asked at the gate
// rather than in the matcher.
func (r *Runner) slashLeavesABracket() bool {
	return r.ask(r.sem().BracketHoldingASlashIsStillABracket,
		"whether a bracket expression holding a `/` is still one when a pattern is matched against pathnames")
}

// holdsAValueBackslash reports whether a field carries either of the marks a
// value's backslash is written as. One question rather than two `IndexByte`
// calls at each site, because a site that learned one mark and not the other is
// the shape this file already has a #1370 about.
func holdsAValueBackslash(s string) bool {
	return strings.IndexByte(s, valueBackslashMark) >= 0
}

// fieldUnitAt reads the one unit of the escaped form that starts at i: the byte
// it stands for, whether that byte is **live** there, and how many bytes of the
// form it took.
//
// A unit and not a byte, for the reason escapedMarks gives: the form's rule is
// that a backslash and the byte behind it are one thing. A width of zero means
// the field ran out.
func fieldUnitAt(s string, i int) (unit string, live bool, width int) {
	switch {
	case i >= len(s):
		return "", false, 0
	case s[i] == '\\' && i+1 < len(s):
		return s[i+1 : i+2], false, 2
	}
	return s[i : i+1], true, 1
}

// hasUnescapedMeta reports whether a field is a pattern at all.
//
// numericRange, patternGroup and extendedPattern are the three constructs
// here whose being a metacharacter is a dialect question rather than a
// universal: a `<`, a `(` and an `@` are ordinary characters in a field
// everywhere else, and in the shells without ranges, bare groups or
// quantified groups none of them ever reaches a pattern at all.
//
// The group is what makes `echo f(1|2)` list `f1` and `f2` — an alternation
// with no `*` or `?` beside it is still a pattern — and it is the same
// answer, read the other way round, that keeps `p="f(1|2)"; echo $p` printing
// six characters: a field with a metacharacter in it is escaped where the
// dialect does not glob the result of an expansion, and one with none is not
// escaped because it has nothing to protect.
//
// **The quantified group is the same answer a third time**, and it was
// missing: `echo @(a|b)` reached the filesystem in no dialect, because `@`
// is not a metacharacter and the `(` behind it is only counted where bare
// groups are. `*(a|b)` and `?(a|b)` worked all along and hid it, their
// quantifier being a metacharacter in its own right — which is why the gap
// showed up as three of the five quantifiers rather than as the construct
// (#1042).
func hasUnescapedMeta(s string, numericRange, patternGroup, extendedPattern, extendedOperators bool,
	slashLeavesABracket func() bool,
) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if extendedOperators && strings.IndexByte(extendedGlobTrigger, s[i]) >= 0 {
			// The closure and the negation. They are only metacharacters
			// while the option is on, which is why the answer is threaded in
			// rather than read off the byte: `echo a#` reaches the
			// filesystem there and prints two characters everywhere else.
			//
			// The exclusion is deliberately not here: it says what to take
			// out of a search, not that there is one. Measured, `keep_a~zzz`
			// prints itself where `keep#_a~zzz` globs — see
			// [extendedGlobTrigger].
			return true
		}
		if extendedPattern && quantifiesAGroup(s, i) {
			// The quantifier and its group are one construct, so the `(` is
			// consumed with it rather than left to be counted again below —
			// which matters in the dialect that has both, where a bare `(`
			// is a metacharacter on its own.
			//
			// `i+1` is where the `(` is, and asking at `i` instead is an
			// equivalent mutant rather than a gap: closesGroup skips every
			// byte that is not a parenthesis, and the byte at `i` is a
			// quantifier. Recorded so the next reader does not go looking
			// for the row that would kill it.
			if closesGroup(s, i+1) {
				return true
			}
			continue
		}
		if s[i] == '(' && patternGroup {
			if closesGroup(s, i) {
				return true
			}
			continue
		}
		if s[i] == '<' && numericRange {
			if _, ok := numericRangeWidth(s[i:]); ok {
				return true
			}
			continue
		}
		if s[i] == '[' {
			// An unterminated bracket expression is not a pattern: `[` on
			// its own is a literal in every shell in the panel, which is
			// what makes `[ a = a ]` run the test builtin rather than being
			// globbed. Treating it as a metacharacter reported "no matches
			// found: [" on every use of `test`; the report was ignored until
			// an unmatched pattern became fatal, and then the builtin
			// stopped running at all.
			//
			// zsh alone goes further and rejects `[a` as a bad pattern where
			// the others take it literally. That divergence is recorded in
			// the corpus rather than guessed at here.
			//
			// A bracket that closes and holds a *live* `/` is the second
			// reading, and the panel parts on it: no metacharacter matches a
			// separator, so such a bracket can never match, and one column
			// reads that as the bracket not being one. See
			// Semantics.BracketHoldingASlashIsStillABracket.
			// The axis is a function and is called here and nowhere else: a
			// question this shell cannot answer must refuse the one word it
			// is about, not every field that reaches the gate.
			if closesBracket(s, i) && (!bracketHoldsASlash(s, i) || slashLeavesABracket()) {
				return true
			}
			continue
		}
		if s[i] == '*' || s[i] == '?' {
			return true
		}
	}
	return false
}

// hasUnescapedByte reports whether c stands in s outside an escape, for the
// caller that has a character in mind rather than a whole alphabet.
//
// Dropping the escape skip survives the suite, and it is an equivalent mutant
// for this one caller rather than a gap. The only way a marked byte here is a
// `|` is a value that carried `\|` of its own, and escapeValueBackslashes has
// already turned that into `\\` plus `\|`; whichever answer comes back,
// globEscape marks the `|` and the backslash to the same string, so the
// matcher is handed the same characters. The skip is kept for what the
// function *says* — the marks are not the text — since a second caller with a
// different alphabet would be told wrong without it.
func hasUnescapedByte(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == c {
			return true
		}
	}
	return false
}

// numericRangeWidth is the length of the numeric range at the start of s, for
// the caller that needs to know one is there rather than what it means.
// splitNumericRange is the reading of it; this is only the shape.
func numericRangeWidth(s string) (int, bool) {
	o := patternOpts{numericRange: true}
	_, _, rest, ok := splitNumericRange(s, &o)
	if !ok {
		return 0, false
	}
	return len(s) - len(rest), true
}

// closesGroup reports whether the parenthesized group opened at i is closed,
// counting the nested pairs and skipping the ones a backslash claims. An
// unclosed `(` is an ordinary character, the same way an unterminated bracket
// expression is.
// quantifiesAGroup reports whether the byte at i is one of the five
// quantifiers with a `(` behind it.
//
// The `(` is the whole of the test: `@` and `+` and `!` are ordinary
// characters anywhere else in a field, and `echo @x` looks for a file called
// `@x` in every shell in the panel.
func quantifiesAGroup(s string, i int) bool {
	if i+1 >= len(s) || s[i+1] != '(' {
		return false
	}
	switch s[i] {
	case '@', '?', '+', '*', '!':
		return true
	}
	return false
}

// closesGroup reports whether the group opening at i is closed.
//
// **It does not step over a bracket expression, and that is deliberate.**
// Every other scan in this family does — see skipBracket, which #3075 folded
// the rule into — and adding it here is an equivalent mutant rather than a
// fix: the only question asked of this is whether the word is a pattern at
// all, and a word holding a `[` that closes has already answered yes at the
// bracket branch in extendedGlobTrigger. A parenthesis *inside* the bracket
// is balanced by the one beside it in every text that reaches here, so the
// depth count arrives at the same answer either way. Written down so the
// next reader does not go looking for the row that would kill it: the change
// was made, and the mutant that took it back passed the whole package.
func closesGroup(s string, i int) bool {
	depth := 0
	for ; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

// closesBracket reports whether the bracket expression opened at i is closed.
//
// A `!` or `^` directly after the bracket negates, and a `]` directly after
// that is a literal member rather than the terminator — so `[]]` is a
// one-member class and `[]` is not a class at all.
// hasUnterminatedPatternGroup reports whether a pattern holds a `(` that
// nothing closes, so the question "will this compile" is asked only of the
// patterns it applies to — the companion to [hasUnterminatedBracket], and
// asked at the same three sites.
//
// A bracket expression is stepped over, which is the one way this differs
// from [closesGroup]'s deliberate omission of the same step: there the
// question is whether the word is a pattern at all and a parenthesis inside
// brackets is balanced by the one beside it, and here it is whether the
// pattern compiles, where `a[(]b` holds one parenthesis and nothing to
// balance it. Measured on zsh 5.9.2 (`-f`, 2026-09-26): `print -r -- a[(]b`
// writes `a(b` at 0, and reading the bracket's `(` as an opener would refuse
// it as a bad pattern.
func hasUnterminatedPatternGroup(p string) bool {
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '\\':
			// A backslash protects the parenthesis behind it, which is how a
			// quoted `"a(b"` and a value's own `(` both stay ordinary
			// characters: both reach here escaped.
			i++
		case '[':
			i = skipBracket(p, i)
		case '(':
			if !closesGroup(p, i) {
				return true
			}
		}
	}
	return false
}

// badPatternFromAnOpenGroup reports whether this dialect refuses a pattern
// outright because a group in it never closes.
//
// The same answer [Semantics.UnterminatedBracket] gives for a bracket, keyed
// on the same value, because it is the same rule: **a pattern that will not
// compile**. The noun is not "an unterminated group" and not "a word with a
// parenthesis in it" — `a(b|c)` carries two and compiles, `[a` carries none
// and is refused identically, and both were measured in both option states.
//
// Asked only where a lone `(` opens a group, because only there is an
// unclosed one a group at all. The **quantified** spelling is not asked, and
// that is measured rather than overlooked: bash 5.3.20 with `extglob` on and
// ksh93u+ both refuse `a@(b` while *parsing* — `unexpected EOF while looking
// for matching )` and a syntax error naming the unmatched `(` — so no word
// reaches an expansion there, and a value carrying one is ordinary text in
// both (`shopt -s extglob; v='a@(b'; [[ x == $v ]]` is 1, not a refusal).
// See [Dialect.UnterminatedPatternGroupIsAWord], which is the parsing half.
//
// Dropping the PatternAlternation test survives the package, and that is an
// **equivalent mutant** rather than a gap — recorded here so the next reader
// does not go looking for the row that would kill it. Only one dialect in the
// tree answers BracketBadPattern and it is the one with bare groups, so the
// first test already implies the second today. It is kept for what it says:
// an unclosed `(` is a group to ask about only where a lone one opens one,
// and a sixth dialect that called an unterminated bracket a bad pattern
// without taking bare groups would otherwise be asked the wrong question.
func (r *Runner) badPatternFromAnOpenGroup(p string) bool {
	return r.sem().UnterminatedBracket == BracketBadPattern &&
		r.lang().PatternAlternation && hasUnterminatedPatternGroup(p)
}

func closesBracket(s string, i int) bool {
	j := i + 1
	if j < len(s) && (s[j] == '!' || s[j] == '^') {
		j++
	}
	if j < len(s) && s[j] == ']' {
		j++
	}
	for ; j < len(s); j++ {
		if s[j] == '\\' {
			j++
			continue
		}
		if s[j] == ']' {
			return true
		}
	}
	return false
}

// splitFieldParts cuts a field into the components a pattern is matched one at
// a time against.
//
// A `/` separates whether it was quoted or not — that is the one thing quoting
// cannot take off it — so this splits on a marked separator exactly as on a
// live one and drops the mark. `strings.Split` on the field would leave the
// mark behind on the end of the piece in front, which is a backslash nobody
// wrote and a pattern piece that matches nothing.
//
// Every other mark is left alone: it is the escaped form's own and belongs to
// whatever reads the piece.
func splitFieldParts(field string) []string {
	var parts []string
	var b strings.Builder
	for i := 0; i < len(field); i++ {
		switch {
		case field[i] == '\\' && i+1 < len(field) && field[i+1] == '/':
			// The mark and the separator it records. The piece ends here.
			parts = append(parts, b.String())
			b.Reset()
			i++
		case field[i] == '\\' && i+1 < len(field):
			b.WriteByte(field[i])
			i++
			b.WriteByte(field[i])
		case field[i] == '/':
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteByte(field[i])
		}
	}
	parts = append(parts, b.String())
	return parts
}

// bracketHoldsASlash reports whether the bracket expression opening at i holds
// a **live** `/` before it closes.
//
// Asked only of a bracket [closesBracket] has already said closes, so the scan
// cannot run off the end looking for one. A marked separator is skipped with
// every other mark, which is the whole reason the mark exists: a quoted `/`
// leaves the bracket a bracket in the column this question is for, and a
// written one does not. The opening run `!`, `^` and a `]` written first are
// stepped over for the reason closesBracket steps over them — they are the
// bracket's own spelling rather than its contents.
func bracketHoldsASlash(s string, i int) bool {
	j := i + 1
	if j < len(s) && (s[j] == '!' || s[j] == '^') {
		j++
	}
	if j < len(s) && s[j] == ']' {
		j++
	}
	for ; j < len(s); j++ {
		if s[j] == '\\' {
			j++
			continue
		}
		switch s[j] {
		case ']':
			return false
		case '/':
			return true
		}
	}
	return false
}

// glob expands one field against the filesystem.
//
// The two restrictions live here rather than in the matcher, which is what
// docs/spec/grammar/patterns.md required: no metacharacter matches `/`, so
// patterns are matched one component at a time, and none matches a *leading*
// period, so a component is skipped unless its pattern begins with one.
// `case` and parameter expansion use the same matcher and have neither
// restriction, because they have no filesystem and no components.
//
// A pattern matching nothing is passed through unchanged, which is what dash,
// bash and ksh93 do by default; zsh reports an error, and that is a recorded
// axis. The second result is the run-time exception: with
// UnmatchedPatternIsEmpty on, a miss deletes the word, and true says so —
// distinct from a nil match list, which means the field was never a pattern
// or should stand as written.
// describesRatherThanSpells reports whether a piece of a field describes a
// name rather than spelling one out — whether it is a pattern at all.
//
// One function because two callers need the identical answer and they are a
// hundred lines apart: the gate at the top of [Runner.glob], which decides
// whether the field reaches the filesystem, and the question a zero-level
// `**` asks of everything ahead of it. A second copy of the composition is
// how the two would come to disagree about a `|`.
func (r *Runner) describesRatherThanSpells(s string) bool {
	// A `~(…)` group is a pattern operator in its own right: it says what
	// language the rest of the piece is written in, so a piece carrying one
	// describes a name whatever else is in it. Without this, `~(i)a.txt` and
	// `~(N)zzz` were spelled-out names — looked up as the characters they
	// were written with, found missing, and passed back through as text.
	// See tildeGlobPattern, which has the rows and the empty-group control.
	if _, ok := tildeGlobPattern(s, r.lang().TildeGroup); ok {
		return true
	}
	if hasUnescapedMeta(s, r.lang().NumericRangePattern,
		r.lang().PatternAlternation, r.lang().ExtendedPattern,
		r.MatchOption(ExtendedPatternOperators), r.slashLeavesABracket) {
		return true
	}
	// Only one of the two readings reaches the filesystem. ksh93 expands
	// `a*` and `a?` out of a value to two fields each in a directory where
	// a bar from the same value stays one field, so this is the bar and not
	// the value (#2528).
	return r.lang().PatternTopLevelAlternation.ReachesPathnameExpansion() &&
		hasUnescapedByte(s, '|')
}

func (r *Runner) glob(field string) ([]string, bool) {
	if r.noglob || r.globSuspended {
		// `set -f`, or a context that reads a word as text. Only the
		// filesystem half is switched off: a pattern in a `case` arm or
		// after `==` still matches, which is measured and is why this is
		// here rather than in the matcher.
		return nil, false
	}
	// A value's backslash is a mark until here, because until here there is
	// no whole field to ask the question of. Resolved once, in front of
	// everything that reads the field, so nothing below this line has to
	// know the mark exists — and the caller keeps the unresolved field for
	// the word a failed match restores. See resolveValueBackslashes.
	field, ok := r.resolveValueBackslashes(field)
	if !ok {
		return nil, false
	}
	// The qualifier list a pattern may carry at its end, read before
	// anything else looks at the field — it decides what the *pattern* is.
	// After `set -f`, because a word globbing is the condition for the group
	// being a list at all: measured, `setopt no_glob; echo MY ( x )` prints
	// those five characters rather than naming a file attribute.
	whole := field
	field, quals, hasQuals, qok := r.fieldQualifiers(field)
	if !qok {
		return nil, false
	}
	if (r.sem().UnterminatedBracket == BracketBadPattern &&
		field != "[" && hasUnterminatedBracket(field)) ||
		r.badPatternFromAnOpenGroup(field) {
		// zsh rejects an unterminated bracket against the filesystem too,
		// with one exception it is worth stating because it is what keeps
		// `[ a = a ]` working: a field that is exactly `[` is left alone.
		// `a[` is not, so the rule is the whole field rather than where the
		// bracket sits in it.
		//
		// **A group nothing closes is the same question**, and it is here
		// rather than in the parser for exactly the reason the switch below
		// exists: a word the lexer refuses never reaches a filename, so
		// `unsetopt badpattern` could not spare it (#4645). See
		// badPatternFromAnOpenGroup, and Dialect.UnterminatedPatternGroup-
		// IsAWord for the half that lets the word through.
		//
		// **Unless the session has turned the refusal off**, which is the
		// one place in the program that can: this is the moment a word on
		// its way to the filesystem meets a pattern that will not compile,
		// and Runner.RefusesABadPatternWhenGlobbing is read here rather than
		// in the matcher because a `case` arm and a `[[ ]]` operand are
		// refused whatever the switch says — measured, and the whole of what
		// distinguishes this switch from the axis above it.
		//
		// Withholding the refusal hands the field back **unchanged** and
		// generates nothing, which is the same `nil, false` `set -f` returns
		// a few lines up. Not BracketLiteral: with the switch off real zsh
		// writes `*[a` rather than the file `x[a` it would have matched had
		// the `[` become an ordinary character.
		if r.RefusesABadPatternWhenGlobbing() {
			// The field **unescaped**, which is what the complaint one line
			// down from here has always done for a miss and what this one
			// never did. The mark a quote or a backslash leaves behind is
			// ours and not the script's: measured on zsh 5.9.2 (`-f`,
			// 2026-09-26), `print -r -- a\[b[c` is `bad pattern: a[b[c`
			// there and was `bad pattern: a\[b[c` here. Invisible to #4630,
			// whose rows carry no backslash, and reachable by a second route
			// now that a group arrives here too — `a(b\)` is the shape that
			// found it.
			r.fatalPattern(globUnescape(field), 1)
		}
		return nil, false
	}
	// `N` in the `~(…)` group this dialect writes in front of a pattern.
	// That the field is a pattern at all is describesRatherThanSpells's
	// answer, which the group now carries on its own; this is the other
	// thing the group decides, which is what a miss does to the word. The
	// letters themselves are the matcher's, one component at a time.
	// Read without tildeGlobPattern's `/` restriction, because `N` is the
	// one letter that is about the **word** rather than about matching a
	// component: `~(N)zz*/x` names nothing and the word goes, whether or not
	// the letters beside it reach every component. The field still has to be
	// a pattern for the walk to happen at all, which is the gate below.
	if tilde, ok := tildePrefixModifier(field, r.lang().TildeGroup); ok && tilde.null {
		// The same answer the qualifier list already had for zsh's `(N)`,
		// reached by the other dialect's spelling, so the two cannot come to
		// disagree about what deleting a word means.
		quals.allowNoMatch = true
	}
	if !hasQuals && !r.describesRatherThanSpells(field) {
		// The bar is the one metacharacter hasUnescapedMeta must not count
		// on its own — a field holding nothing else is not a pattern in the
		// dialects where it only means something inside a group, and
		// counting it there would send every such field to the filesystem.
		// Where a top-level one *is* an alternation it is a pattern by
		// itself, and measured: with `L='a|b'` in a directory holding `a`
		// and `b`, `print -l -- ${~L}` lists both files in zsh 5.9.2 (#1497).
		//
		// The same composition resultReadsAsPattern already makes one level
		// up, and for the same reason: the escaping is what says the bar was
		// live, so a quoted one never reaches here unescaped.
		return nil, false
	}
	// The `~` exclusions, taken off the field before it is split into
	// components, because they are the one pattern operator that is *looser*
	// than `/`.
	//
	// Measured on zsh 5.9.2, 2026-09-10, in a tree holding `/tmp/gx/keep_a`
	// and `/tmp/gx/gxdir`: `print -l -- /tmp/gx/*~*gx*` answers nothing,
	// where the same exclusion run from inside that directory keeps
	// `keep_a`. So the right side is matched against **the whole word the
	// left side produced**, with `/` an ordinary character in it — a `*`
	// there crosses directories where the same `*` on the left does not —
	// and it is the word *as written* rather than a cleaned or absolute
	// path: `cd /tmp; ./gx/*~./gx/keep_a` takes `keep_a` out and
	// `./gx/*~gx/keep_a` does not.
	//
	// This is why it cannot be answered inside [Runner.matchIn] with the
	// rest of a component's pattern, which was the shape that refused it
	// by name until now (#1719).
	field, excl := r.fieldExclusions(field)
	// A list makes the field a pattern whatever is in front of it: `f1(.)`
	// is `f1` where the name alone is no pattern at all, so the qualifiers
	// are what sent it to the filesystem.
	defer func() {
		if r.ctl == controlExit || r.ctl == controlAbandon {
			// The pattern was rejected while it was being read, or something
			// before it gave up, and either way what is running is already
			// on its way out. Reporting a miss on top of that says the
			// pattern matched nothing, which is a different and weaker claim
			// than the one already made.
			//
			// It also keeps one refusal to one sentence where a word is
			// expanded more than once. A redirection target is expanded in
			// three views — see Runner.expandRedirectTargetViews — so a
			// pattern that misses in the target of `cat < nosuch*` reaches
			// here twice, and the second pass finds the first pass's
			// unwinding here. Naming only controlExit covered the shell that
			// stops and not the one that gives up the statement, which wrote
			// the complaint twice.
			r.globMissed = false
			return
		}
		if r.globMissed {
			// The axis is asked whether or not the option has already
			// decided, so a dialect that answered nothing about it is still
			// told so.
			axis := r.ask(r.sem().GlobNoMatchIsError, "an unmatched pattern being an error")
			// Two routes to one refusal, and they order themselves against
			// the emptying option differently — see UnmatchedPatternIsError,
			// where the measurement for each is written down. The option is
			// a script's own request and wins outright; the axis is the
			// shell's standing answer and yields to a script that asked for
			// the word to be deleted.
			//
			// Reporting it and then passing the pattern through was the same
			// report-then-continue bug as the others, which is why neither
			// route stops at the diagnostic.
			if r.MatchOption(UnmatchedPatternIsError) ||
				(axis && !r.MatchOption(UnmatchedPatternIsEmpty)) {
				r.refuseUnmatchedPattern(globUnescape(whole))
			}
		}
		r.globMissed = false
	}()
	// A miss is a miss wherever it is noticed, and what it means is decided
	// once: the word is deleted if the option says so, kept otherwise.
	missed := func() ([]string, bool) {
		if quals.allowNoMatch {
			// `N` is `null_glob` for one pattern: the word is deleted and
			// nothing is said. Measured, `echo zz*(N)` prints an empty line
			// at status 0 where `echo zz*` is fatal.
			return nil, true
		}
		r.globMissed = true
		return nil, r.MatchOption(UnmatchedPatternIsEmpty)
	}
	if len(excl) > 0 && strings.HasSuffix(field, "/") {
		// The left side ends at a `/`, so its last component is the empty
		// pattern — and no file is named nothing. The trailing slash that
		// means "directories only" is the one at the end of the *word*, and
		// this one is not: measured, `/tmp/gx/sub/` lists `/tmp/gx/sub/` and
		// `/tmp/gx/sub/~*zzzz*` lists nothing at all.
		return missed()
	}
	// `D` is `glob_dots` for one pattern, and the option is the other way
	// into the same question.
	seeHidden := r.MatchOption(PatternsMatchHidden) || quals.seeHidden ||
		r.ignoredNamesRevealHidden()
	starstar := r.MatchOption(StarStarCrossesDirectories)
	starstarAlone := r.MatchOption(StarStarAloneCrossesDirectories)
	parts := splitFieldParts(field)
	// Whether a zero-level `**` is reported with the separator the pattern
	// wrote in front of it, which is decided by what stands ahead of the
	// last `**` **as written** — ahead of it in the field, before the run
	// below is collapsed and before the walk has looked at anything.
	//
	// Read here rather than at the component for a reason the collapse makes
	// plain: `a/**/**` and `a/**` list the same names, and bash reports the
	// zero-level one as `a` for the first and `a/` for the second. The run
	// that collapses away is still a component that described rather than
	// spelled, so the answer cannot be taken from what is left.
	selfKeepsSeparator := false
	for j := len(parts) - 1; j >= 0; j-- {
		if parts[j] == "**" {
			selfKeepsSeparator = r.spelledOut(parts[:j])
			break
		}
	}
	if starstar && r.MatchOption(RepeatedStarStarIsOneComponent) {
		parts = collapseStarStarRun(parts)
	}
	// And whether a zero-level `**` has a match to report at all, for the
	// dialect that does not report the directory the walk stood in. Read
	// **after** the collapse, which is the opposite of the line above and is
	// measured rather than symmetric: `a/**/**` is `a a/a …` in bash and
	// `a/a …` in ksh93, so the run that collapses away still describes for
	// the separator and no longer counts for this. See
	// StarStarZeroLevelIsTheDirectoryItStartsFrom and globZeroLevelSource.
	zeroLevelFromAListing := globZeroLevelSource(r, parts)
	// And whether this field holds a level-crossing `**` at all, which is
	// what makes its listings physical in the dialect that walks such a
	// field. Hoisted out of the loop because it is a property of the field
	// and the loop would rescan it once per component.
	holdsAStarStar := starstar && slices.Contains(parts, "**")
	// And the listing it reads: the matches the component ahead of the `**`
	// produced, before the gate below takes the ones that are no directory
	// out of them. Held one component at a time, because the `**` that reads
	// it is the next component or none.
	var listed []string

	// The slashes a pattern ends with are text, and they come back on every
	// match. `*/` is the standard spelling of "directories only" and the
	// whole panel answers `d1/ d2/` where this walk answered `d1 d2` — a
	// quiet wrong answer rather than a loud one, because the name alone is
	// still usable for `cd` and only stops being right once something joins
	// it to a second path or compares the two spellings (#1350).
	//
	// The filtering that makes the idiom mean what it means was already
	// here: the empty component the trailing slash leaves in parts is what
	// puts the real one under `i < len(parts)-1` and keeps only directories.
	// So all that was missing is writing the slash back.
	//
	// **The run is reproduced as written rather than normalized to one**,
	// which is measured and is where the panel splits. dash, ksh93 and zsh
	// answer `d1//` for `*//` and `d1///` for `*///` — the trailing text
	// comes back byte for byte — while bash alone collapses the run to a
	// single slash. Reproducing it is the same rule the mid-pattern case
	// already follows unanimously, `cx//*` being `cx//ax` in all six
	// columns, so it is the reading that stays consistent rather than the
	// one that needs a second rule for the end of the word. bash's collapse
	// is a divergence recorded in the corpus and not implemented; it is a
	// question about a shape no script writes.
	trail := field[len(strings.TrimRight(field, "/")):]

	// The order the dialect's sort parameter asks for, read once for the
	// whole expansion — it cannot change while one runs — and false for the
	// order this walk would give anyway, which is every column but one and
	// every script but the one that asked. See interp/globsort.go.
	//
	// Read here rather than at the end because one of its values reaches the
	// walk: `nosort` is the order the *directory* gave, which is destroyed
	// by the per-component sorts below and cannot be recovered from the
	// result. Measured, `GLOBSORT=nosort; echo */*` is the top level in the
	// directory's own order with each level below it in its own.
	order, ordered := r.globSortOrderOf()
	unsorted := ordered && order.key == globSortNone
	if unsorted {
		// The listing's own order is what is asked for, and the listing is
		// several calls below here — see Runner.globUnsorted.
		saved := r.globUnsorted
		r.globUnsorted = true
		defer func() { r.globUnsorted = saved }()
	}

	// An absolute pattern starts at the root; a relative one at the working
	// directory, which is the shell's rather than the process's.
	base := r.workDir()
	dirs := []string{base}
	prefix := ""
	if parts[0] == "" {
		dirs, prefix, parts = []string{"/"}, "/", parts[1:]
	}

	// The directories `**` matched zero levels deep, when it was the last
	// component: the one shell with the option reports those ahead of what
	// is inside them — `d/**` lists `d/` and then `d/e`.
	var selfDirs map[string]bool

	// An empty component — two adjacent slashes — is a separator the pattern
	// wrote and every column writes back: `cx//*` is `cx//ax` in all six, and
	// `cx///*` is `cx///ax`. It used to be dropped, so the match came back
	// with one slash where the pattern had two (#1511).
	//
	// It is **held** rather than applied, and that is what keeps the trailing
	// run out of it. The slashes a pattern ends with are already written back
	// by `trail`, and the empty component the trailing slash leaves is also
	// what puts the real last component under `i < len(parts)-1` and keeps
	// only directories — so applying every empty component where it stands
	// would put a second slash on the end of `*/`. Flushing only when a
	// *real* component follows makes the distinction without a second test:
	// nothing follows a trailing run, so nothing flushes it.
	//
	// n held components mean n+1 slashes between the two real ones, and the
	// join supplies one of them — except against a directory already ending
	// in a separator, the root, where it supplies none.
	held := 0
	flush := func() {
		if held == 0 {
			return
		}
		for j, dir := range dirs {
			n := held
			if !strings.HasSuffix(dir, "/") {
				n++
			}
			dirs[j] = dir + strings.Repeat("/", n)
		}
		held = 0
	}

	// The names the dialect's ignore parameter takes back out. Read once for
	// the whole expansion rather than per word, because the parameter cannot
	// change while one runs — and read *before* the walk, because one of the
	// two readings is applied to each listing as the walk produces it. See
	// interp/ignorednames.go, and Semantics.IgnoredNamesMatchTheLastComponent
	// for which reading a dialect holds.
	ignore := r.ignoredNamePatterns()
	filterListing := false
	if len(ignore) > 0 {
		filterListing = r.ignoredNamesFilterTheListing()
		if r.unspecified {
			ignore = nil
		}
	}

	// Where the walk may go on from, set by a `**` component and read by the
	// gate below it — nil for every other component, which is what makes the
	// restriction belong to `**` and not to descent in general.
	//
	// It is a set rather than a filter because what `**` **matched** and
	// what the walk may **enter** are two different lists: a symbolic link
	// to a directory is matched, so `**/` names it, and is never entered.
	var onward map[string]bool

	for i, part := range parts {
		if part == "" {
			held++
			continue
		}
		flush()
		onward = nil
		var next []string
		// The two questions a `**` component raises, and they are separate:
		// whether it crosses levels at all, and whether it still does with
		// nothing behind it. A **slash** is what the second one asks about,
		// so the index is the test rather than lastComponent — `**/` has a
		// component after it, empty and written, and is level-crossing in
		// all three shells that have the construct, where bare `**` is not.
		slashed := i < len(parts)-1
		if starstar && part == "**" && (slashed || starstarAlone) {
			// The component is the directory itself and everything beneath
			// it. Exactly `**`: anything more — `a**`, an escaped star — is
			// an ordinary component, where adjacent stars collapse to one.
			last := lastComponent(parts, i)
			// Where a zero-level `**` takes its match from, which is the one
			// question about `**` the panel splits three ways. On, it is the
			// directory the walk stands in — `d/**` names `d/`. Off, it is
			// what the component ahead of this one listed, which is nothing
			// where the path was spelled out and a plain file where the
			// listing held one. See
			// StarStarZeroLevelIsTheDirectoryItStartsFrom.
			selfIsTheStart := r.MatchOption(StarStarZeroLevelIsTheDirectoryItStartsFrom)
			onward = map[string]bool{}
			if last && !selfIsTheStart && zeroLevelFromAListing {
				for _, m := range listed {
					if selfDirs == nil {
						selfDirs = map[string]bool{}
					}
					next = append(next, m)
					selfDirs[m] = true
				}
			}
			for _, dir := range dirs {
				if !last || selfIsTheStart {
					// The directory itself: an answer where this is the last
					// component, and a place to carry on from where it is
					// not. The dialect that takes its zero-level match from
					// the listing above still needs the second of those, so
					// only the *answer* hangs on the option.
					next = append(next, dir)
					if last {
						if selfDirs == nil {
							selfDirs = map[string]bool{}
						}
						selfDirs[dir] = true
					}
				}
				// The directory the component starts from is one the walk
				// may go on from however it was reached, in two of the three
				// — measured, `s/**/x` through a symlink `s` is `s/x` in
				// bash 5.3.15 and in zsh, and no match at all in ksh93. The
				// rule bounds where a `**` **descends to**, and this is
				// whether the pattern naming a starting point is exempt from
				// it. See StarStarPatternsReadLinkedDirectories.
				if r.linkedToAPhysicalWalk(dir) {
					continue
				}
				onward[dir] = true
				next = r.appendDescendants(next, dir, seeHidden, onward)
			}
			if !unsorted {
				sortMatches(next)
			}
		} else if !r.describesRatherThanSpells(part) {
			// A component that **spells a name out** rather than describing
			// one, which is resolved by asking whether the path is there and
			// never by listing the directory above it.
			//
			// Unanimous across the panel, and visible without any permission
			// fixture: with a directory `Dir` holding `File`, `*/file`
			// answers `Dir/file` in bash 5.3.20, zsh 5.9.2, ksh93u+ and dash
			// 0.5.12 alike — the spelling the *pattern* wrote, where a match
			// found in a listing would have carried the spelling on disk.
			// (It resolves at all because the filesystem it was measured on
			// folds case; what the row shows is which of the two routes the
			// answer came down, and that is the same on either kind.) The
			// same probe says a fold — `nocaseglob` — does not reach such a
			// component, since a stat has no case rule of its own.
			//
			// Listing the directory instead is wrong in both directions, and
			// #3387 has the measurement for each:
			//
			//	*/f   under a directory that is `--x`   every column finds it,
			//	                                        a listing cannot
			//	*/.   under a directory that is `---`   no column finds it,
			//	                                        a listing of the
			//	                                        *parent* offers it
			//
			// `.` and `..` are this case rather than a case of their own,
			// which is what folded the branch that used to stand here: no
			// listing reports either name, so they were joined unchecked, and
			// that is exactly the second row above. Every other literal fell
			// through to the listing and is the first.
			//
			// The literal behind the quoting marks is what is joined, because
			// quoting a component does not change what it names: `"."/cx/*`
			// and `\./cx/*` both list `./cx/ax` in all six.
			//
			// lstat rather than stat, measured: a literal component naming a
			// **dangling** symbolic link matches in every column — `*/d` is
			// `x/d` with `x/d` pointing nowhere — while `*/d/` matches in
			// none, because the trailing separator asks a question about the
			// target that this component does not.
			//
			// This is also the branch the comment below has always described:
			// a literal component is joined and stat'd, so it reaches through
			// a symbolic link even in the dialect that walks a `**` field
			// physically. Until now it went through the listing with every
			// other component and the physical rule applied to it.
			lit := globUnescape(part)
			for _, dir := range dirs {
				joined := globJoin(dir, lit)
				if _, err := r.lstat(joined); err != nil {
					continue
				}
				next = append(next, joined)
			}
		} else {
			// A pattern component in a field that holds a level-crossing
			// `**` is read physically in the dialect that walks such a field
			// rather than descending it, so a directory reached through a
			// symbolic link is not listed: `*/*/**` names nothing under a
			// linked `t` in ksh93 while `*/*` lists three names under it in
			// the same shell. A **literal** component is joined and stat'd
			// rather than listed, so it reaches through the link in every
			// column — `*/a/**` is `t/a/b …` there too, which is why this
			// asks the same question globZeroLevelSource asks.
			describes := r.describesRatherThanSpells(part)
			physical := holdsAStarStar && describes
			// A component that spelled a name rather than describing one
			// reaches the filesystem by a lookup and not by a listing, so
			// the listing filter has nothing to look at. Measured on
			// ksh93u+ 2026-09-16: `FIGNORE='b.txt'` takes nothing out of
			// `*/b.txt` while it empties `d/*` of everything but `.` and
			// `..`, and `FIGNORE='d'` leaves `d/*` alone while it makes
			// `*/b.txt` a word with no match at all.
			var listingIgnore []string
			if filterListing && describes {
				listingIgnore = ignore
			}
			o := r.patternOpts(part)
			o.fold = r.MatchOption(GlobFoldsCase)
			// The subjects are the names in each directory, which are not
			// read yet; the pattern is what there is to ask about, and a
			// pattern of ASCII against a name that is not is the case the
			// narrowing leaves alone anyway — an ASCII letter folds the same
			// way in every locale.
			o.foldWide = o.fold && r.caseFoldReachesBeyondASCII(part)
			for _, dir := range dirs {
				if physical && r.linkedToAPhysicalWalk(dir) {
					continue
				}
				next = append(next, r.matchIn(dir, part, o, seeHidden, listingIgnore)...)
			}
		}
		if len(next) == 0 {
			return missed()
		}
		if !unsorted {
			sortMatches(next)
		}
		dirs = next
		listed = next
		if i < len(parts)-1 {
			// Only directories can be descended into.
			// Through the gate, like every stat: a match the policy hides
			// is not descended into, the same as a match that is no
			// directory.
			//
			// A `**` component answers this itself, because the set it hands
			// on need not be the set it matched: it walked without following
			// a symbolic link, and whether a link it listed is a level the
			// next component may look inside is the dialect's to say. Two
			// questions, not one, because the panel answers them with
			// different columns. Where nothing real follows — `**/`, where
			// this filter is producing the answer rather than choosing where
			// to look next — StarStarSeesLinkedDirectories decides. Where a
			// component does follow, ComponentBehindStarStarSeesLinkedLevels
			// does — except of a `**` that both begins the word and has one
			// separator behind it, which is measured rather than chosen and
			// is written out at the option. `i` is the component's own index
			// because the run of `**` has already collapsed to one.
			behind := !lastComponent(parts, i) &&
				(i > 0 || parts[i+1] == "") &&
				r.MatchOption(ComponentBehindStarStarSeesLinkedLevels)
			sees := onward == nil ||
				(lastComponent(parts, i) && r.MatchOption(StarStarSeesLinkedDirectories)) ||
				behind
			var kept []string
			for _, d := range dirs {
				if !sees && !onward[d] {
					continue
				}
				if info, err := r.stat(d); err == nil && info.IsDir() {
					kept = append(kept, d)
				}
			}
			dirs = kept
			if len(dirs) == 0 {
				return missed()
			}
		}
	}

	// Results are reported the way the pattern was written: relative if it
	// was relative, so `echo *` lists names and not paths — and spelled the
	// way the pattern spelled it, which is why this strips a prefix rather
	// than asking filepath.Rel. Rel *cleans*, so it would answer `cx/ax`
	// where all six columns answer `./cx/ax` even once the walk carries the
	// component; the whole point of globJoin is undone by one call here.
	//
	// A function because the exclusions below need the same answer: they are
	// matched against the word rather than against the absolute path the
	// walk is holding, and rendering it twice two ways is how the two would
	// drift apart.
	rel := strings.TrimSuffix(base, "/") + "/"
	render := func(d string) (string, bool) {
		self := selfDirs[d]
		if prefix == "" {
			if d == base {
				// The starting point itself, which only a zero-level `**`
				// can produce, and which the shell with the option leaves
				// out: `**` lists what is beneath the directory, never the
				// directory. Asked against the base rather than against a
				// rendered `.`, because `.` is now a spelling a pattern can
				// legitimately produce — zsh's `.(/)` is `.` — and the two
				// are different strings here: this one is `<base>`, that one
				// is `<base>/.`.
				return "", false
			}
			d = strings.TrimPrefix(d, rel)
		}
		if self && trail == "" && selfKeepsSeparator && !strings.HasSuffix(d, "/") {
			// The zero-level `**` writes its own separator, and only three
			// things can stop it.
			//
			// **The pattern already asked for one.** `cx/**/` is
			// `cx/ cx/dx/` in the two shells that cross levels, not
			// `cx// cx/dx/`, so the two sources of a trailing slash are one
			// slash and not two — which is what `trail` covers. The same
			// answer a second way for a run the pattern wrote mid-word:
			// `a//**` is `a// a//b …`, so the separators already standing
			// are the ones reported and none is added behind them.
			//
			// **Something ahead of the component described a name rather
			// than spelling one.** See [Runner.spelledOut] — this is the
			// whole of what selfKeepsSeparator carries.
			d += "/"
		}
		return d + trail, true
	}

	if len(excl) > 0 {
		// Ahead of the qualifiers, which is measured: `/tmp/gx/*~*gxdir*([1])`
		// is `/tmp/gx/keep_a`, so the list `[1]` counts into has already had
		// the exclusion taken out of it — the other order would pick `gxdir`
		// and then throw it away.
		//
		// **A mutation that swaps these two blocks survives today**, and is
		// recorded so the next reader does not go hunting for the row that
		// would kill it: both are filters, and two filters commute. The
		// order becomes observable only for a qualifier that *selects* —
		// `[1]`, `[-1]`, `om` — and none of those is implemented. What is
		// observable is the order against a **modifier**, which replaces the
		// word rather than filtering it, and that is pinned:
		// `d/p_*~*d/*(N:t)` is empty here and in zsh, where running `:t`
		// first would leave all three.
		var kept []string
		for _, d := range dirs {
			w, ok := render(d)
			if !ok {
				continue
			}
			if r.excludedBy(w, excl) {
				continue
			}
			kept = append(kept, d)
		}
		dirs = kept
		if len(dirs) == 0 {
			return missed()
		}
	}

	if hasQuals {
		// Narrowed here, on the absolute paths the walk produced, because a
		// type test is a question about a file and the relative names below
		// are not what would answer it.
		dirs = r.keepQualified(dirs, quals)
		if len(dirs) == 0 {
			return missed()
		}
	}

	// And the other reading of the same parameter: the whole word the
	// expansion produced, matched against the word rather than against the
	// path the walk is holding — the same subject the `~` exclusions above
	// are matched against, and for the same reason: `GLOBIGNORE='./a.txt'`
	// takes `./a.txt` out where `GLOBIGNORE='a.txt'` does not, so it is the
	// word as the pattern spelled it. See interp/ignorednames.go.
	//
	// Only where the dialect does not filter the listings, which is where
	// the two readings genuinely part: one of them has already run, once per
	// component, and running this one over it as well would take out a word
	// whose *last* component matched a pattern the listing did not produce.
	//
	// Where it stands against the qualifiers below is not observable and is
	// not claimed to be: the one dialect with this parameter has no
	// qualifiers and the one with qualifiers has no such parameter. A
	// mutation that moves this block past them survives, which is recorded
	// here so the next reader does not go hunting for the row that would
	// kill it.
	out := make([]string, 0, len(dirs))
	// The paths the words came from, kept beside them only where an order
	// was asked for: a size or a time is a question about a file and the
	// word is not what can be asked it. See Runner.sortMatchesBy.
	var paths []string
	if ordered {
		paths = make([]string, 0, len(dirs))
	}
	for _, d := range dirs {
		w, ok := render(d)
		if !ok {
			continue
		}
		if !filterListing && len(ignore) > 0 && r.ignoredName(w, ignore) {
			continue
		}
		out = append(out, w)
		if ordered {
			paths = append(paths, d)
		}
	}
	if quals.modifiers != "" {
		// Before the sort, because the shell sorts what the modifiers
		// produced rather than what they were given: `*/*(N:e)` answers
		// `md txt` for names that arrived as `b.txt c.md`.
		for i, w := range out {
			m, ok := r.applyGlobModifiers(w, quals.modifiers)
			if !ok {
				return nil, false
			}
			out[i] = m
		}
	}
	if ordered {
		r.sortMatchesBy(out, paths, order)
	} else {
		sortMatches(out)
	}
	if len(out) == 0 {
		// Everything matched was the starting point itself — `**` over an
		// empty directory — which is no match at all.
		return missed()
	}
	return out, false
}

// fieldExclusions peels a field's top-level `~` exclusions off, returning the
// pattern the walk should generate from and the patterns that take matches
// back out of it.
//
// Nothing is peeled where the operator is off, where the field carries no
// top-level `~`, or where the `~` is inside a group — `(*~sub)/*` keeps its
// exclusion inside the one component, which is the reading the walk already
// gives it and which zsh agrees with.
func (r *Runner) fieldExclusions(field string) (string, []string) {
	if !r.MatchOption(ExtendedPatternOperators) {
		return field, nil
	}
	o := patternOpts{extended: true}
	left, rights, ok := splitExclusion(field, &o)
	if !ok {
		return field, nil
	}
	return left, rights
}

// excludedBy reports whether a generated word is taken back out by one of a
// pattern's `~` exclusions.
//
// The word is the subject and the exclusion is an ordinary pattern over it:
// no component splitting, so a `*` crosses `/`, and no leading-period rule,
// so `*~*hidden*` takes `.hidden` out of a listing that `(D)` put it into.
// Both are measured, and both are the opposite of what the walk does with the
// left side.
func (r *Runner) excludedBy(word string, rights []string) bool {
	for _, x := range rights {
		o := r.patternOpts(x, word)
		o.fold = r.MatchOption(GlobFoldsCase)
		o.foldWide = o.fold && r.caseFoldReachesBeyondASCII(x, word)
		if matchPattern(x, word, o) {
			return true
		}
	}
	return false
}

// lastComponent reports whether nothing but trailing slashes follows parts[i].
// spelledOut reports whether every component ahead of a `**` is a name the
// pattern wrote out rather than one it described.
//
// It is the question that decides how a **zero-level** `**` is reported, and
// it is the only thing that decides it. Measured 2026-09-13 against bash
// 5.3.15 under `shopt -s globstar`, in a tree holding `a/b/c`, `a/f1`,
// `a/b/f2`, `a/b/c/f3`, `d/e/f4` and `top`:
//
//	a/**        a/ a/b a/b/c a/b/c/f3 a/b/f2 a/f1
//	a/b/**      a/b/ a/b/c a/b/c/f3 a/b/f2
//	a//**       a// a//b a//b/c a//b/c/f3 a//b/f2 a//f1
//	"a"/**      a/ a/b a/b/c a/b/c/f3 a/b/f2 a/f1
//	*/**        a a/b a/b/c a/b/c/f3 a/b/f2 a/f1 d d/e d/e/f4
//	?/**        a a/b …  d d/e d/e/f4
//	[a]/**      a a/b a/b/c a/b/c/f3 a/b/f2 a/f1
//	a/*/**      a/b a/b/c a/b/c/f3 a/b/f2
//	a/**/**     a a/b a/b/c a/b/c/f3 a/b/f2 a/f1
//	**/c/**     a/b/c a/b/c/f3
//
// So the separator is not a property of the directory and not a property of
// the `**`: `a/**` and `*/**` report the same directory two different ways,
// and the only difference between the two patterns is one component nobody
// looked at. Quoting it or escaping the slash changes nothing, which is what
// makes this a question about the pattern's **metacharacters** rather than
// about its source text — `"a"/**` reports `a/` exactly as `a/**` does.
//
// An earlier `**` counts as describing, so the rule composes with itself:
// `a/**/**` reports `a`, not `a/`, even though everything the reader can see
// ahead of the last component was spelled out.
//
// zsh is not a second reading of this. Its bare `**` does not cross levels at
// all, so no zero-level match arises there without a trailing slash — and
// with one, the slash comes from the pattern and this never runs.
//
// ksh93 *is* a second reading, and since #3152 it is modeled — under an
// option of its own rather than as this function's other answer, because the
// two ask different questions about the same prefix. This one asks whether
// the path was **spelled**, and writes a separator where it was. That one
// asks where the starting directory's name **came from**, and reports it only
// where a listing produced it. See globZeroLevelSource and
// StarStarZeroLevelIsTheDirectoryItStartsFrom, and note that the two are read
// on either side of the `**` run's collapse: `a/**/**` is `a` here and
// nothing there.
func (r *Runner) spelledOut(ahead []string) bool {
	for _, p := range ahead {
		if r.describesRatherThanSpells(p) {
			return false
		}
	}
	return true
}

func lastComponent(parts []string, i int) bool {
	for _, p := range parts[i+1:] {
		if p != "" {
			return false
		}
	}
	return true
}

// appendDescendants adds everything beneath dir, however deep: files and
// directories both, because whether only directories survive is the caller's
// question — the same split the main loop already makes.
//
// Hidden names are skipped, and skipped for descent too, unless the option
// says otherwise. A symbolic link is listed and never followed: following one
// is how a walk finds the same file twice and a looped link forever.
//
// onward collects the directories this walk actually entered, which is the
// half the caller cannot reconstruct afterwards. Asking the filesystem again
// gives the wrong answer by design: os.Stat follows a link, so a link to a
// directory reads back as a directory and the whole restriction disappears.
// The listing already holds the fact — a DirEntry answers from the name's own
// type — so the set is recorded where it is known and never re-derived.
//
// A method so each directory read passes the gate — `echo /**` enumerates
// whatever it can reach, which is exactly the walk a policy wants to see. A
// denied directory reads as empty and the walk goes no deeper there.
//
// `.` and `..` are two of the names the listing holds where the dialect says
// it holds them, and the descent produces them at every level it reads — the
// same answer Semantics.GlobListsDotAndDotDot gives a component match, asked
// at the second place a listing is read. They are produced and never
// followed, which is a rule of its own rather than the same one: a walk that
// descended into `..` would climb out of the tree it was given and never
// stop, and no column does that. So neither name reaches onward and neither
// is recursed into, and what a component behind the `**` may look inside is
// unchanged by them.
func (r *Runner) appendDescendants(out []string, dir string, seeHidden bool, onward map[string]bool) []string {
	entries, err := r.readDir(dir)
	if err != nil {
		return out
	}
	// The leading-period rule is what keeps both names out of an ordinary
	// descent, exactly as it keeps them out of an ordinary `*`: they are
	// visible only once something has turned that rule off, which is the
	// ignore parameter or the option that matches a leading period.
	if seeHidden && r.sem().GlobListsDotAndDotDot == Yes {
		out = append(out, globJoin(dir, "."), globJoin(dir, ".."))
	}
	for _, e := range entries {
		name := e.Name()
		if !seeHidden && strings.HasPrefix(name, ".") {
			continue
		}
		path := globJoin(dir, name)
		out = append(out, path)
		if e.IsDir() {
			onward[path] = true
			out = r.appendDescendants(out, path, seeHidden, onward)
		}
	}
	return out
}

// linkedToAPhysicalWalk reports whether a directory is one this dialect's `**`
// walk refuses to read: a symbolic link, in the shell that answers a `**`
// pattern with a physical walk.
//
// The lstat is skipped entirely where the dialect reads such a link like any
// other directory, which is two of the three columns and every expansion that
// has no `**` in it — so the cost lands only where the answer can differ. See
// StarStarPatternsReadLinkedDirectories.
func (r *Runner) linkedToAPhysicalWalk(dir string) bool {
	if r.MatchOption(StarStarPatternsReadLinkedDirectories) {
		return false
	}
	info, err := r.lstat(dir)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

// globZeroLevelSource reports whether a zero-level `**` has a match to report
// in the dialect that does not report the directory the walk stands in.
//
// The other reading of [Runner.spelledOut]'s question, and it is a different
// question rather than that one negated — which is the whole reason this
// function exists instead of a `!`. bash asks whether the path ahead of the
// `**` was *spelled*, and writes a separator where it was. ksh93 asks where
// the starting directory's **name came from**, and reports it only where a
// listing produced it.
//
// A literal component is joined onto the path and stat'd, so nothing listed
// it. A pattern is resolved by reading the directory it sits in, so its
// matches are a listing. And a literal that follows a `**` is a listing too,
// because that walk reads every level it crosses on the way past. That is the
// whole rule, and it is measured rather than derived — the four rows that
// pull the three clauses apart, 2026-09-16 on ksh93u+ 2012-08-01 in a tree
// holding `p/pf`, `p/q/qf`, `p/q/w/leaf` and `p/q/w/q/deepq`:
//
//	*/**         [p][topf]…      the `*` listing, a plain file with it
//	*/q/**       no `p/q`        `q` was joined onto what `*` matched
//	**/q/**      [p/q]…          the walk past `q` had already listed it
//	**/q/w/**    no `p/q/w`      and `w` was joined onto that
//
// The last two are the pair that says this is not a question about the
// prefix as a whole: both spell a literal `q`, both hold a `**` ahead of it,
// and they differ because only one has the `**` immediately behind the
// component the `**` starts from. `p/**/w/**` reports `p/q/w` and
// `p/*/w/**` does not, the same split with the run moved one component in.
//
// Read on the **collapsed** parts, unlike spelledOut: `a/**/**` is one `**`
// by the time the walk runs, so the component ahead of it is the literal `a`
// and ksh93 reports nothing — measured, against bash's `a` for the same
// pattern.
func globZeroLevelSource(r *Runner, parts []string) bool {
	last := -1
	for j := len(parts) - 1; j >= 0; j-- {
		if parts[j] == "**" {
			last = j
			break
		}
	}
	if last < 0 {
		return false
	}
	// The separators a pattern wrote are not components anybody listed, and
	// they stand between the ones that are: `d//**` reads `d` as the
	// component ahead of the `**`, exactly as `d/**` does.
	ahead := make([]string, 0, last)
	for _, p := range parts[:last] {
		if p != "" {
			ahead = append(ahead, p)
		}
	}
	if len(ahead) == 0 {
		// Nothing ahead of it at all, so the walk starts where the pattern
		// does and no listing named that. `**` alone is the case, and the
		// starting point is left out of it in every column.
		return false
	}
	if r.describesRatherThanSpells(ahead[len(ahead)-1]) {
		return true
	}
	return len(ahead) > 1 && ahead[len(ahead)-2] == "**"
}

// collapseStarStarRun reads a run of `**` components, separators and all, as
// a single `**`.
//
// The run rather than the pair, and the separators with it: `**/**/**` is one
// component in the shells that do this, and so is `**//**`, where the empty
// component an ordinary pattern reproduces — `cx//*` is `cx//ax` in all six
// columns — goes with the run instead of surviving it.
//
// It matters because nothing downstream takes duplicates out of a pathname
// expansion, and no shell in the panel does either. Two `**` components are
// two alternatives, each standing for zero or more levels, so a directory
// three deep is reached four ways and named four times. That is what zsh
// answers; what the shells with this option answer is the list once. See
// RepeatedStarStarIsOneComponent for the measurement.
func collapseStarStarRun(parts []string) []string {
	var out []string
	for i := 0; i < len(parts); i++ {
		if parts[i] != "**" {
			out = append(out, parts[i])
			continue
		}
		// The last `**` reachable from here across nothing but separators.
		// Ending the run at that one rather than at the first component
		// which is neither is what keeps a trailing slash out of it: the
		// empty component a pattern ends with has no further `**` behind it,
		// so it is never inside a run and still writes its own separator.
		last := i
		for j := i + 1; j < len(parts); j++ {
			if parts[j] == "**" {
				last = j
			} else if parts[j] != "" {
				break
			}
		}
		out = append(out, "**")
		i = last
	}
	return out
}

// sortMatches puts a glob's matches in order.
//
// Which order that is — and why it is a decision rather than the absence of
// one — is shellOrder in interp/order.go, and it is there rather than here
// because a pathname expansion is not the only surface that asks.
func sortMatches(names []string) { slices.SortFunc(names, shellOrder) }

// matchIn lists the entries of dir matching one pattern component. seeHidden
// lifts the leading-period rule, which is the run-time option's doing and not
// the pattern's. A method so the listing passes the gate; a denied directory
// matches nothing, as an unreadable one does.
func (r *Runner) matchIn(dir, pattern string, o patternOpts, seeHidden bool, ignore []string) []string {
	entries, err := r.readDir(dir)
	if err != nil {
		return nil
	}
	hidden := seeHidden || patternBeginsWithPeriod(pattern, o.group, o.quantified)
	// Whether `.` and `..` are in this dialect's listings at all, which is
	// what separates the two ways they reach the match below.
	listsDotAndDotDot := r.sem().GlobListsDotAndDotDot == Yes

	// Whether a unit is a character is a question about the subject as well
	// as the pattern, and here the subjects are the names in this directory —
	// `?` is ASCII and still has to consume a whole character of a filename.
	// Asked once for the listing rather than once per name, so a directory
	// full of them earns one diagnostic from the core rather than one each.
	o.chars = r.patternMatchCountsCharacters(pattern, entryNames(entries)...)

	var out []string
	for _, name := range r.globListingNames(entries, patternBeginsWithPeriod(pattern, o.group, o.quantified)) {
		// Only a *leading* period is special, and only in pathname
		// expansion: `*.b` matches `a.b`, and `.hid` needs `.*id`.
		if strings.HasPrefix(name, ".") && !hidden {
			continue
		}
		// And the other half of the rule, which is this name's rather than
		// the pattern's: the period has to be taken by a period the pattern
		// wrote, on the branch that matched. A pattern is offered a hidden
		// name because *some* place it could start writes one — the arm that
		// reaches this name still has to. See patternOpts.period.
		//
		// The switch that reveals hidden names lifts it, and for `.` and
		// `..` that depends on how the two got into the listing. Where the
		// dialect's listing simply holds them — GlobListsDotAndDotDot — they
		// are ordinary hidden names and the switch reveals them with the
		// rest: ksh93's `FIGNORE=x; echo *` lists `.` and `..`. Where they
		// are there only because *this pattern* wrote a leading period —
		// PeriodPatternListsDotAndDotDot — the switch does not reach them,
		// and that is measured on bash 5.3.20 with `globskipdots` off, where
		// `dotglob` makes `echo *` list `.a` and never `.`, and `@(.foo|*)`
		// lists `.a` through its star and still not `.`.
		no := o
		no.period = strings.HasPrefix(name, ".") &&
			(!seeHidden || (!listsDotAndDotDot && (name == "." || name == "..")))
		if !matchPattern(pattern, name, no) {
			continue
		}
		// The names the dialect's ignore parameter takes back out, in the
		// reading where the subject is the entry rather than the word — so
		// the filter belongs to the *listing*, at every component of the
		// walk, and a directory it removes is a directory the walk never
		// descends into. See Runner.ignoredNamesFilterTheListing.
		//
		// Empty for the other reading and for every dialect with no such
		// parameter, and empty for a component that spelled a name rather
		// than describing one: a literal reaches the filesystem by a lookup
		// and not by a listing, which is measured — `FIGNORE='b.txt'` takes
		// nothing out of `*/b.txt` where it empties `d/*` of everything but
		// `.` and `..`.
		if len(ignore) > 0 && r.ignoredListedName(name, ignore) {
			continue
		}
		out = append(out, globJoin(dir, name))
	}
	return out
}

// globListingNames is the names a component match may reach in one directory,
//
// Named for its caller rather than for what it holds, because `listedNames`
// is taken: interp/producedlisting.go has a method of that name about a
// *parameter* listing, and two methods one word apart on the same receiver
// compile until the day they both exist. They did — #2834 and #2836 each
// went green against a `main` the other had not landed in yet, and the merge
// of the two did not build.
// which is not quite the names a directory holds: three of the six columns
// list `.` and `..` beside them and three do not.
//
// Measured 2026-09-14 in a directory holding `a.txt`, `.dot` and `sub`:
//
//	           echo .*            echo .*/
//	bash 5.3   .dot               .*/
//	zsh        .dot               no matches found
//	ksh93      . .. .dot          ../ ./
//	dash       . .. .dot          ../ ./
//	bash 3.2   . .. .dot          ../ ./
//
// So it is not the ignore parameter's doing and not a hidden-name option's
// either — it is what the listing holds, and the leading-period rule is what
// keeps the two names out of an ordinary `*`. The parameter reaches it only
// by turning that rule off: ksh93's `FIGNORE=x; echo *` lists `.` and `..`
// where bash's `GLOBIGNORE=x; echo *` never does, which is the row #2748 was
// filed on and is this axis rather than a second rule about the parameter.
//
// The names go in front, which is where a listing that holds them puts them
// and is invisible anyway: every column sorts what it matched.
//
// The component match only. A `..` the walk *descended into* would climb out
// of the tree and never stop, and no column does that — ksh93's `**` lists
// the tree below and nothing above it.
//
// The second route to the same two names is PeriodPatternListsDotAndDotDot,
// and it is an option rather than an axis because the one shell that has it
// switches it while it runs. It is asked only of a component the pattern
// wrote with a leading period, which the axis is not: the shells the axis
// holds have the names in the listing whatever the pattern says, and the
// leading-period rule is what keeps them out of an ordinary `*` there. See
// the option for the table that separates the two.
//
// The descent lists them too, at every level it reads, and that is the same
// answer rather than a second one: a listing is a listing, and the axis says
// what one holds. Measured 2026-09-18, with the ignore parameter set so the
// leading-period rule is off and the names are visible at all, in a tree
// holding `topf`, `p/pf`, `p/q/qf`, `p/q/w/leaf` and `p/q/w/q/deepq`, the
// column that lists them and crosses levels writes
//
//	[.][..][p][p/.][p/..][p/pf][p/q][p/q/.][p/q/..][p/q/qf]…[topf]
//
// where this walk wrote `[p][p/pf]…` until #3175. The trailing-slash form
// keeps them, since both are directories — `**/` is `[../][./][p/][p/../]…`
// — and a component *behind* the `**` never sees them, because neither name
// is a level the walk entered: `**/qf` is `[p/q/qf]` with the parameter set
// and without it. appendDescendants is where that half lives.
func (r *Runner) globListingNames(entries []os.DirEntry, periodPattern bool) []string {
	names := make([]string, 0, len(entries)+2)
	if r.sem().GlobListsDotAndDotDot == Yes ||
		(periodPattern && r.MatchOption(PeriodPatternListsDotAndDotDot)) {
		names = append(names, ".", "..")
	}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// globJoin appends one name to a directory the walk is holding, and — unlike
// filepath.Join — does not clean.
//
// Cleaning is what destroyed the spelling. `filepath.Join("<base>", ".")` is
// `<base>`, so the written form of a `.` or `..` component was gone at the
// first join, long before anything relativized; joining the component instead
// of matching it would have made `./cx/*` *match* and still answer `cx/ax`
// where all six columns answer `./cx/ax`. Both helpers that build a path have
// to agree about this — matchIn and appendDescendants — or a `**` descent
// quietly cleans back what the component walk kept.
//
// An uncleaned path is what the kernel resolves anyway, and it is the more
// faithful answer where `..` meets a symbolic link: `sym/../ax` resolves
// through the link, which is what every shell in the panel reports, rather
// than textually back to the link's own parent.
//
// A directory already ending in a separator is the one case worth a branch:
// the root, and a Dir written with a trailing slash.
func globJoin(dir, name string) string {
	if strings.HasSuffix(dir, "/") {
		return dir + name
	}
	return dir + "/" + name
}

// entryNames is the names of a directory listing, for the question above.
func entryNames(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// workDir is where relative paths start: r.Dir, or `.` when nothing set one.
//
// Never os.Getwd. The process's directory is one answer shared by every
// Runner in the program, and asking for it here is how two embedded shells
// end up fighting over one cwd — the PATH lesson again, this time for
// directories. `.` keeps everything relative and lets the operating system
// resolve each use against wherever the process is, which is the only
// reading of "no directory was handed in" that stays true when the process
// moves. A shell binary wants the absolute answer, and driver — the binary,
// where process-wide questions belong — seeds Dir at construction.
func (r *Runner) workDir() string {
	if r.Dir != "" {
		return r.Dir
	}
	return "."
}

// refuseUnmatchedPattern reports a pattern that matched no file and ends what
// the dialect says such a failure ends.
//
// Both routes into it — the axis and the option — arrive here so that the
// wording is written once. The wording itself is the dialect's, because the
// two shells that can reach this path spell the same complaint differently;
// Diagnostics.GlobNoMatch carries it and the fallback is the substrate's.
//
// The *scope* is deliberately not decided here either. A failed pathname
// expansion is a failed expansion, so it ends exactly what every other one
// ends — Semantics.FailedExpansionAbandonsTheLine — which is how one call
// site produces bash giving up the statement and going on to the next, and
// zsh stopping the script, with neither dialect named.
func (r *Runner) refuseUnmatchedPattern(pattern string) {
	r.diagf("%s\n", Wording(r.diag().GlobNoMatch, "no matches found: %s", pattern))
	r.failedExpansion()
}
