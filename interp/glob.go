// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"sort"
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

// markedByGlobEscape is the alphabet above, named because two readers need
// it: globEscape, which puts the marks on, and the value-backslash escaping,
// which asks whether a mark on one of these could change what a field means.
// A second spelling of the set is how the two would come apart.
//
// NUL is in it for valueBackslashMark's sake and for nothing else: marking a
// byte no pattern reads costs nothing, and it is what makes a bare mark
// unambiguous.
const markedByGlobEscape = "*?[\\<()|\x00" + extendedPatternMeta

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
			// A NUL that is *data*, marked so that a bare mark can only be the
			// one this function writes. See valueBackslashMark.
			b.WriteByte('\\')
			b.WriteByte(s[i])
			continue
		}
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) {
			b.WriteString(`\\`)
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
func rewriteValueBackslashes(field string, p ValueBackslashPolicy) string {
	if strings.IndexByte(field, valueBackslashMark) < 0 {
		return field
	}
	var b strings.Builder
	for i := 0; i < len(field); i++ {
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
	if strings.IndexByte(field, valueBackslashMark) < 0 {
		return field, true
	}
	quotes := rewriteValueBackslashes(field, ValueBackslashQuotesWhatFollows)
	disarms := rewriteValueBackslashes(field, ValueBackslashDisarmsWhatFollows)
	data := rewriteValueBackslashes(field, ValueBackslashIsData)
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
	if strings.IndexByte(s, valueBackslashMark) < 0 && strings.IndexByte(s, '\\') < 0 {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == valueBackslashMark {
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
func hasUnescapedMeta(s string, numericRange, patternGroup, extendedPattern, extendedOperators bool) bool {
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
			if closesBracket(s, i) {
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
	if r.sem().UnterminatedBracket == BracketBadPattern &&
		field != "[" && hasUnterminatedBracket(field) {
		// zsh rejects an unterminated bracket against the filesystem too,
		// with one exception it is worth stating because it is what keeps
		// `[ a = a ]` working: a field that is exactly `[` is left alone.
		// `a[` is not, so the rule is the whole field rather than where the
		// bracket sits in it.
		r.fatalPattern(field, 1)
		return nil, false
	}
	if !hasQuals && !hasUnescapedMeta(field, r.dialect().NumericRangePattern,
		r.dialect().PatternAlternation, r.dialect().ExtendedPattern,
		r.MatchOption(ExtendedPatternOperators)) &&
		(!r.dialect().PatternTopLevelAlternation || !hasUnescapedByte(field, '|')) {
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
		if r.ctl == controlExit {
			// The pattern was rejected while it was being read, and the
			// script is already being abandoned. Reporting a miss on top of
			// that says the pattern matched nothing, which is a different
			// and weaker claim than the one already made.
			r.globMissed = false
			return
		}
		if r.globMissed && r.ask(r.sem().GlobNoMatchIsError, "an unmatched pattern being an error") &&
			!r.MatchOption(UnmatchedPatternIsEmpty) {
			// An error, which in zsh means the command does not run and the
			// script stops. Reporting it and then passing the pattern
			// through was the same report-then-continue bug as the others.
			//
			// Deleting the word wins over complaining about it, which is the
			// only ordering the two settings can have: measured, `setopt
			// nullglob; echo "[" zz* "]"` prints `[ ]` at 0 in a zsh where
			// nomatch is still on. The axis is still asked, so a dialect
			// that answered nothing about it is still told so.
			r.fatal("no matches found: %s\n", globUnescape(whole))
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
	seeHidden := r.MatchOption(PatternsMatchHidden) || quals.seeHidden
	starstar := r.MatchOption(StarStarCrossesDirectories)
	starstarAlone := r.MatchOption(StarStarAloneCrossesDirectories)
	parts := strings.Split(field, "/")

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

	// An absolute pattern starts at the root; a relative one at the working
	// directory, which is the shell's rather than the process's.
	base := r.workDir()
	dirs := []string{base}
	prefix := ""
	if parts[0] == "" {
		dirs, prefix, parts = []string{"/"}, "/", parts[1:]
	}

	// The directories `**` matched zero levels deep, when it was the last
	// component: the one shell with the option reports those with a trailing
	// slash — `d/**` lists `d/` ahead of what is inside it.
	var selfDirs map[string]bool
	crossed := false

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
			crossed = true
			last := lastComponent(parts, i)
			onward = map[string]bool{}
			for _, dir := range dirs {
				next = append(next, dir)
				// The directory the component starts from is always one the
				// walk may go on from, however it was reached: measured,
				// `s/**/x` through a symlink `s` is `s/x` in bash 5.3.15 and
				// in zsh. The rule bounds where a `**` **descends to**, not
				// where a pattern says to begin.
				onward[dir] = true
				if last {
					if selfDirs == nil {
						selfDirs = map[string]bool{}
					}
					selfDirs[dir] = true
				}
				next = r.appendDescendants(next, dir, seeHidden, onward)
			}
			sortMatches(next)
			next = compactSorted(next)
		} else if lit := globUnescape(part); lit == "." || lit == ".." {
			// `.` and `..` **name** a directory rather than describe one, so
			// this component is joined and never matched. No listing reports
			// either name — Go's ReadDir does not, and neither does any
			// shell's — so matching it against one answers nothing, which is
			// why the whole pattern used to be a miss. Measured unanimous
			// across the six: `./cx/*` is `./cx/ax`, `cx/./*` is `cx/./ax`,
			// `cx/../*` is `cx/../ax`, and `*/..` is `cx/..`.
			//
			// The literal behind the quoting marks is what is tested,
			// because quoting a component does not change what it names:
			// `"."/cx/*` and `\./cx/*` both list `./cx/ax` in all six. Today
			// the two spellings coincide — globEscape marks only the
			// metacharacters, and a period is not one — so this normalizing
			// is defensive rather than load-bearing, and a mutation that
			// drops it survives. It is written against the literal so that
			// it stays right if that set ever grows.
			//
			// Nothing here checks that the join exists, and nothing needs to.
			// Every directory standing at this point came out of a listing or
			// through the descent gate below, so `dir/.` and `dir/..` both
			// do. That gate is also the reason `ax/./*` is a miss in all six
			// and stays one here: `ax` is a file, and it is dropped before
			// this component is reached.
			for _, dir := range dirs {
				next = append(next, globJoin(dir, lit))
			}
		} else {
			o := r.patternOpts(part)
			o.fold = r.MatchOption(GlobFoldsCase)
			for _, dir := range dirs {
				next = append(next, r.matchIn(dir, part, o, seeHidden)...)
			}
		}
		if len(next) == 0 {
			return missed()
		}
		sortMatches(next)
		dirs = next
		if i < len(parts)-1 {
			// Only directories can be descended into.
			// Through the gate, like every stat: a match the policy hides
			// is not descended into, the same as a match that is no
			// directory.
			//
			// A `**` component answers this itself, because the set it hands
			// on is not the set it matched: it walked without following a
			// symbolic link, and a link it listed is not a level the next
			// component may look inside. Only where nothing real follows —
			// `**/`, where this filter is producing the answer rather than
			// choosing where to look next — does a link get through, and
			// only where the dialect says it is one of the levels.
			sees := onward == nil ||
				(lastComponent(parts, i) && r.MatchOption(StarStarSeesLinkedDirectories))
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
		if self && trail == "" {
			// The zero-level `**` writes its own separator, and only when
			// the pattern did not already ask for one. `cx/**/` is
			// `cx/ cx/dx/` in the two shells that cross levels, not
			// `cx// cx/dx/`, so the two sources of a trailing slash are one
			// slash and not two.
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

	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		w, ok := render(d)
		if !ok {
			continue
		}
		out = append(out, w)
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
	sortMatches(out)
	if crossed {
		out = compactSorted(out)
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
		if matchPattern(x, word, o) {
			return true
		}
	}
	return false
}

// lastComponent reports whether nothing but trailing slashes follows parts[i].
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
func (r *Runner) appendDescendants(out []string, dir string, seeHidden bool, onward map[string]bool) []string {
	entries, err := r.readDir(dir)
	if err != nil {
		return out
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

// compactSorted removes adjacent duplicates, which is all the duplicates a
// sorted list has. Only `**` can produce one: two components can expand to
// the same directory by different routes.
func compactSorted(names []string) []string {
	out := names[:0]
	for i, n := range names {
		if i > 0 && n == names[i-1] {
			continue
		}
		out = append(out, n)
	}
	return out
}

// sortMatches puts a glob's matches in order, which is byte order — and that
// is a decision rather than the absence of one.
//
// **In the C locale it is unanimous.** All four shells give
// `1digit Apple Cherry _under banana date` for a directory holding those
// names, on macOS and on Linux alike, and that is what this produces. Both
// sweeps here run under LC_ALL=C, so it is also the only ordering the corpus
// can record.
//
// **Outside it, three of the four collate and dash never does.** That much is
// a clean axis. What cannot be done is the collation itself, and the reason
// is worth keeping next to the code so it is not attempted again:
//
//   - The platforms disagree. Same shells, same locale name, opposite
//     answers: macOS gives `_under 1digit Apple banana Cherry date` and glibc
//     gives `1digit Apple banana Cherry date _under`. No single table is
//     right on both.
//   - A dependency does not settle it. golang.org/x/text/collate implements
//     CLDR, which is close to glibc and not to macOS — so taking this
//     library's first direct dependency would buy a third answer, and be
//     wrong on the platform the panel is measured on.
//   - An approximation is not close enough, and this was tried rather than
//     assumed. "Digits before letters, letters case-insensitively" gets the
//     obvious cases right and is still wrong twice over on an ordinary
//     directory: macOS orders `_` before `-`, which needs the real
//     punctuation weights, and sorts `Ápple` next to `Apple` and `éclair`
//     next to `date`, which needs base-letter folding. Both come from the
//     full table and neither can be derived from what the standard library
//     ships.
//
// So the shell sorts by byte, which is right in the C locale, right for one
// dialect everywhere, and wrong for three outside it — knowingly, and in a
// place that says so.
func sortMatches(names []string) { sort.Strings(names) }

// matchIn lists the entries of dir matching one pattern component. seeHidden
// lifts the leading-period rule, which is the run-time option's doing and not
// the pattern's. A method so the listing passes the gate; a denied directory
// matches nothing, as an unreadable one does.
func (r *Runner) matchIn(dir, pattern string, o patternOpts, seeHidden bool) []string {
	entries, err := r.readDir(dir)
	if err != nil {
		return nil
	}
	hidden := seeHidden || patternBeginsWithPeriod(pattern, o.group)

	// Whether a unit is a character is a question about the subject as well
	// as the pattern, and here the subjects are the names in this directory —
	// `?` is ASCII and still has to consume a whole character of a filename.
	// Asked once for the listing rather than once per name, so a directory
	// full of them earns one diagnostic from the core rather than one each.
	o.chars = r.patternCountsCharacters(append(entryNames(entries), pattern)...)

	var out []string
	for _, e := range entries {
		name := e.Name()
		// Only a *leading* period is special, and only in pathname
		// expansion: `*.b` matches `a.b`, and `.hid` needs `.*id`.
		if strings.HasPrefix(name, ".") && !hidden {
			continue
		}
		if matchPattern(pattern, name, o) {
			out = append(out, globJoin(dir, name))
		}
	}
	return out
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
