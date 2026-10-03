// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/blairham/sh/syntax"
)

// patternMeta is the set of characters that mean something in a pattern, and
// so the set escaping has any effect on. Escaping is what carries "this text
// was quoted, match it literally" into the matcher; a string holding none of
// these escapes to itself.
const patternMeta = `*?[\()<|`

// extendedPatternMeta is the three characters that become metacharacters only
// once [ExtendedPatternOperators] is on: the closure, the exclusion and the
// negation. They are a set of their own because the question they answer is a
// run-time one — with the option off they are ordinary text, and asking a
// dialect about globbing a `#` it has never heard of would refuse a pattern
// that has nothing wrong with it.
const extendedPatternMeta = "#~^"

// extendedGlobTrigger is the subset of those three that makes a *field* a
// pattern in the first place, and the exclusion is not in it.
//
// Measured on zsh 5.9.2 with `extendedglob` on, in a directory holding one
// file called `keep_a`: `print -l -- keep_a~zzz` prints those eleven
// characters, where `print -l -- keep#_a~zzz` prints `keep_a` and `^zzz`
// lists the directory. So a `~` on its own never sends a word to the
// filesystem — it only says what to take out of a search something else
// started — while a closure or a negation does.
//
// It is a separate set from [extendedPatternMeta] rather than a smaller one
// because the two questions differ: what has to be escaped when an expansion
// is pasted into a field, and what makes a field worth globbing. A `~` still
// belongs to the first, since an unescaped one in an expanded word would
// otherwise take matches out of a pattern the script never wrote.
const extendedGlobTrigger = "#^"

// bracketMeta is the characters that mean something only *inside* a bracket
// expression: the terminator, the range operator and the portable negation.
// Outside one they are ordinary text, which is why they are not in
// patternMeta — counting them there would send every expansion holding a dash
// to the axis that decides whether an expansion's result globs.
//
// They are escaped by escapePatternMeta all the same, because that is the one
// channel quoting has: a member the source quoted has to arrive at the matcher
// still marked, and inside a bracket expression the mark is all that separates
// `[a"-"z]` — three members — from `[a-z]`, a range. Measured 2026-09-07: all
// six panel shells answer a, a dash and z, and this implementation answered
// the range, having spent the quoting before the matcher could see it.
//
// `^` needs no entry: extendedPatternMeta already escapes it unconditionally.
const bracketMeta = "-]!"

// escapePatternMeta marks every metacharacter in text as ordinary.
//
// The parentheses are in the set because they are metacharacters where the
// dialect reads groups: without escaping them a `(b)` arriving from a variable
// became a group in the shell that does not re-read an expansion as a pattern,
// so `case b in $p` matched.
//
// `<` joined them for the dialect with numeric ranges, where a quoted `"<->"`
// is four ordinary characters and an expanded one is too: measured,
// `p="<->"; [[ 1 = $p ]]` does not match in zsh.
//
// `#`, `~` and `^` are marked here too, from extendedPatternMeta, and
// unconditionally: quoted text is literal in every dialect, and an escaped
// ordinary character is that character, so marking one where nothing reads
// it costs nothing for the reason the paragraph below gives. Where the
// option *is* asked is the other direction — see expansionPattern, which
// decides whether a metacharacter that arrived from a value stays live.
//
// They are escaped even where they are *not* metacharacters, and that is not
// an oversight: an escaped ordinary character is that character, so the two
// spellings match the same text and no test could tell a guard here from its
// absence.
func escapePatternMeta(text string) string { return escapePatternMetaIn(text, everyDialectsMeta) }

// everyDialectsMeta is the mark set above: every character that means
// something in a pattern in *some* dialect, plus the three that mean
// something only inside a bracket expression.
const everyDialectsMeta = patternMeta + extendedPatternMeta + bracketMeta

// escapePatternMetaIn is escapePatternMeta over a named set.
func escapePatternMetaIn(text, meta string) string {
	if !strings.ContainsAny(text, meta) {
		return text
	}
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		if strings.IndexByte(meta, text[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(text[i])
	}
	return b.String()
}

// markedMeta is the set escapePatternMeta marks for *this* dialect, and it is
// the wide one in four columns out of five.
//
// The paragraph above escapePatternMeta says a surplus mark costs nothing,
// because an escaped ordinary character is that character and no test could
// tell the guard from its absence. That is true under two of the three
// readings of [Semantics.BracketEscape] and **false under the third**: where
// the backslash is a member of the set rather than protection for what
// follows, every surplus mark is a surplus member, and a quoted `(` inside a
// bracket would admit a backslash BusyBox never puts there.
//
// So the set narrows exactly where it becomes visible, to what this dialect
// itself reads: the three that are metacharacters everywhere, the backslash,
// the three that mean something inside a bracket, and the negating caret;
// groups, numeric ranges, a top-level bar and the extended operators join it
// only in the dialects that have them. Measured 2026-09-16 against BusyBox
// ash 1.37.0 a character at a time — `case 'a\c' in a[\X]c)` for 37 values of
// X — and the characters it keeps a mark before are `* ? [ ] \ ! ^ -` and no
// others: `( ) | < ~ #` are ordinary there, and this shell has none of the
// four constructs that would make them anything else (#3271).
func (r *Runner) markedMeta() string {
	if r.sem().BracketEscape != BracketEscapeIsOnlyAMember {
		return everyDialectsMeta
	}
	d := r.dialect()
	meta := `*?[\` + bracketMeta + "^"
	if d.PatternAlternation || d.ExtendedPattern || d.ExtendedPatternInCondition {
		meta += "()"
	}
	if d.NumericRangePattern {
		meta += "<"
	}
	if d.PatternTopLevelAlternation.ReadsATopLevelBar(false) {
		meta += "|"
	}
	if d.ExtendedPattern || d.ExtendedPatternInCondition {
		meta += "#~"
	}
	return meta
}

// patternOf renders a word as a pattern, expanding it and escaping the parts
// that were quoted.
//
// docs/spec/grammar/patterns.md: quoting decides whether text is a pattern at
// all. `$p` matches as a pattern where `"$p"` matches as a literal, so the
// matcher cannot be handed a plain string — it has to be told which characters
// were quoted, and escaping them here is how that is carried.
//
// **A pattern operand is expanded like any other word.** It expanded only its
// parameters for a while, and every other substitution in one reached the
// matcher as its own source text: `v=abcd; echo ${v#$(echo ab)}` answered
// `abcd` where all six panel shells answer `cd`, because the pattern was the
// five characters `echo ab`. That is the failure mode this repository exists
// to avoid — a plausible wrong string and no diagnostic — and it reached
// `case` and `[[ ]]` too, which share this function (#882).
func (r *Runner) patternOf(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	// Before the spans are read, as in every other entry point: a pattern is
	// a word and the run may divide it differently from the parse. See
	// wordForRun.
	w = r.wordForRun(w)
	// A pattern is a word of its own, and the diagnostics raised inside it
	// have to say so. The operand of `#`, `%` or `/` is reached from within
	// the word that holds the expansion, and without this the run around a
	// failure was measured from the *outer* word: `"${v#${BAD}}"` blamed all
	// of `${v#${BAD}}` where bash 5.3 blames `${BAD}` alone, and
	// `"pre${v#${BAD}}post"` dragged in the literal text on both sides
	// (#1064). The value operands — `:-`, `:=` — already scoped themselves
	// through wordTextUnsplit; this is the same promise for the pattern ones,
	// which `case` and `[[ ]]` share.
	defer r.inWord(w)()
	var b strings.Builder
	// Where a value's text landed in the pattern, which is the only thing
	// that separates a live `|` from a written one. See markWrittenBars.
	var fromValue [][2]int
	spans := r.patternTilde(w, &b)
	// Which regular-expression flavor the word's backslashes belong to, if
	// any: they are the expression's there rather than this matcher's. See
	// tildeKeepsBackslash for which of them the reading reaches.
	flavor := r.tildeRegexFlavor(spans)
	// Whether the word carries a `~(K)` group at all, which is what makes a
	// written `\d` worth keeping the backslash on. **Where** the group
	// stands is the walk's question and not this one — the reading is
	// positional, and the walk turns it on as it consumes the group — so
	// this only narrows which words are touched: a word with no such group
	// reads every escape exactly as it did. See kshClassEscapes.
	classes := tildeGlobClasses(spans, r.lang().TildeGroup)
	for i, s := range spans {
		r.expandingSpan = i
		if tildeKeepsBackslash(flavor, s) {
			b.WriteByte('\\')
			b.WriteString(s.Value)
			continue
		}
		if classes && kshClassEscapeSpan(s) {
			// The backslash is the glob's own here, so the pair reaches the
			// matcher as it was written rather than as the letter quote
			// removal would otherwise take.
			b.WriteByte('\\')
			b.WriteString(s.Value)
			continue
		}
		// A pattern operand reads its spans the way a word does, and the
		// readers of a subscript in one are the same readers. Held here so a
		// `case` arm or a trim's pattern costs one run of a substitution
		// written into brackets in it rather than one per reader — eight,
		// measured, against the panel's one. See subscriptSubstHold (#3240).
		release := r.armSubscriptSubsts(s)
		text, live := r.patternSpan(s)
		release()
		if live {
			if s.Kind != syntax.Literal {
				fromValue = append(fromValue, [2]int{b.Len(), b.Len() + len(text)})
			}
			b.WriteString(text)
			continue
		}
		// Quoted text, and the result of an expansion the dialect does not
		// re-read as a pattern, are literal: every metacharacter is escaped.
		b.WriteString(escapePatternMetaIn(text, r.markedMeta()))
	}
	return r.markWrittenBars(b.String(), fromValue)
}

// markWrittenBars escapes every top-level `|` that the script *wrote*, leaving
// the ones that arrived in a value live. valueAt names the byte ranges of
// pattern the values contributed.
//
// A top-level bar is an alternation of the whole pattern only where it arrived
// live, and the source is the whole of what decides it. Measured on zsh 5.9.2,
// 2026-09-12, with `v=abc`:
//
//	L='a|ab'; ${v#a|ab}                  abc — written, so an ordinary character
//	L='a|ab'; ${v#${~L}}                 bc  — live, so an alternation
//	L='a|ab'; ${v#$L}                    abc — not live without the flag
//	setopt globsubst; ${v#$L}            bc  — the option is the same answer
//	setopt globsubst; ${v#a|ab}          abc — and does not reach a written bar
//	${v#(a|ab)}                          bc  — a written *group* still splits
//	w='a|b'; ${w#a|b}                    ''  — the written bar matches itself
//
// The last two rows are why this cannot be done by escaping every written bar:
// inside a group the bar is the group's own separator, and it is written there
// in the one spelling zsh does read. So the walk is topAlternatives' walk —
// past a group, past a bracket expression, past an escape — and only a bar
// standing at depth zero is asked where it came from.
//
// A live bar splits the *whole* pattern and not only the value it came in,
// which is measured rather than assumed: with `N='x|abc'`, `${v#a${~N}}` is
// empty, so the arms are `ax` and `abc` rather than `a` followed by a group;
// and with `I='ab|x'`, `${v#${~I}z}` is `c`, so the written `z` joined the
// second arm. Concatenation would have answered `abc` to both.
//
// It runs only where the dialect reads a top-level bar at all. Elsewhere the
// character is ordinary already, and an escape would be a difference nothing
// could observe.
func (r *Runner) markWrittenBars(pattern string, valueAt [][2]int) string {
	if !strings.Contains(pattern, "|") || !r.lang().PatternTopLevelAlternation.ReadsATopLevelBar(false) {
		return pattern
	}
	// A dialect that reads a written bar has no provenance rule to arrange,
	// so there is nothing to escape: ksh93 answers `bc` to `${v#a|ab}` and
	// to `${v#$L}` alike (#2528).
	if r.lang().PatternTopLevelAlternation.ReadsAWrittenBar() {
		return pattern
	}
	written := func(i int) bool {
		for _, v := range valueAt {
			if i >= v[0] && i < v[1] {
				return false
			}
		}
		return true
	}
	var b strings.Builder
	depth, last := 0, 0
	// Where a bare parenthesis is text (patternOpts.bareParenIsText), a bar
	// inside one stands at the top level, and there the provenance rule is
	// the other way round: a bar the script wrote divides the pattern and
	// one a value supplied is a character. Measured on zsh 5.9.2 with
	// `shglob` and `kshglob`: `[[ 'a(b' == a(b|c) ]]` matches and, with
	// `L='a(b|c)'`, `[[ 'a(b' == ${~L} ]]` does not (#5467).
	bare := r.lang().BarePatternGroupInsideAWord && !r.lang().PatternAlternation
	var counted []bool
	bareDepth := 0
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i++
		case '(':
			opens := !bare || (i > 0 && strings.IndexByte("@?*+!", pattern[i-1]) >= 0)
			counted = append(counted, opens)
			if opens {
				depth++
			} else {
				bareDepth++
			}
		case ')':
			if n := len(counted); n > 0 {
				if counted[n-1] {
					depth--
				} else {
					bareDepth--
				}
				counted = counted[:n-1]
			} else {
				depth--
			}
		case '[':
			if end, ok := bracketEnd(pattern, i, r.emptyBracketCompiles()); ok {
				i = end
			}
		case '|':
			if depth != 0 {
				continue
			}
			if bareDepth > 0 {
				if written(i) {
					continue
				}
			} else if !written(i) && r.lang().PatternAlternation {
				// A value's bar divides the pattern only where groups do:
				// measured on zsh 5.9.2, `L='a|b'; [[ b == ${~L} ]]` matches,
				// and does not once `shglob` is on, with or without
				// `kshglob` (#5467).
				continue
			}
			b.WriteString(pattern[last:i])
			b.WriteString(`\|`)
			last = i + 1
		}
	}
	if last == 0 {
		return pattern
	}
	b.WriteString(pattern[last:])
	return b.String()
}

// tildeRegexFlavor reports which regular-expression language these pattern
// spans open in, and is tildeGlob for spans that open in none.
//
// Read off the **source** rather than off the finished pattern, because the
// thing it decides has to be decided while a backslash is still a backslash:
// quote removal turns a written `\1` into the span `1`, and by the time the
// pattern is a string the two are the same text. The group is literal and
// unquoted, so the first span holds all of it.
// See tildeKeepsBackslash and #3894.
//
// **The group need not be at the front of the word**, since one where it
// stands settles the language just as much: `[[ za1b == z~(P)a\db ]]` matches
// in ksh93u+ and its backslash belongs to the expression, not to the shell.
// So this asks findTildeFlavorGroup rather than reading a head group alone —
// which also means a head group that names no flavor, `~(i)` or `~(K)`, does
// not spend the one reading a later group could have used.
func (r *Runner) tildeRegexFlavor(spans []syntax.Span) tildeFlavor {
	if !r.lang().TildeGroup || len(spans) == 0 {
		return tildeGlob
	}
	s := spans[0]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
		return tildeGlob
	}
	_, body, _, ok := findTildeFlavorGroup(s.Value)
	if !ok {
		// No group here names a language, which covers a group holding a
		// letter this shell declines as well: that one is refused by name in
		// Runner.tildeModifierOpts rather than guessed at here.
		return tildeGlob
	}
	m, _ := readTildeModifier(body)
	return m.flavor
}

// tildeKeepsBackslash reports whether this span is a quoting backslash that
// belongs to the regular expression rather than to the shell — one to write
// back into the pattern rather than to let quote removal take.
//
// The span kind is the whole of the reading: a backslash written in front of
// a character is its own [syntax.Quoting] rather than text, which is what
// makes the construct visible here and invisible one step later.
//
// **The reference shell keeps a different set per flavor, and that is
// measured rather than tidied.** ksh93u+ 2012-08-01, 2026-09-20, each row
// with the control that separates the two readings:
//
//	[[ aXb == ~(E)a\.b ]]        no      so `E` keeps the backslash
//	[[ aXb == ~(G)a\.b ]]        yes     and `G` does not
//	[[ abcd == ~(E)\(ab\)cd ]]   no      `E` keeps it here too
//	[[ abc == ~(G)a\(b\)c ]]     yes     and `G` keeps this one
//	[[ aab == ~(G)a\+b ]]        no      while dropping this one
//	[[ ab == ~(G)ax\?b ]]        yes     and keeping this one
//
// So the extended flavors keep every backslash there and the basic ones keep
// only some. What is modeled is narrower than either, and deliberately:
//
//   - For the extended flavors, the digits and the letters in
//     tildeExtendedEscapes. A `\1` to `\9` keeps its backslash because
//     dropping it turns the pattern into the perfectly ordinary `(ab)1` and
//     hides from unsupportedERE the one construct this shell has to name.
//     The letters keep theirs because the two engines read them the same
//     way; the ones left out are the ones where they do not, and they are
//     left to quote removal exactly as they were.
//   - For `X` the ampersand as well, because there it is an **operator**:
//     dropping the backslash would turn the literal `a\&b` into the
//     conjunction `a&b`, which is a wrong answer rather than a narrower one.
//     Measured, `[[ "a&b" == ~(X)a\&b ]]` matches and `[[ ab == … ]]` does
//     not. See tildeConjunction, which undoes the spelling for the engine.
//   - For the **basic** flavors, the characters the rows above and their
//     siblings show kept — `(`, `)`, `|`, `?`, `*`, `[`, `^`, `<`, `>` and
//     the digits — and no others. Those are the ones whose backslash decides
//     whether the character is an operator, so dropping one changes the
//     expression rather than narrowing it; `.`, `+`, `{` and `}` are
//     measured **dropped** there and are dropped here for the same reason
//     the digits are kept, which is that the reference is what is being
//     reproduced.
func tildeKeepsBackslash(flavor tildeFlavor, s syntax.Span) bool {
	if s.Kind != syntax.Literal || s.Quoting != syntax.BackslashQuoted ||
		len(s.Value) != 1 {
		return false
	}
	c := s.Value[0]
	if c >= '1' && c <= '9' {
		return flavor == tildeERE || flavor == tildeAugERE ||
			flavor == tildePerl || flavor == tildeBRE
	}
	switch flavor {
	case tildeERE, tildeAugERE, tildePerl:
		// The ampersand is `X`'s operator and is its own row above; the
		// letters are the ones the two engines agree about.
		return (flavor == tildeAugERE && c == '&') ||
			strings.IndexByte(tildeExtendedEscapes, c) >= 0
	case tildeBRE:
		return strings.IndexByte(`()|?*[^<>`, c) >= 0
	}
	return false
}

// tildeExtendedEscapes are the letters whose backslash an **extended** flavor
// keeps — `E`, `X` and `P` — because ksh93's engine and Go's `regexp` read
// the pair the same way.
//
// This is the set that used to be empty, and the emptiness was a silent wrong
// answer in **both** directions: a written `\d` lost its backslash to quote
// removal and became the ordinary letter, so `[[ za1b == ~(E)za\db ]]` was a
// quiet no where ksh93u+ matches, and `[[ zadb == ~(E)za\db ]]` was a quiet
// *yes* where ksh93u+ does not. #4894 reported the first half and its stated
// cause — that RE2 has no `\d` — is not the case: Go's `regexp` has the Perl
// classes and reads them exactly as that shell does. The backslash simply
// never reached it.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-27, `-c` under `env -i` with a
// scratch `HOME`, each letter with the case that separates the class reading
// from the literal one:
//
//	\d  [[ za1b == ~(E)za\db ]] yes    [[ zadb == … ]] no
//	\D  [[ zaXb == ~(E)za\Db ]] yes    [[ za1b == … ]] no
//	\w  [[ za1b == ~(E)za\wb ]] yes    [[ za.b == … ]] no
//	\W  [[ za.b == ~(E)za\Wb ]] yes    [[ za1b == … ]] no
//	\s  [[ 'za b' == ~(E)za\sb ]] yes  [[ zasb == … ]] no
//	\S  [[ za1b == ~(E)za\Sb ]] yes    [[ 'za b' == … ]] no
//	\t  [[ $'x\ty' == ~(E)x\ty ]] yes  [[ xty == … ]] no
//	\n \r \f \v \a   the same pair each, the control character and not
//	                 the letter
//	\b  [[ xy == ~(E)\bxy ]] yes       [[ axy == ~(E)a\bxy ]] no
//	\B  [[ xy == ~(E)x\By ]] yes       [[ 'x y' == ~(E)x\B ]] no
//	\A  [[ xy == ~(E)\Axy\z ]] yes     [[ axy == ~(E)\Axy ]] no
//	\z  the same pair, and `[[ xzy == ~(E)x\zy ]]` is no
//
// Every one of those is what Go's `regexp` answers too, which is why the
// letter is here rather than translated.
//
// **What is left out is measured as a disagreement rather than forgotten**,
// and each stays exactly as it was — the backslash goes and the letter
// stands for itself:
//
//	\Z  an end anchor there, and Go has `\z` only
//	\e  the escape character there, and Go has no such escape
//	\c  \C  \x  \E  each something in that engine and either absent from
//	                Go's or spelled with an argument it would have to be
//	                given
//
// A **basic** flavor is not in this set at all, which is measured too:
// `[[ za1b == ~(G)za\db ]]` does not match in ksh93u+, so `G` and `V` keep
// the characters breToRE2's own table names and nothing else.
const tildeExtendedEscapes = "dDsSwWbBAzafnrtv"

// patternTilde expands a leading tilde into the builder and gives back the
// spans still to be read as a pattern.
//
// **A tilde is expanded before the word becomes a pattern**, which is
// unanimous across the panel and was missing here entirely:
//
//	h=$HOME; [[ $h == ~ ]]            true in zsh, bash, bash 3.2, ksh93
//	case $HOME in ~) …                taken in all six
//	x=$HOME/sub; echo "${x#~}"        `/sub` in all six
//
// It belongs here rather than at `case` and `[[ ]]` and each trim, because
// this is the one function that turns a word into a pattern — the same reason
// #882 moved the rest of the expansion here. A tilde written anywhere but the
// front of an unquoted word is ordinary text and never reached this.
//
// What the directory is worth once it is in the pattern is the other half:
// zsh matches it as text, so this writes it escaped. See tildeSplit for the
// measurement, and docs/spec/grammar/patterns.md for the panel's three
// answers to a home directory that holds a metacharacter — a split entangled
// with #1367, which is why only the tail is left live here.
//
// The spans come back rewritten rather than the word being edited: patternOf
// is handed the syntax tree, and a `case` inside a loop reads the same arm on
// every pass.
func (r *Runner) patternTilde(w *syntax.Word, b *strings.Builder) []syntax.Span {
	if len(w.Spans) == 0 {
		return w.Spans
	}
	s := w.Spans[0]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted ||
		!strings.HasPrefix(s.Value, "~") {
		return w.Spans
	}
	// The same question a word asks, asked here through the same function for
	// the same reason the rest of this function exists: a `case` arm and a
	// `[[ ]]` operand are words, and a rule written into one road only is a
	// shell no column has. That includes the colon —
	// `case /usr/xyz:x in ~:x)` matches in bash and ksh93 and not in the other
	// three — so this takes wordTildeHead rather than naming a set. See
	// Semantics.TildeColonEndsAnOrdinaryWordsPrefix.
	h := r.wordTildeHead(w.Spans)
	if r.refuseTilde(h.miss) || !h.moved {
		return w.Spans
	}
	b.WriteString(escapePatternMeta(h.dir))
	spans := slices.Clone(w.Spans)
	// The directory has gone into the builder already, so what the prefix
	// occupied is replaced by nothing rather than by it.
	tildeHead{moved: true, span: h.span, off: h.off}.apply(spans, 0)
	return spans
}

// patternSpan expands one span of a pattern, reporting whether the
// metacharacters in what comes back are live — a pattern — or ordinary text.
func (r *Runner) patternSpan(s syntax.Span) (text string, live bool) {
	switch s.Kind {
	case syntax.ParamExp:
		// A group that marks its join separator answers for itself: the
		// words it joined are literal and the separator between them is not,
		// which is one string neither of the two answers below can be. See
		// interp/tildeflaggroup.go.
		if text, live, ok := r.markedSeparatorPattern(s); ok {
			return text, live
		}
		// An unbraced subscript the run does not read as one is the
		// parameter followed by the brackets as text — here as much as in a
		// word, since a `case` arm and a condition read the same spans. See
		// baresubscript.go.
		s, tail := r.unreadBareSubscript(r.indirectionReadAsTheSubscriptFlag(s))
		// A `-` or `+` word that is what the expansion came to is part of
		// the *pattern* being built, not a value pasted into it. See
		// substitutedWordPattern.
		if tail == nil {
			if text, ok := r.substitutedWordPattern(s); ok {
				return text, true
			}
		}
		// The `${~spec}` flag reaches here too, and that is measured rather
		// than assumed: `p='a*'; [[ abc == ${~p} ]]` is true in the shell
		// that has the construct where `${p}` alone is false, and
		// `${v#${~p}}` trims where `${v#${p}}` does not. It is the same
		// question GlobExpansionResults answers, so it is the same override.
		savedLive, savedHead := r.liveMarksFor, r.liveMarksAtHead
		r.liveMarksFor, r.liveMarksAtHead = s.Param, true
		v := r.resolvePending(r.expandParam(s.Param))
		r.liveMarksFor, r.liveMarksAtHead = savedLive, savedHead
		var text string
		var live bool
		if strings.Contains(v, liveMark) {
			// A replacement's pattern characters stay live in a pattern
			// too: `s=xQy; [[ xay = ${s:s/Q/?/} ]]` matches. See liveMark.
			text, live = r.liveMarkedPattern(v), true
		} else {
			text, live = r.expansionPattern(v, s.Quoting, r.globSubstAnswer(s))
		}
		if tail == nil {
			return text, live
		}
		// Two provenances in one span now, and only one flag to report them
		// with: whatever the value was worth is settled here, and what comes
		// back is the finished pattern.
		if !live {
			text = escapePatternMetaIn(text, r.markedMeta())
		}
		return text + r.bareSubscriptPattern(tail), true
	case syntax.CommandSubst:
		// A pattern operand is text that never becomes a field, so the cut a
		// field gets at a substituted NUL is taken here, on the value — see
		// Runner.cutAtNul. Measured on ksh93u+: `v=abc; echo
		// "${v#$(printf 'a\0b')}"` is `bc`, the operand having ended at the
		// NUL.
		return r.expansionPattern(r.cutAtNul(r.commandSubst(r.ctx, s), false), s.Quoting, r.sem().GlobExpansionResults)
	case syntax.ArithSubst:
		v, ok := r.arithSpanValue(s)
		if !ok {
			return "", false
		}
		return r.expansionPattern(v, s.Quoting, r.sem().GlobExpansionResults)
	case syntax.ProcSubstIn, syntax.ProcSubstOut, syntax.ProcSubstFile:
		// Performed, like any other substitution in a word, and the *path*
		// is the pattern.
		//
		// Which is what makes it never match anything a script would write
		// down — that is the measured answer rather than a shortcut. It used
		// to hand the matcher the substitution's *inner* text, so
		// `case x in <(x))` matched, `${v#<(x)}` trimmed a bare `x`, and
		// neither is anything a shell in the panel does (#902).
		//
		// Whether a `<(` in this position opens a substitution at all is the
		// grammar's question and is answered before this: only bash reads
		// one inside a `${…}` operand, so in the other dialects a span of
		// this kind can only have come from a `case` arm or a condition,
		// where bash and zsh both perform it.
		path, ok := r.procSub(r.ctx, s)
		if !ok {
			return "", false
		}
		return path, false
	}
	if s.Quoting == syntax.DollarSingleQuoted {
		// `$'\t'` is a tab, and the lexer keeps both bytes so the source text
		// stays recoverable. Decoding it is this function's job as much as it
		// is expandSpan's: without it `v=$'\tx'; echo ${v#$'\t'}` kept the tab,
		// which is the same silent wrong answer wearing a different span.
		return r.expandDollarSingle(s.Value), false
	}
	// Unquoted literal text is a pattern; quoted literal text is not.
	return s.Value, s.Quoting == syntax.Unquoted
}

// substitutedWordPattern renders the word a `-` or `+` substituted as part of
// the pattern being built, and reports whether this expansion is one of those.
//
// **The word is source text, so it is a pattern, and it never reaches the
// filesystem.** Both halves of that were wrong, and both came from sending the
// word through the ordinary word pipeline — which matches against the
// filesystem and then hands back a value, the one shape a pattern operand can
// never be. Measured 2026-09-10 across bash 5.3.15, bash 3.2.57 and zsh 5.9.2:
//
//	r=a; [[ abc == ${r:+a*} ]]          matches in all three; this said no
//	r=a; case abc in ${r:+a*}) ;;       matches in all three; this said no
//	r=a; x=abc; printf '%s' ${x%${r:+b*}}   is `a` in all three; this said `abc`
//
// The listing is what the matcher was being handed: `a*` had already been
// replaced by the names it found, and those names were then escaped as a
// value. Where nothing matched it was worse than a wrong answer — in the
// dialect where an unmatched pattern is fatal the whole command stopped, which
// is how this was found, as one `no matches found: |shim-list` on a real
// startup where the shell it is measured against is silent (#1955).
//
// The word going back through patternOf rather than being escaped wholesale is
// what keeps the two provenances apart inside it: `${r:+"a*"}` is four
// ordinary characters and `${r:+$v}` with `v='a*'` is the same question
// GlobExpansionResults already answers for any other value — zsh reads it as
// text, bash as a pattern — measured in the same run.
//
// It answers false when the expansion is not a `-` or a `+`, or when the
// *parameter* is what it came to, both of which leave the caller on the
// ordinary path. The test for which it came to is testFires, the same one
// expandParam and substitutedWordFields apply, so the three cannot drift.
func (r *Runner) substitutedWordPattern(s syntax.Span) (string, bool) {
	e := s.Param
	if e == nil || e.Arg == nil || e.Length || e.Indirect {
		return "", false
	}
	if e.Op != syntax.ParamDefault && e.Op != syntax.ParamAlternate {
		return "", false
	}
	fires := r.testFires(e)
	if (e.Op == syntax.ParamDefault && !fires) ||
		(e.Op == syntax.ParamAlternate && fires) {
		return "", false
	}
	if s.Quoting != syntax.Unquoted {
		// Quoted, so the word substitutes as text and nothing in it is a
		// pattern — `[[ abc == "${r:+a*}" ]]` is false in all three. Still
		// not matched against the filesystem: it is the same word, read
		// under different quoting.
		return escapePatternMeta(r.substitutedWordText(e.Arg)), true
	}
	return r.patternOf(e.Arg), true
}

// expansionPattern applies the one axis that decides what the *result* of an
// expansion is worth in a pattern: whether its metacharacters stay live.
//
// Quoted, they never are — that is unanimous. Unquoted, it is the same
// question that decides whether `x="et*"; echo $x` globs, which is why the
// answer is asked for rather than assumed; escaping unconditionally made
// `p="a*"; [[ abc == $p ]]` fail.
//
// The axis is asked only where the two answers differ. A result holding no
// metacharacter escapes to itself, so `pat=ab; echo ${v#$pat}` — which every
// shell in the panel answers the same way — is answered rather than refused.
func (r *Runner) expansionPattern(v string, q syntax.Quoting, glob Answer) (string, bool) {
	if q != syntax.Unquoted {
		return v, false
	}
	if !strings.ContainsAny(v, r.patternMetaSet()) {
		return v, true
	}
	if !r.ask(glob, "globbing the result of an expansion") {
		return v, false
	}
	return r.bracketEscapeAsMember(v), true
}

// bracketEscapeAsMember rewrites a value about to be matched as a pattern so
// that the backslashes inside its bracket expressions are members of their
// sets as well as protection for the characters behind them — which is what
// [Semantics.BracketEscape] records at BracketEscapeProtectsAndIsAMember,
// and what one column does.
//
// It is spelled as a rewrite rather than as a flag the matcher reads because
// the matcher cannot tell the two provenances apart. A backslash reaching it
// inside a bracket expression is either one the *source* wrote — or one
// patternOf inserted to carry "this member was quoted", quoting having no
// other channel — and those are unanimously an escape and nothing more; or it
// is one that came out of a value, which is the only case this is about. The
// difference is visible here and nowhere downstream, so the answer is applied
// here, in the language the matcher already speaks: `\x` becomes `\\`, the
// backslash as a member, followed by `\x`, the member it protects.
//
// Off — every column but one — nothing is rewritten and the value reaches the
// matcher as it stands.
func (r *Runner) bracketEscapeAsMember(v string) string {
	if !hasBracketEscape(v, r.emptyBracketCompiles()) || r.bracketEscape() != BracketEscapeProtectsAndIsAMember {
		return v
	}
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		switch {
		case v[i] == '\\':
			// Outside a bracket expression the escape rule is the one
			// PatternEscapeReaches already carries, so this passes the pair
			// through untouched.
			b.WriteByte(v[i])
			if i+1 < len(v) {
				i++
				b.WriteByte(v[i])
			}
		case v[i] == '[' && closesBracket(v, i, r.emptyBracketCompiles()):
			i = writeBracketWithEscapesAsMembers(&b, v, i)
		default:
			b.WriteByte(v[i])
		}
	}
	return b.String()
}

// writeBracketWithEscapesAsMembers copies the bracket expression opened at i
// into b, doubling each backslash it holds, and returns the index of its
// closing `]`.
//
// The prologue is copied verbatim for the reason closesBracket has one: a `!`
// or `^` directly after the bracket negates, and a `]` directly after that is
// a member rather than the terminator.
func writeBracketWithEscapesAsMembers(b *strings.Builder, v string, i int) int {
	b.WriteByte(v[i])
	j := i + 1
	if j < len(v) && (v[j] == '!' || v[j] == '^') {
		b.WriteByte(v[j])
		j++
	}
	if j < len(v) && v[j] == ']' {
		b.WriteByte(v[j])
		j++
	}
	for ; j < len(v); j++ {
		if v[j] == '\\' {
			b.WriteString(`\\`)
			b.WriteByte('\\')
			if j+1 < len(v) {
				j++
				b.WriteByte(v[j])
			}
			continue
		}
		b.WriteByte(v[j])
		if v[j] == ']' {
			return j
		}
	}
	return j
}

// patternMetaSet is the characters that make the result of an expansion worth
// asking the axis about.
//
// The three extended ones are in it only while the option is on, and that is
// the measurement rather than caution: `p="a#b"; [[ ab == $p ]]` does not
// match in real zsh with `extendedglob` set, because a `#` that arrived from
// a value is not the closure operator — the same answer `p="a*"` gets there,
// and the reason `${~p}` exists. Counting it as a metacharacter is what routes
// it to the axis that says so.
func (r *Runner) patternMetaSet() string {
	if r.MatchOption(ExtendedPatternOperators) {
		return patternMeta + extendedPatternMeta
	}
	return patternMeta
}

// matchPattern reports whether pattern matches the whole of s.
//
// The language is the one in patterns.md and nothing more: `*`, `?` and
// bracket expressions. There is no alternation and no repetition, and no way
// to ask for a longer or shorter match — `${x#p}` and `${x##p}` choose that by
// doubling the *operator*.
//
// The restrictions that apply in pathname expansion — no metacharacter matches
// `/` or a leading period — are deliberately not here. They belong to the
// caller, because the same language is used by `case` and by parameter
// expansion where there is no filesystem and no components. Putting them here
// would be wrong in two places out of three.
// patternOpts carries the dialect's answers into the matcher, which has no
// Runner and should not need one. It grew from a single bool the moment a
// second axis reached the same code.
type patternOpts struct {
	caret bool
	// nth is which match a substring search takes, counting one per
	// starting position in the search's own direction: the `I` flag. Zero
	// and one both mean the first. See matchIndexFlag.
	nth int
	// tilde reads a `~(…)` prefix on the pattern, which is one dialect's
	// pattern-modifier group — see interp/tildemodifier.go. A grammar
	// answer, because the lexer has to have let the `(` into the word
	// before anything here can see it.
	tilde bool
	// tildePrefix is the glob standing in front of a `~(…)` flavor group,
	// already rendered as the regular expression that matches the same text.
	// It is carried rather than concatenated at the call site because every
	// flavor turns into one language in tildeRegex and nowhere earlier — a
	// literal flavor quotes its pattern, a basic one is translated, and an
	// extended one is taken as it stands, so the only place a prefix can be
	// joined to all three is after that. See interp/tildeflavorhere.go.
	tildePrefix string
	// tildeFold is the same construct read where it *stands* rather than at
	// the front, for the letters that are an option for the rest of the
	// branch. Separate from tilde because that one is cleared the moment a
	// prefix group has been read — so that a flavor falling back to this
	// matcher cannot loop on its own group — and a second group further
	// along the pattern still has to be found: `~(i)z~(-i)A` is two of them.
	// See interp/tildemidpattern.go.
	tildeFold bool
	// whole says the caller is asking whether the pattern matches a whole
	// subject, rather than choosing how much of one a match takes.
	//
	// Nothing but the `~(…)` flavors reads it, and they have to: ksh93's
	// regular expressions match a *substring* where its globs match the
	// whole string, so the same pattern answers a condition and a
	// substitution's span differently. Every other question in this matcher
	// is about the piece it was handed and cannot tell the two apart.
	whole bool
	// classEscapes reads a written `\d` as a **digit class** rather than as
	// the letter, and the five escapes beside it likewise. One dialect's
	// `~(K)` group asks for it and nothing else does — see kshClassEscapes
	// for the six and for why the letter rather than the language decides.
	classEscapes bool
	// pieceBase and pieceEnd are where the piece being matched begins and
	// ends in the subject, both absolute, so that a zero-width assertion can
	// ask about the piece's edges rather than the whole value's.
	//
	// **The piece and not the subject**, which is measured rather than
	// assumed and is the opposite of what it first looked like. On the
	// whole-subject surfaces the two are the same and no row can tell them
	// apart; a `%` trim is where they part, and ksh93 asks about the piece:
	// `v=aab; ${v%~(K)\bab}` is `a` there — the piece `ab` begins at a
	// position the *subject* has a word character in front of, so a
	// subject-relative boundary would not hold and nothing would be trimmed.
	// `v=ab; ${v%~(K)\Bb}` is the same fact read the other way: no trim,
	// because the piece's front is a boundary and `\B` wants one that is
	// not. See kshZeroWidthHolds.
	pieceBase, pieceEnd int
	// tildeGlobRead says a `~(K)` group is read **on this surface** at all.
	// Where it is false the group is ordinary characters and the pattern it
	// prefixes matches whatever that text matches, which is usually nothing.
	//
	// One dialect's, and the shape is a quirk rather than a design — it is
	// recorded because it is measured, not because it is explicable.
	// Measured 2026-09-28 against `/bin/ksh` `Version AJM 93u+ 2012-08-01`,
	// `-c` under `env -i PATH=/usr/bin:/bin` with a scratch `HOME`, `v=xab`:
	//
	//	${v#~(K)x}    xab   the group is **not** read
	//	${v##~(K)x}   xab
	//	${v%%~(K)b}   xab
	//	${v/~(K)x/Q}  xab
	//	${v%~(K)b}    xa    and on this one it **is**
	//	${v#x}        ab    the controls: the trims work
	//	${v%b}        xa
	//
	// So `%` is the one span-choosing operator that reads it, and `%%` — the
	// same anchor with the other length preference — does not.
	//
	// **It is the `K` letter and not the group.** `~(i)`, `~(E)` and `~(g)`
	// are each read on `#` and on `%` alike there, and this shell already
	// agrees on all six of those rows: `${v#~(i)x}` is `ab` and
	// `${v%~(i)b}` is `xa`. So nothing here may turn a tilde group off in
	// general, only the one letter.
	//
	// It also gates [patternOpts.classEscapes], which was gated on `whole`
	// while pathname expansion and `%` were the two surfaces that read the
	// group without reading its escapes.
	tildeGlobRead bool
	// tildeLeftUnread says the surface does not read the `~(l)` anchor at
	// all, so a piece that does not begin the subject is still a match.
	//
	// One surface asks for it and it is the *operator* that decides, not the
	// end the trim is pinned to: a **shortest** suffix trim ignores `l` and
	// the doubled spelling of the same trim honors it. Measured on ksh93u+
	// 2012-08-01, 2026-09-27, `v=abcd` unless another value is shown — see
	// trimSpan for the pairs and for what they rule out.
	tildeLeftUnread bool
	bracket         BracketPolicy
	// emptyBracket says a bracket the POSIX reading leaves unterminated is
	// re-read with the `]` written first as its terminator, so `[]` is a set
	// with no members and `[!]` matches any one character. See
	// Semantics.EmptyBracketExpressionCompiles, and memberReadingCloses in
	// glob.go, which is the scan this is asked after.
	emptyBracket bool
	// unknownClass is what a `[:name:]` the shell has never heard of does to
	// the bracket around it — see Semantics.UnknownCharacterClass. Read only
	// when a pattern actually holds one, so the zero value here is "no
	// bracket in this pattern asked".
	unknownClass UnknownClassPolicy
	// unterminatedClass is what a `[:` nothing closes does to the bracket
	// around it — see Semantics.UnterminatedCharacterClass. Read only when a
	// pattern actually holds one, so the zero value here is "no bracket in
	// this pattern asked".
	unterminatedClass UnterminatedClassPolicy
	// group says a parenthesised group in the pattern is a group rather than
	// literal parentheses, and quantified says a `@?+*!` in front of one is
	// its quantifier rather than an ordinary character.
	//
	// They are separate because the two dialects that have groups do not have
	// the same one: `a(b|c)` matches `ab` in the shell with bare groups, and
	// `@(abc|xyz)` is a literal `@` followed by a group there where the other
	// reads it as an extended pattern.
	group      bool
	quantified bool
	// counted says a `{n,m}` written in front of a group is its repetition
	// count rather than ordinary characters — one dialect's, and a third
	// reading beside the two above rather than a widening of either. See
	// splitCountedGroup and syntax.Dialect.CountedPatternGroup.
	counted bool
	// operandParens says a bare `(` opens a group on **this surface** even
	// where a count is what let it into the text, so noBareGroup is not
	// armed here.
	//
	// The parameter-expansion operand is that surface and it is the only
	// one. A written bare group at the top of a pattern is a syntax error
	// in a word and in a condition in the dialect that has counts —
	// `[[ ab == a(b) ]]` — and is read in an operand: `s=ab; ${s#a(b)}` is
	// empty on ksh93u+, where `s='a(b)'; ${s#a(b)}` is unchanged. So the
	// parenthesis has a door of its own there and does not need the count's.
	//
	// Measured 2026-09-27, and the pair is what says it is the surface
	// rather than the brace:
	//
	//	s='{z,y}a';    ${s#{z,y}(a)}   empty — the group is read
	//	s='{z,y}(a)';  ${s#{z,y}(a)}   unchanged
	//	[[ '{z,y}(a)' == {z,y}(a) ]]   matches — and there it is not
	//
	// The same text, the same brace, two surfaces and two answers.
	operandParens bool
	// noBareGroup says a `(` with no quantifier in front of it is an
	// ordinary character here, whatever `quantified` says.
	//
	// It is set once a counted group has been read and never cleared for
	// the rest of that branch, because a count is the **only** door a bare
	// parenthesis has into a word in the dialect that has one:
	// `[[ ab == a(b) ]]` is a syntax error on ksh93u+, so a written bare
	// group at the top of a pattern is not a thing that shell has, and
	// every parenthesis that arrives behind a count is text. Measured
	// 2026-09-27:
	//
	//	[[ '{z,y}(a(b))' == {z,y}(a(b))  ]]   matches — the inner one too
	//	[[ '{z,y}(ab)'   == {z,y}(a(b))  ]]   no
	//	[[ '{z,y}(ab)'   == {z,y}(a@(b)) ]]   matches — a *quantified* one
	//	                                      inside is still a group
	//
	// The arms of a group are the exception and matchGroupTimes clears it
	// for them, which is measured on both spellings: `@(a|(b))` matches `b`
	// in that shell and `{1}(a(b))` matches `ab`, so a bare group inside a
	// group is one however the group in front of it was written.
	//
	// The row that would say what happens **behind** the group —
	// `f {2,3}(a)(b)`, which is `1 | [{2,3}(a)(b)]` there — cannot be asked
	// here yet: a `(` straight after a `)` ends the word, and the same is
	// true of `f @(a)(b)` with no count anywhere in it, so that refusal is
	// a lexer gap of its own and not this flag's.
	noBareGroup bool
	// armParens says a bare `(` opens a group on **this surface** even behind
	// a pattern group, so noBareGroup is not armed here either.
	//
	// A `case` arm is that surface and the parameter-expansion operand is the
	// other; a word and a condition are on the far side of it. What a run
	// standing behind a pattern group *means* is where ksh93 disagrees with
	// itself, and this is that disagreement written down rather than smoothed
	// over. Measured 2026-09-28 against /bin/ksh `Version AJM 93u+
	// 2012-08-01`, in a directory holding `ab`, `a(b)` and `a()`:
	//
	//	                                    reads
	//	printf "[%s]" @(a)(b)       a(b)    text
	//	[[ "a(b)" == @(a)(b) ]]     yes     text
	//	[[ ab == @(a)(b) ]]         no
	//	case ab in @(a)(b))         HIT     a group
	//	case "a(b)" in @(a)(b))     MISS
	//	v=ab;     ${v#@(a)(b)}      empty   a group
	//	v='a(b)'; ${v#@(a)(b)}      a(b)
	//
	// Each surface is a **pair** on purpose: one subject the text reading
	// matches and one the group reading does, so a row cannot agree for the
	// other reading's reason. The quoted spelling is the control that says
	// quoting still literalises where a group is read —
	// `v=ab; ${v#@(a)"(b)"}` is `ab` and `v='a(b)'; …` is empty.
	//
	// The control that ties all four surfaces together is that a bare run
	// with **no** group in front of it is a syntax error in a word and in a
	// condition — `[[ ab == a(b) ]]` — so it is the group in front that
	// licenses the run at all, and then each surface reads what it licensed
	// its own way. See syntax.Dialect.ParenRunAfterPatternGroupIsText, which
	// is the lexical half (#4972).
	armParens bool
	// topGroup reads a `|` standing outside every group and bracket as an
	// alternation of the whole pattern, which one dialect does and only for
	// a bar that arrived live — see matchTopLevel. Separate from group for
	// the reason group and quantified are separate: the dialect that has
	// bare groups is not the only one that could have this.
	//
	// A bar the script *wrote* never reaches here unescaped, and that is
	// arranged rather than assumed. This comment used to say a written bar
	// was a parse error in every shell measured, "so nothing but a value can
	// put one here" — which is false inside a `${…}`, where the braces keep
	// the bar out of the command grammar and it arrives as pattern text.
	// zsh reads it as an ordinary character and this read it as an
	// alternation (#2168). markWrittenBars escapes it at the one place a
	// pattern is built, so the invariant the matcher relies on holds again.
	topGroup bool
	// bareParenIsText reads a `(` with no quantifier in front of it, and the
	// `)` that would close it, as ordinary characters, so a `|` between them
	// stands at the top level. That is zsh with `shglob` and `kshglob` both
	// on: the word may hold the parenthesis, but only a quantified one opens
	// a group. Measured 2026-10-02 on zsh 5.9.2, `[[ S == a(b|c)d ]]` matches
	// `a(b` and `c)d` and not `abd`, and `[[ 'x(y)z' == x(y)z ]]` matches
	// (#5467).
	bareParenIsText bool
	// period says the subject begins with a character only a period
	// *written* in the pattern may consume — the other half of the
	// leading-period rule, and the half that is about this name rather than
	// about the pattern.
	//
	// interp/globhidden.go answers the first half: whether a pattern begins
	// with an explicit period at all, which decides whether a directory's
	// hidden names are offered to the matcher. That answer is about the
	// pattern alone, so it is the same for every name — and on its own it
	// lets an arm that has nothing to do with the period take one. Measured
	// 2026-09-22 in a directory holding `.a`, `.b`, `.foo`, `bar` and `x`,
	// where all three shells with groups agree:
	//
	//	                  bash 5.3.20   ksh93u+       zsh 5.9.2
	//	echo @(.foo|*)    .foo bar      .foo bar x    (.foo|*) → .foo bar x
	//	echo @(?|.?)      .a .b x       .. .a .b x    (?|.?)   → .a .b x
	//
	// So the `*` arm reaches `bar` and not `.a`: a period the *subject*
	// begins with has to be taken by a period the pattern wrote, on the
	// branch that actually matched. A wildcard, a bracket or a negation
	// standing there does not take it, however the pattern begins.
	//
	// Set per name by the walk, because it is the name's own question: the
	// option that lifts it — bash's `dotglob`, zsh's `globdots` — lifts it
	// for an ordinary hidden name and never for `.` and `..`, which is why
	// `echo *` under `dotglob` lists `.a` and not `.`.
	period bool

	// bad is set when the pattern is one the dialect rejects outright. It is
	// a field rather than a return value because matchHere recurses, and
	// threading a second result through every branch obscured the matching.
	bad *bool
	// numericRange reads `<n-m>` as a run of digits whose value falls in the
	// range. Its own field rather than a spelling of `group`, because the
	// two dialect answers are independent — the shell that has bare groups
	// happens to be the one with ranges, and neither implies the other.
	numericRange bool
	// fold compares letters without case. Not a dialect answer but a
	// run-time one — GlobFoldsCase and MatchFoldsCase — which is why the two
	// call sites that honor an option set it and the rest leave it off: the
	// shell with the options keeps parameter expansion exact either way.
	fold bool
	// foldWide says the fold above reaches past ASCII, which is the locale's
	// answer rather than the option's: an explicit C or POSIX locale narrows
	// a fold to ASCII and any other locale folds Unicode. The conclusion is
	// carried rather than the question, for the reason chars below is —
	// the matcher has no Runner to ask — and it is resolved by the one
	// helper the converting sites use, interp/multibyte.go's
	// caseFoldReachesBeyondASCII. See #2644.
	//
	// Never set with fold off, so nothing here asks a locale question a
	// script did not reach for by turning the option on.
	foldWide bool
	// chars makes one unit of the subject a character rather than a byte, so
	// `?` consumes a whole one, a bracket matches a whole one, and a `*`
	// tries only the split points between them.
	//
	// A run-time answer like fold, and for a stronger reason: it is the
	// dialect's MultibyteEncodingIsHonored *and* the locale in force, which
	// interp/multibyte.go resolves together. The matcher is handed the
	// conclusion because it has no Runner to ask — and because the answer
	// only ever moves when the pattern or the subject holds a byte above
	// ASCII, which is where the callers ask.
	chars bool
	// classes is the character-class names beyond the twelve POSIX ones that
	// this dialect answers, with the shell state two of them read already
	// resolved. Its own value rather than a Runner because the matcher has
	// none, which is the same reason chars and fold are here.
	classes patternClasses
	// record says this surface writes what the pattern matched into the
	// record a `=~` fills, which one dialect does for a condition and for
	// every pattern operator of parameter expansion. It is the surface's
	// answer and not the pattern's, which is why it is set by
	// Runner.recordingPatternOpts rather than read off the text — see
	// interp/patternrecord.go.
	record bool
	// extended says the operators one shell keeps behind an option of its
	// own are live: `(#…)` flag groups, the `#` and `##` closures, the `^`
	// negation and the `~` exclusion. Off, all four are ordinary characters,
	// which is measured — see interp/patternflags.go.
	extended bool
	// foldClass says the fold above also reaches a POSIX character class
	// inside a bracket expression. It is a narrowing of fold rather than a
	// second one — never set with fold off — and the two are separate
	// because the fold's *source* decides it.
	//
	// Measured 2026-09-14, `LC_ALL=C`, with each shell's own switch on:
	//
	//	                            bash 5.3.15   ksh93u+   ours before
	//	[[ A == [[:lower:]] ]]      exact         —         fold
	//	[[ a == [[:upper:]] ]]      exact         —         fold
	//	[[ A == [a-z] ]]            fold          —         fold
	//	case A in [[:lower:]])      exact         —         fold
	//	v=ABC; ${v//[[:lower:]]/X}  ABC           —          XXX
	//	[[ A == ~(i)[[:lower:]] ]]  —             fold      fold
	//	[[ A == ~(i)[a-z] ]]        —             fold      fold
	//
	// So an *option* — `nocasematch`, `nocaseglob` — folds a literal and a
	// range inside a bracket and stops at a class, while the inline `~(i)`
	// flag folds all three. bash 3.2.57 folds the class one way and not the
	// other — `A` into `[[:lower:]]` but not `a` into `[[:upper:]]` — which
	// no single rule explains and which no preset here claims, so 5.3 is the
	// column followed. zsh's `(#i)` reaches no bracket at all and is litFold
	// below rather than either of these. #2716.
	//
	// A `=~` expression is a different mechanism again: the fold is a
	// property of the compiled regular expression, so a class inside one
	// folds in every column and never reaches this matcher.
	foldClass bool
	// litFold is the case comparison a `(#i)`, `(#I)` or `(#l)` flag asked
	// for. It reaches only the literal characters of a pattern, which is
	// what keeps it apart from fold above.
	litFold caseFolding
	// where is the subject's length, the group numbering and the place a
	// match reports itself — everything the position-aware flags need, held
	// behind one pointer. See matchWhere for why it is not three fields.
	where *matchWhere
	// escapes is the set of characters a backslash escapes. Empty means
	// every character, which is five of the six shells' answer; a set means
	// a backslash before anything outside it is a literal backslash and the
	// character after it stands on its own. See
	// Semantics.PatternEscapeReaches, which is where it is measured.
	escapes string
	// askBracketAfterSub resolves
	// Semantics.UnterminatedBracketAfterASubExpression, and is a function
	// rather than a value so that the axis is asked only where a bracket
	// really ran off the end behind a `[:name:]`, a `[.x.]` or a `[=x=]`.
	// Nil leaves the question to bracket above, which is every caller that
	// has not been given the dialect to ask.
	askBracketAfterSub func() BracketPolicy
	// collating is what `[.x.]` and `[=x=]` inside a bracket expression are
	// — see Semantics.CollatingElements. NoCollatingElements leaves the
	// delimiters as ordinary members, which is the one column that has
	// neither construct.
	//
	// Resolved only where the pattern really opens one, so a shell without
	// an answer is asked nothing by a bracket that never spells it.
	collating CollatingElementPolicy
	// bracketMember says a backslash *inside* a bracket expression escapes
	// nothing and is an ordinary member of the set — the third reading of
	// [Semantics.BracketEscape], which BusyBox ash holds. The escape rule
	// outside a bracket is `escapes` above and is untouched by it, which is
	// the control that says this is about the bracket.
	//
	// Set only where the pattern really holds one, so the axis is asked
	// where it decides and nowhere else.
	bracketMember bool
}

// bracketPolicy is what the text an unterminated bracket left behind means,
// and it is asked here rather than resolved up front because only a bracket
// that really ran off the end poses the question — which is something only
// the matcher's own scan knows. A pattern with a bracket in it is not a
// pattern that asks: `[[:alpha:]]` closes and `[[:alpha:]` does not, and the
// two differ by a character no separate scan of ours reads the same way as
// this one does. Asking through a function is what keeps the one reading.
//
// sub says a bracket sub-expression is what left it open. It is a second axis
// and not a corner of the first, because ksh93 moves between them: a bare `[`
// is a literal `[` there and `[[:alpha:]` matches nothing. See
// Semantics.UnterminatedBracketAfterASubExpression.
func (o *patternOpts) bracketPolicy(sub bool) BracketPolicy {
	if sub && o.askBracketAfterSub != nil {
		return o.askBracketAfterSub()
	}
	return o.bracket
}

// escapeReaches reports whether a backslash escapes c rather than standing
// for itself.
func (o *patternOpts) escapeReaches(c byte) bool {
	return o.escapes == "" || strings.IndexByte(o.escapes, c) >= 0
}

// unitWidth is how many bytes of a non-empty subject one `?` consumes, one
// bracket matches, and one step of a `*` passes over.
func (o *patternOpts) unitWidth(s string) int {
	if !o.chars {
		return 1
	}
	return characterWidth(s, "")
}

// unitBefore is the start of the unit ending at i — unitWidth walked
// backwards. A unit here is a rune rather than a grapheme cluster, so the
// boundary is findable by stepping back over UTF-8 continuation bytes and
// needs no table of its own.
func (o *patternOpts) unitBefore(s string, i int) int {
	if i <= 0 {
		return 0
	}
	if !o.chars {
		return i - 1
	}
	j := i - 1
	for j > 0 && s[j]&0xC0 == 0x80 {
		j--
	}
	return j
}

// eqByte compares two bytes, without case when fold says so. ASCII only: the
// folding a shell does inside a pattern is `nocasematch` and its kin, which
// this implementation has never taken past ASCII, and which is its own
// measurement rather than this one's.
func eqByte(a, b byte, fold bool) bool {
	return a == b || (fold && swapCase(a) == b)
}

// eqUnit compares two whole units — one byte each, or one character each.
//
// wide is whether the fold reaches past ASCII, which is where a multi-byte
// unit stops being a run of bytes to compare and becomes a character with a
// case of its own: with it off, `[[ é == [É] ]]` is a miss, and with it on it
// matches, which is what bash 5.3.15 answers under a UTF-8 locale.
func eqUnit(a, b string, fold, wide bool) bool {
	if len(a) == 1 && len(b) == 1 {
		return eqByte(a[0], b[0], fold)
	}
	if a == b {
		return true
	}
	if !fold || !wide {
		return false
	}
	ar, an := utf8.DecodeRuneInString(a)
	br, bn := utf8.DecodeRuneInString(b)
	if an != len(a) || bn != len(b) {
		return false
	}
	return eqRuneFolded(ar, br)
}

// ordOf ranks one unit for a bracket range or a character class: its code
// point, or its byte where the unit is one byte.
//
// It needs no answer about whether characters are being matched, and no case
// for an undecodable byte, because a unit is never either of the two things
// that would want one. unitWidth answers 1 for every byte where characters
// are not being counted, and DecodeRuneInString answers a width above 1 only
// for a sequence it decoded — so a unit longer than a byte is always a valid
// character, and a byte that begins no sequence arrives here alone and ranks
// as itself. Both were written as guards first and both are dead, which
// mutation testing is what said.
func ordOf(unit string) rune {
	if len(unit) == 1 {
		return rune(unit[0])
	}
	c, _ := utf8.DecodeRuneInString(unit)
	return c
}

// eqRuneFolded reports whether two characters are the same letter in
// different cases.
//
// Lower-casing **both** sides rather than swapping one and comparing, which
// is measured rather than tidiness. The two disagree on the characters whose
// case mapping is not a pair, and the panel follows the lower-casing:
// measured 2026-09-13 on bash 5.3.15 under `en_US.UTF-8` with `nocasematch`
// on, the Kelvin sign K (U+212A) matches both `k` and `K` — its lower case is
// the ASCII `k` — while ſ (U+017F) matches neither `s` nor `S`, because it is
// already lower case and lower-casing `s` cannot reach it. A swap through
// `unicode.ToUpper` would answer the opposite on both: ſ upper-cases to `S`
// and the Kelvin sign upper-cases to itself.
//
// It is the C library's towlower that those shells fold with, and Go's
// unicode.ToLower is the same simple mapping, so this is a correction and not
// a platform's answer.
func eqRuneFolded(a, b rune) bool {
	return a == b || unicode.ToLower(a) == unicode.ToLower(b)
}

// swapRuneCase is the other case of a letter, or the character itself, for
// the two places a bracket expression needs one character rather than a
// comparison. Lower case first, for the reason eqRuneFolded lower-cases:
// measured, `[[ K == [a-z] ]]` matches with the fold on, because the Kelvin
// sign's lower case is the ASCII `k` that the range holds.
func swapRuneCase(c rune) rune {
	if l := unicode.ToLower(c); l != c {
		return l
	}
	return unicode.ToUpper(c)
}

// swapCase is the other case of an ASCII letter, or the byte itself.
func swapCase(c byte) byte {
	switch {
	case c >= 'a' && c <= 'z':
		return c - 'a' + 'A'
	case c >= 'A' && c <= 'Z':
		return c - 'A' + 'a'
	}
	return c
}

// periodHere reports that the matcher stands at a leading period only a
// written period may consume. See patternOpts.period.
func (o *patternOpts) periodHere(s string, at int) bool {
	return o.period && at == 0 && len(s) > 0 && s[0] == '.'
}

func matchPattern(pattern, s string, o patternOpts) bool {
	// The whole-subject entry point by definition: every caller here asks
	// whether the pattern describes the string, and none of them is choosing
	// a span. See patternOpts.whole.
	o.whole = true
	ok, _ := matchPatternIn(pattern, s, s, 0, o)
	return ok
}

// matchPatternAt is matchPattern for a *piece* of a larger subject: base is
// where the piece begins in the whole subject, and total is the whole
// subject's length.
//
// The two are what the position-aware flags need and what nothing else in the
// matcher carries, because `(#s)` and `(#e)` ask about the *subject* and not
// about the piece a surface happened to hand over. Measured on zsh 5.9.2:
// `x=abcd; ${x#ab(#e)}` leaves `abcd` alone where `${x#abcd(#e)}` empties it,
// and `${x%(#s)cd}` leaves it alone where `${x%(#s)abcd}` empties it — so a
// trim's prefix trial is at the start of the subject and never at its end,
// and a suffix trial the other way round.
//
// Surfaces that match a whole string pass base 0 and the string's length,
// which matchPattern does for them. Pathname expansion is one of those and
// that is measured rather than assumed: `**/(#s)a*` matches `cx/ax`, so the
// anchors bind to the *component* the walk is matching and not to the path.
// matchPatternIn is matchPattern for a *piece* of a subject, and the one entry
// point that reports what the position-aware flags captured.
//
// piece begins at byte base of subject, and the two together are what the
// anchors need: `(#s)` and `(#e)` ask about the subject and not about the
// piece a surface happened to hand over. Measured on zsh 5.9.2: `x=abcd;
// ${x#ab(#e)}` leaves `abcd` alone where `${x#abcd(#e)}` empties it, and
// `${x%(#s)cd}` leaves it alone where `${x%(#s)abcd}` empties it — so a
// trim's prefix trial is at the start of the subject and never at its end,
// and a suffix trial the other way round.
//
// Surfaces that match a whole string pass the same string twice, which
// matchPattern does for them. Pathname expansion is one of those and that is
// measured rather than assumed: `**/(#s)a*` matches `cx/ax`, so the anchors
// bind to the *component* the walk is matching and not to the path.
//
// The report is empty for a pattern that asks for nothing, which is nearly
// all of them, and for one that did not match — measured, a failed `(#b)`
// leaves `$match` exactly as it was.
func matchPatternIn(pattern, piece, subject string, base int, o patternOpts) (bool, matchReport) {
	if o.tilde {
		if body, rest, ok := splitTildeModifier(pattern); ok {
			m, _ := readTildeModifier(body)
			if m.classes && !o.tildeGlobRead {
				// A `~(K)` group this surface does not read. The text is
				// ordinary characters, so the walk below is handed the
				// pattern with the group still on it — which is what makes
				// `${v#~(K)x}` leave `xab` alone. See
				// patternOpts.tildeGlobRead, and note that only this letter
				// is declined: `~(i)` and the rest are read here as before.
				o.tilde = false
				return matchPatternIn(pattern, piece, subject, base, o)
			}
			// Read once and not again: the prefix is off the pattern now, so
			// a `~(K)` that falls back to this matcher cannot loop on its
			// own group.
			o.tilde = false
			return matchTilde(m, rest, piece, subject, base, o)
		}
		// And one standing further along, which settles the whole match the
		// same way rather than being a flag the walk carries: a flavor says
		// what language the pattern is written in. See
		// interp/tildeflavorhere.go.
		o.tilde = false
		if got, isFlavor := matchTildeFlavorHere(pattern, piece, subject, base, o); isFlavor {
			return got, matchReport{}
		}
		o.tilde = true
	}
	// And a group further along asking for an anchor, which is a question
	// about where this *piece* sits in the subject rather than about where
	// the group stands — the same comparison matchTilde makes for a group at
	// the front, and asked here for the reason tildeHereAnchors gives.
	//
	// Outside the block above because that one is cleared once a front group
	// has been read, and a pattern may carry both: `~(i)z~(r)ab` is a fold
	// at the front and an anchor one character along.
	if o.tildeFold {
		if left, right := tildeHereAnchors(pattern); (left && !o.tildeLeftUnread && base != 0) ||
			(right && base+len(piece) != len(subject)) {
			return false, matchReport{}
		}
	}
	w := o.where
	if w == nil {
		// A caller that built its options by hand rather than through
		// Runner.patternOpts. It asks for no flag, so the plan is empty and
		// this allocates once for the whole match rather than per trial.
		w = &matchWhere{}
		o.where = w
	}
	w.total, w.caps = len(subject), newCaptures(w.plan)
	// Where this piece sits in the subject, for the zero-width escapes. Set
	// on every entry rather than once: matchPatternIn is re-entered for a
	// group this surface declines, and a stale pair would answer about the
	// piece before it.
	o.pieceBase, o.pieceEnd = base, base+len(piece)
	// The captures are this trial's and are always replaced. The memo is
	// about the pattern and the subject, so it survives a trial and is
	// dropped only when one of those changes — see matchWhere.
	if !w.ready || w.pattern != pattern || w.subject != subject {
		newPattern := !w.ready || w.pattern != pattern
		w.ready, w.pattern, w.subject = true, pattern, subject
		w.asked, w.deadWide = 0, nil
		w.dead.reset()
		w.packable = len(pattern) < packBase && len(subject) < packBase
		if newPattern {
			// Only when the *pattern* moved. Pathname expansion matches one
			// pattern against every name in a directory, so the subject
			// changes far more often than the pattern does, and none of
			// what prepare works out is about the subject.
			w.prepare(pattern, o.emptyBracket)
		}
	}
	// And the memo is dropped again when the **base** moves under a pattern
	// that carries a zero-width escape, which is the one thing in this
	// language whose answer is not a function of the position alone.
	//
	// It is the aliasing tildeHereAnchors names from the other side: the
	// memo is keyed on a position, and two trials of a suffix trim reach the
	// same position from different starts — `${v%\bab}` against `aab` asks
	// about offset 1 with the piece beginning there and again with the piece
	// beginning at 0, and `\b` answers those two differently. Everything
	// else the key leaves out is a fact about the pattern or the subject, so
	// this is the only escape that needs it. See kshZeroWidthHolds.
	if base != w.base {
		if patternHasZeroWidthEscape(pattern) {
			w.asked, w.deadWide = 0, nil
			w.dead.reset()
		}
		w.base = base
	}
	if !matchTopLevel(pattern, piece, base, o) {
		return false, matchReport{}
	}
	span := capSpan{begin: base, end: base + len(piece), set: true}
	m := w.caps.report(subject, span, w.plan.whole)
	// Who publishes it travels with it: a plan a surface seeded goes to the
	// record rather than to the parameters a pattern flag fills. See
	// interp/patternrecord.go.
	m.recording = w.plan.recording
	return true, m
}

// matchTopLevel matches a whole pattern, splitting it on a `|` that stands
// outside every group and bracket where the dialect reads one as an
// alternation.
//
// One shell in the panel does, and only for a `|` that arrived *live* — from
// an expansion marked with `${~name}`, from a value under `globsubst`, or from
// a group the rest of the word supplied. The written spelling is a parse error
// there and here alike, which is what makes this reachable from a value and
// nowhere else: `[[ a = a|b ]]` is `parse error near '|'` in zsh 5.9.2.
//
// Measured on zsh 5.9.2, 2026-09-08 and again 2026-09-11, each probe in a
// script of its own under `env -i`, with `L='a|b'`:
//
//	[[ a = ${~L} ]]                  matches
//	case a in ${~L}) …               takes the arm
//	setopt globsubst; [[ a = $L ]]   matches
//	print -l -- ${~L}                lists the files `a` and `b`
//	[[ 'a|b' = ${~L} ]]              does *not* match
//
// The last row is the one that says this is a split rather than an extra
// character: the text the value holds stops matching itself.
//
// A quoted bar is untouched, because a quoted character reaches the matcher
// escaped and this walks past an escape — which is the same rule that keeps
// `[[ a = 'a|b' ]]` a literal comparison in the shell that splits live ones.
//
// Inside a bracket a bar is an ordinary member: measured, `L='[a|b]'` matches
// `a` and matches `|`, so the split skips a bracket expression the way it
// skips a group. An empty arm is allowed and matches the empty string, so
// `L='a|'` matches `a` and matches nothing at all.
func matchTopLevel(pattern, piece string, base int, o patternOpts) bool {
	if !o.topGroup {
		return matchHere(pattern, piece, 0, base, o)
	}
	arms, armAt := topAlternatives(pattern, o.emptyBracket, o.bareParenIsText)
	if len(arms) == 1 {
		return matchHere(pattern, piece, 0, base, o)
	}
	for i, arm := range arms {
		// The captures of a failed arm are unwound before the next is tried,
		// which is the invariant matchGroup already keeps for the arms of a
		// written group: a trial that fails must leave `$match` as it was.
		mark := o.where.caps.mark()
		if matchHere(arm, piece, armAt[i], base, o) {
			return true
		}
		o.where.caps.rollback(mark)
	}
	return false
}

// topAlternatives splits a pattern on the `|` at its top level, with each
// arm's offset in the pattern — which is what a group inside an arm needs in
// order to know its own number.
//
// Its own walker rather than alternativesAt, which splits a *group's* body and
// counts only parentheses. A top-level bar has a bracket expression to stay
// out of as well, and `[a|b]` is measured to be a bracket holding three
// members rather than two arms — bracketEnd is the same scan the matcher's
// own bracket reader uses, so the two cannot disagree about where one ends.
func topAlternatives(pattern string, emptyCompiles, bareParenIsText bool) (arms []string, offsets []int) {
	depth, start := 0, 0
	// Which open parentheses counted, where a bare one is text: only those
	// close a level. See patternOpts.bareParenIsText.
	var counted []bool
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i++
		case '(':
			opens := !bareParenIsText || (i > 0 && strings.IndexByte("@?*+!", pattern[i-1]) >= 0)
			counted = append(counted, opens)
			if opens {
				depth++
			}
		case ')':
			if n := len(counted); n > 0 {
				if counted[n-1] {
					depth--
				}
				counted = counted[:n-1]
			} else if !bareParenIsText {
				depth--
			}
		case '[':
			// Past the whole bracket expression, class names included:
			// measured, `L='[a|b]'` matches `a` and matches `|`, so a bar
			// between two members is not a split. An unterminated `[` is not
			// a bracket expression and its text is ordinary, which is what
			// skipBracket answers by standing still.
			i = skipBracket(pattern, i, emptyCompiles)
		case '|':
			if depth == 0 {
				arms, offsets = append(arms, pattern[start:i]), append(offsets, start)
				start = i + 1
			}
		}
	}
	return append(arms, pattern[start:]), append(offsets, start)
}

// matchHere matches p against the whole of s, where p begins at offset pp of
// the pattern and s at offset at of the subject the caller named.
//
// Both are threaded rather than derived because both strings are re-sliced on
// every step and a slice does not remember where it came from.
//
// at is what the anchors need: `(#s)` is `at == 0` and `(#e)` is
// `at == o.total`. pp is what the *backreferences* need, and for a different
// reason — a group's number is a fact about where its `(` stands in the
// pattern text and not about the path the matcher took to reach it, which is
// measured: `(#b)((x)|a(b)c)` numbers `(x)` as 2 even in the run where the
// arm holding it is never taken.
//
// It also remembers which questions came back **false**, which is what keeps
// a pattern with alternation over closures in it from taking eight seconds.
// Nothing is remembered about a match that succeeded: which arm and which
// split won decides what a `(#b)` reports, so a successful trial has to be
// re-run to write its captures, and the order the search tries things in is
// left exactly as it was. Only the dead ends are skipped, and a dead end
// wrote nothing by the invariant matchGroup states — every attempt that can
// write a capture unwinds it when it fails.
//
// That the key is *exact* is a fact about this matcher rather than a hope
// about it; matchKey says which fact, and how it was established. The one
// side effect a failing trial can have is `*o.bad`, which only ever goes
// true, so a question already asked has already set it.
//
// #1383: 11.2 million calls for an 82-byte subject against a 68-byte pattern,
// where about 5,600 distinct questions exist — a 2,000-fold redundancy, and a
// cost growing as the fourth power of the subject. Memoized, the same match
// is 61x faster and grows as roughly n^1.5. The startup it was found in went
// from 8.05s to 0.145s against real zsh's 0.143s on the same configuration.
func matchHere(p, s string, pp, at int, o patternOpts) bool {
	w := o.where
	// A `~(…)` flavor group inside this piece of the pattern settles the
	// language of the piece, so the piece is not walked at all — it is
	// translated and handed to that flavor's engine. matchPatternIn reads the
	// one at the top of a pattern; this is the same reading for the body of a
	// group, an arm, or whatever follows one. See matchTildeFlavorInPiece.
	//
	// Ahead of the memo rather than behind it, so the answer is never keyed
	// on a position that does not carry the fold the branch arrived with.
	//
	// `whole` is the gate because a surface that **chooses a span** is one
	// the reference shell does not answer consistently for this shape — see
	// matchTildeFlavorInPiece's last table — so a trim or a substitution is
	// left exactly as it was.
	if o.whole && o.tildeFold && (!w.ready || !w.noTilde) {
		if got, claimed := matchTildeFlavorInPiece(p, s, at, o); claimed {
			return got
		}
	}
	w.asked++
	if w.asked <= memoThreshold {
		return matchBranch(p, s, pp, at, o)
	}
	if w.known(pp, len(p), at, len(s), o.litFold) {
		return false
	}
	if matchBranch(p, s, pp, at, o) {
		return true
	}
	w.remember(pp, len(p), at, len(s), o.litFold)
	return false
}

// memoThreshold is how many questions a trial may ask before it starts
// writing the answers down.
//
// A threshold rather than always, because the memo is not free — a key to
// build and a map to probe on every question, and a map to allocate on the
// first one — and it earns nothing on the patterns a shell actually spends
// its life on. `*.go` against a filename is a handful of questions, none of
// them repeated, so a memo there is a pure loss; it is the same argument
// patternOpts' pointer was made for, from the other side.
//
// The number is loose on purpose. It only has to be high enough that no
// ordinary pattern reaches it and low enough that a pathological one pays
// the linear prefix once rather than the polynomial. Anything in the
// hundreds satisfies both: the #1383 pattern asks eleven million questions.
//
// A var rather than a const so that a test can drive a pattern down both
// paths and compare, which is the only way to say "the memo changes no
// answer" rather than to hope it. Nothing outside a test writes it.
var memoThreshold = 512

// matchBranch is matchHere without the memo — the matching itself. Every
// recursion goes back through matchHere so that it is memoized too.
func matchBranch(p, s string, pp, at int, o patternOpts) bool {
	for len(p) > 0 {
		if o.extended {
			// The exclusion binds loosest, so it is read before anything
			// else in the branch: every side is matched against the whole
			// of what is left of the subject.
			if left, rights, ok := splitExclusion(p, &o); ok {
				if !matchHere(left, s, pp, at, o) {
					return false
				}
				xp := pp + len(left)
				for _, x := range rights {
					xp++ // the `~` between this side and the last
					if matchHere(x, s, xp, at, o) {
						return false
					}
					xp += len(x)
				}
				return true
			}
			// `^` turns the sense of the rest of the branch — measured,
			// `[[ ab == a^x ]]` matches, so it starts where it stands
			// rather than only at the front of a pattern.
			if p[0] == '^' {
				return !matchHere(p[1:], s, pp+1, at, o)
			}
			// A flag group has to be read before splitGroup below, which
			// would otherwise take `(#i)` for an alternation of one.
			if body, rest, ok := splitPatternFlags(p, o.emptyBracket); ok {
				if a := anchorPatternFlag(body); a != anchorNone {
					// A zero-width assertion about where the match
					// stands, so it consumes nothing and decides the
					// branch outright.
					if !a.holds(at, o) {
						return false
					}
					pp, p = pp+len(p)-len(rest), rest
					continue
				}
				next, unknown := applyPatternFlags(body, o)
				if unknown != 0 {
					// Refused by name before matching began; there is
					// nothing this can honestly answer.
					return false
				}
				o, pp, p = next, pp+len(p)-len(rest), rest
				continue
			}
			// A closure repeats the one item in front of it, so the item is
			// read here rather than by the branches below.
			if item, rest, ok := splitClosableItem(p, pp, &o); ok {
				if lo, hi, after, isClosure := closureBounds(rest, &o); isClosure {
					return matchRepeat(item, pp, lo, hi, after, pp+len(p)-len(after), s, at, o)
				}
			}
		}
		// A ksh `~(…)` group is read where it stands, and before splitGroup
		// for the reason a `(#i)` is read before it: the scan would
		// otherwise take the group's own parentheses for an alternation.
		// What the walk does with one is **consume it**, and only `i` changes
		// anything it carries — see splitTildeHereGroup for what each letter
		// asks and where the rest of them are answered, and for why a flavor
		// letter is left as the text it already was.
		//
		// Outside the `o.extended` block above because the two dialects that
		// have a mid-pattern option group are not the same dialect: `(#i)`
		// is one shell's and behind that shell's option, and this is the
		// other's and behind the grammar flag that let the `(` into the word.
		if o.tildeFold {
			if g, ok := splitTildeHereGroup(p); ok && (!g.classes || o.tildeGlobRead) {
				// The second half is the surface asking whether it reads a
				// `~(K)` group at all — `g.classes` is that letter and
				// nothing else. Where it does not, the group is left
				// standing and the walk below spends it as ordinary
				// characters — which is what makes `${v#~(K)x}` leave its
				// value alone. Only that letter is declined: `~(i)` and the
				// rest are consumed here on every surface, which is measured
				// (`${v#~(i)[0-9]}` trims in both columns). See
				// patternOpts.tildeGlobRead.
				_, rest, _ := splitTildeModifier(p)
				o = tildeFoldHere(o, g.foldSet, g.fold)
				// `K` is read where it stands too, and that is measured:
				// `[[ za1b == za\db~(K) ]]` is no in ksh93u+ while
				// `[[ za1b == z~(K)a\db ]]` is yes, so a group behind the
				// escape does not reach it. See kshClassEscapes.
				o = tildeClassesHere(o, g.classesSet, g.classes)
				pp, p = pp+len(p)-len(rest), rest
				continue
			}
		}
		// A repetition count written in front of a group, read before
		// splitGroup for the reason the two above it are: the scan would
		// otherwise reach the `(` with the brace already spent as five
		// ordinary characters, and read the group as though nothing stood
		// in front of it.
		if g, ok := splitCountedGroup(p, pp, &o); ok {
			// From here on a bare `(` in this branch is a character: the
			// count is the only way one reached the word at all — except on
			// the one surface where a parenthesis has a door of its own.
			// See patternOpts.noBareGroup and patternOpts.operandParens.
			o.noBareGroup = !o.operandParens
			if !g.counts {
				// The brace is not a count, so the `{…}` and the `(`
				// behind it are ordinary characters and the group is not
				// read: `[[ '{z,y}a' == {z,y}(a) ]]` does not match on
				// ksh93u+ where `[[ '{z,y}(a)' == {z,y}(a) ]]` does. The
				// `)` further on becomes a character the same way — by
				// nothing having opened a group for it. See #4933, the row
				// that catches a reading keyed on whether the contents are
				// a number.
				//
				// Spending them one unit at a time rather than comparing
				// the run, because the rest of the pattern is still a
				// pattern: a `?`, a `*`, a bracket or a group *inside*
				// these parentheses keeps its meaning, which the three
				// rows at splitCountedGroup measure.
				//
				// The parenthesis goes with them on every surface but the
				// operand, where it opens a group of its own and is left
				// for splitGroup to read: `s='{z,y}a'; ${s#{z,y}(a)}` is
				// empty on ksh93u+ and `s='{z,y}(a)'; ${s#{z,y}(a)}` is
				// unchanged, which is the opposite of the answer the same
				// text gets in a condition. See patternOpts.operandParens.
				spend := g.lead
				if !o.operandParens {
					spend++
				}
				for n := spend; n > 0; {
					if s == "" {
						return false
					}
					pw, sw, ok := o.eqPatternHere(p, s)
					if !ok {
						return false
					}
					p, s, pp, at, n = p[pw:], s[sw:], pp+pw, at+sw, n-pw
				}
				continue
			}
			return matchGroupTimes(g.body, pp, g.lead, 0, g.bound, true,
				g.rest, pp+len(p)-len(g.rest), s, at, o)
		}
		if body, quant, rest, ok := splitGroup(p, pp, &o); ok {
			lead := 0
			if quant != 0 {
				lead = 1
			}
			// From here on a bare `(` in this branch is a character on the
			// surfaces that read one that way, exactly as it is once a
			// *count* has been read. The lexer only lets a bare run into a
			// word behind a group at all — see
			// [syntax.Dialect.ParenRunAfterPatternGroupIsText] — so the two
			// doors a parenthesis has are a count and a group, and this is
			// the second of them. patternOpts.armParens has the grid and
			// says which surfaces are on which side (#4972).
			//
			// **`!o.group` is the whole of what keeps this off the other
			// dialect**, and it is not a tidiness guard: this branch is
			// every dialect's, where the count's identical line is reached
			// only through splitCountedGroup. Where a bare `(` opens a group
			// *anywhere*, a second group behind the first is still a group —
			// arming it there turned
			// `(#b)[[:blank:]]#([![:blank:]=]##)[[:blank:]]#[=][[:blank:]]#(*)`
			// into a pattern whose last group was three characters, which is
			// the prompt line #1585 and #1217 are both about.
			o.noBareGroup = !o.group && !o.operandParens && !o.armParens
			return matchGroup(body, pp, lead, quant, rest, pp+len(p)-len(rest), s, at, o)
		}
		if lo, hi, rest, ok := splitNumericRange(p, &o); ok {
			return matchNumericRange(lo, hi, rest, pp+len(p)-len(rest), s, at, o)
		}
		switch p[0] {
		case '*':
			// Collapse a run of stars, then try every split point.
			//
			// **Longest first, and the order is the behavior.** A `*` is
			// greedy: it takes as much as it can and leaves the rest to what
			// follows. Which end it claims does not change whether a match
			// exists — so this read "the order does not matter" for as long
			// as the answer was a bool — but it decides what a `(#b)` group
			// after it receives, and that is written into `$match` for a
			// script to read. Measured against zsh 5.9.2: `*(*)` over
			// `abc93` leaves the group **empty**, because the star took the
			// lot. Shortest first gives the group the whole subject. #2513.
			//
			// The run stops at a star that opens a *group*: `**(e|f)` is a
			// wildcard followed by the closure `*(e|f)`, not two wildcards
			// followed by a bare group — and the dialect with quantified
			// groups has no bare ones, so collapsing both left `(e|f)` to be
			// read as five ordinary characters. Measured 2026-09-22 on bash
			// 5.3.20 with `extglob`, in a directory holding `ab`, `abef`,
			// `abcdef` and `abcfef`: `ab**(e|f)` lists all four where
			// `ab*+(e|f)` lists the three that end in one.
			//
			// The first star is this branch's own and is taken whatever
			// stands behind it — the group reading was already tried and
			// declined before the switch, and a `*(` nothing closes would
			// otherwise leave the pattern where it was and recur forever.
			p, pp = p[1:], pp+1
			for len(p) > 0 && p[0] == '*' && !quantifierOpensAGroup(p, o) {
				p, pp = p[1:], pp+1
			}
			// A star may stand in front of a leading period and still not
			// take it: the only split left is the empty one.
			if o.periodHere(s, at) {
				mark := o.where.caps.mark()
				if matchHere(p, s, pp, at, o) {
					return true
				}
				o.where.caps.rollback(mark)
				return false
			}
			if p == "" {
				return true
			}
			// The split points are between units, not between bytes: a `*`
			// that stopped inside a character would hand the rest of the
			// pattern a subject beginning with a continuation byte, which a
			// following `?` would then take for a character of its own.
			//
			// Walked backwards from the end, which one-byte units can do
			// arithmetically and characters cannot — a width is only
			// readable forwards. So the character case collects the
			// boundaries it passes and then reads them in reverse, and the
			// byte case, which is every subject under `nomultibyte` and
			// every ASCII one, allocates nothing.
			if !o.chars {
				for i := len(s); ; i-- {
					mark := o.where.caps.mark()
					if matchHere(p, s[i:], pp, at+i, o) {
						return true
					}
					o.where.caps.rollback(mark)
					if i == 0 {
						return false
					}
				}
			}
			bounds := make([]int, 0, len(s)+1)
			for i := 0; ; i += o.unitWidth(s[i:]) {
				bounds = append(bounds, i)
				if i == len(s) {
					break
				}
			}
			for j := len(bounds) - 1; j >= 0; j-- {
				i := bounds[j]
				mark := o.where.caps.mark()
				if matchHere(p, s[i:], pp, at+i, o) {
					return true
				}
				o.where.caps.rollback(mark)
			}
			return false

		case '?':
			if s == "" || o.periodHere(s, at) {
				return false
			}
			w := o.unitWidth(s)
			p, s, pp, at = p[1:], s[w:], pp+1, at+w

		case '[':
			// A bracket is not a written period however its members read,
			// which is the row `[.]hidden` has always answered.
			if s == "" || o.periodHere(s, at) {
				return false
			}
			w := o.unitWidth(s)
			rest, ok := matchBracket(p, s[:w], &o)
			if !ok {
				return false
			}
			p, s, pp, at = rest, s[w:], pp+len(p)-len(rest), at+w

		case '\\':
			// An escaped metacharacter is an ordinary character.
			if len(p) < 2 {
				return s == "\\"
			}
			if o.classEscapes {
				if letter, ok := kshGlobZeroWidth(p[1]); ok {
					// And three consume **nothing**: they ask about the
					// position rather than about a character. See
					// kshGlobZeroWidth. Nothing but `pp` and `p` moves —
					// `s` and `at` are exactly where they were, which is
					// what zero-width means.
					if !kshZeroWidthHolds(letter, &o, o.where.subject, at, o.pieceBase, o.pieceEnd) {
						return false
					}
					p, pp = p[2:], pp+2
					continue
				}
				if c, ok := kshGlobControlEscape(p[1]); ok {
					// Under one dialect's `~(K)`, eight letters name a
					// **control character** rather than themselves. One
					// byte and not one unit, which is the difference from
					// the class below it: every one of them is ASCII, so a
					// multi-byte character can equal none of them and
					// taking a whole unit would consume text the escape
					// never named. See kshGlobControlEscape.
					//
					// No leading-period question. A class can match a `.`
					// and has to be asked; a control escape names one
					// character and it is not that one, so the comparison
					// answers it.
					//
					// **Advancing by one byte rather than by `unitWidth` is
					// an equivalent mutant**, and it is written down so the
					// next reader does not go looking for the row that would
					// kill it: the comparison above has already established
					// that the first byte is an ASCII control character, and
					// a unit beginning with an ASCII byte is one byte wide
					// in every locale. The mutant was run and survives. The
					// byte form is kept for what it *says* — this family
					// names a byte and the one below it names a unit.
					if s == "" || s[0] != c {
						return false
					}
					p, s, pp, at = p[2:], s[1:], pp+2, at+1
					continue
				}
				if kshClassEscape(p[1]) {
					// And six name a **character class**. One unit, exactly
					// as a `?` and a bracket expression take one — see
					// matchKshClassEscape for the pair that fixes that.
					if s == "" || o.periodHere(s, at) {
						return false
					}
					w := o.unitWidth(s)
					if !matchKshClassEscape(p[1], s[:w]) {
						return false
					}
					p, s, pp, at = p[2:], s[w:], pp+2, at+w
					continue
				}
			}
			if !o.escapeReaches(p[1]) {
				// The escape does not reach this character in this
				// dialect, so the backslash is a character of its own and
				// what follows it is matched on its next turn round the
				// loop — `bet\a` is five characters there and matches
				// `beta` not at all.
				if s == "" || s[0] != '\\' {
					return false
				}
				p, s, pp, at = p[1:], s[1:], pp+1, at+1
				continue
			}
			if s == "" || !o.eqPatternByte(p[1], s[0]) {
				return false
			}
			p, s, pp, at = p[2:], s[1:], pp+2, at+1

		default:
			if s == "" {
				return false
			}
			pw, sw, ok := o.eqPatternHere(p, s)
			if !ok {
				return false
			}
			p, s, pp, at = p[pw:], s[sw:], pp+pw, at+sw
		}
	}
	return s == ""
}

// unbounded is the bound a `<n-m>` leaves out — `<2->` has no upper one and
// `<-9>` no lower — and is not a value any side can otherwise take, because
// the digits a range is written with are never negative.
const unbounded = -1

// splitNumericRange peels a `<n-m>` off the front of a pattern.
//
// The shape the parser admitted is re-read here rather than carried, because
// the same text arrives from places the parser never saw: a pattern held in a
// variable is a pattern in the shell that has ranges, so the matcher has to
// recognize one in a plain string.
//
// A bound too large to hold is not a range at all, and the text stays
// literal. Real zsh reports `number truncated after 19 digits` and fails the
// match; refusing to read it as a range fails the same match without
// inventing a diagnostic, which is the honest half of the answer.
func splitNumericRange(p string, o *patternOpts) (lo, hi int64, rest string, ok bool) {
	if !o.numericRange || p == "" || p[0] != '<' {
		return 0, 0, "", false
	}
	i := 1
	lo, i, ok = readBound(p, i)
	if !ok {
		return 0, 0, "", false
	}
	if i >= len(p) || p[i] != '-' {
		return 0, 0, "", false
	}
	i++
	hi, i, ok = readBound(p, i)
	if !ok {
		return 0, 0, "", false
	}
	if i >= len(p) || p[i] != '>' {
		return 0, 0, "", false
	}
	return lo, hi, p[i+1:], true
}

// readBound reads one side of a range: a run of digits, or none at all for
// the side that is left open. It fails only on digits that do not fit.
func readBound(p string, i int) (bound int64, next int, ok bool) {
	start := i
	for i < len(p) && isDigit(p[i]) {
		i++
	}
	if i == start {
		return unbounded, i, true
	}
	v, err := strconv.ParseInt(p[start:i], 10, 64)
	if err != nil {
		return 0, i, false
	}
	return v, i, true
}

// matchNumericRange matches a run of digits whose value is in the range, and
// then whatever follows it.
//
// Every length is tried, shortest first, because only what comes after can
// say where the number ends: `<1-10>0` matches `100` by stopping the range at
// `10`. Leading zeros are part of the run and not of the value — `007` is 7,
// which is why `[[ 007 = <1-10> ]]` matches.
//
// A subject too large to hold saturates rather than failing. It is above
// every upper bound and below no lower one, which is the answer real zsh
// gives: `[[ 99999999999999999999 = <1-> ]]` matches there and
// `[[ 99999999999999999999 = <1-5> ]]` does not.
func matchNumericRange(lo, hi int64, rest string, pp int, s string, at int, o patternOpts) bool {
	for k := 1; k <= len(s) && isDigit(s[k-1]); k++ {
		v, err := strconv.ParseInt(s[:k], 10, 64)
		if err != nil {
			v = math.MaxInt64
		}
		if lo != unbounded && v < lo {
			continue
		}
		if hi != unbounded && v > hi {
			// Every longer run is larger still, so nothing is left to try.
			break
		}
		mark := o.where.caps.mark()
		if matchHere(rest, s[k:], pp, at+k, o) {
			return true
		}
		o.where.caps.rollback(mark)
	}
	return false
}

// unboundedReach is patternReach's answer for a pattern whose own text does
// not say how far it can go.
const unboundedReach = -1

// patternReach is an upper bound on how many *units* of a subject the pattern
// p can consume, or unboundedReach where there is no such bound.
//
// It exists to stop a search that cannot succeed, and the bound it uses is
// the pattern's own length, which is sound for a reason worth stating: every
// unit matchBranch consumes it consumes in the `?`, `[`, `\` or default
// branch, and each of those advances the pattern by at least one byte as it
// does. A group without a repeating quantifier takes one arm, and an arm is a
// substring of the group's text, so the same accounting holds inside it. So a
// pattern of n bytes reaches at most n units — unless it can spend one
// stretch of its text on any number of them, and the constructs that can are
// exactly these:
//
//   - `*`, which passes over as much as it likes.
//   - the `#` closures, `(#c…)` among them, which repeat an item. Any `#`
//     under `extendedglob` is taken for one rather than read in context; a
//     literal `#` there is written `\#` and is skipped as an escape below.
//   - `^` and `~`, which are not consumers at all but operators over the rest
//     of the branch — a bare `^` matches every non-empty subject.
//   - a numeric range, whose digits are bounded by the number it names and
//     not by the four characters of `<->`.
//   - a group under a `+` or a `!` quantifier, which repeat and complement
//     respectively. `@(…)` and `?(…)` are neither and stay bounded.
//
// A bracket expression is stepped over whole, because every one of those
// characters is an ordinary member inside one — `[#^~*]` is four literals and
// reaches one unit.
//
// Over-estimating is always safe here and under-estimating is never, so
// anything not understood is answered unbounded.
//
// #1575: without a bound, matchGroup and repeatFrom each walked every split
// of the subject, asking questions no arm could answer yes to. A trim tries
// every prefix of its value in turn, so the two together cost the square of
// the length: `${x## ##}` — the ordinary idiom for stripping leading spaces —
// took 33s on a 64,000 character value, and the startup this was found in
// reached it on one of 524,629 characters, which is around forty minutes at a
// hundred per cent of a core. The shell installs handlers for the signals a
// shell handles, so SIGTERM did not end it either; it had to be killed.
// Remembered by where the item stands, because a closure asks this of the
// same item at every level of its recursion and at every position it is tried
// from: `[^\}]##` asks it once per repetition and the answer is a fact about
// six bytes of pattern text that cannot move. pp is -1 for a caller that does
// not know where its piece begins, which reads the scan directly.
func patternReach(p string, pp int, o *patternOpts) int {
	if n, ok := o.where.reachOf(pp, len(p)); ok {
		return n
	}
	n := patternReachScan(p, pp, o)
	o.where.rememberReach(pp, len(p), n)
	return n
}

// patternReachScan is patternReach without the memo — the scan itself.
func patternReachScan(p string, pp int, o *patternOpts) int {
	for i := 0; i < len(p); {
		switch c := p[i]; {
		case c == '\\' && i+1 < len(p) && o.escapeReaches(p[i+1]):
			i += 2
			continue
		case c == '[':
			if end, found := bracketEndAt(o, p, i, pp); found {
				i = end + 1
				continue
			}
		case c == '*':
			return unboundedReach
		case o.extended && (c == '#' || c == '^' || c == '~'):
			return unboundedReach
		case o.numericRange && c == '<':
			return unboundedReach
		case o.quantified && (c == '+' || c == '!') && i+1 < len(p) && p[i+1] == '(':
			return unboundedReach
		case o.counted && c == '{':
			// A written count in front of a group. Its reach is the
			// group's times the ceiling, and a ceiling may be absent —
			// so the honest bound is none. Conservative on purpose: this
			// only ever *stops* a search early, so an answer that is too
			// large costs work and an answer that is too small costs a
			// match.
			if g, ok := splitCountedGroup(p[i:], pp+i, o); ok && g.counts {
				return unboundedReach
			}
		}
		i++
	}
	return len(p)
}

// patternReachBounds is whether a search stops where the pattern's reach
// does.
//
// A var rather than a const, for the reason memoThreshold is one: it lets a
// test drive the same pattern down both paths and require the same answer,
// which is the only way to say "the bound changes no answer" rather than to
// hope it. Nothing outside a test writes it.
var patternReachBounds = true

// splitFloor is the *smallest* split of s that leaves the rest of the pattern
// a piece it could still fill — the other end of the same bound splitCeiling
// gives, asked of what follows a group rather than of the group itself.
//
// The case it exists for is the commonest one there is: a group with nothing
// after it. `rest` is then the empty pattern, whose reach is zero, so the only
// split worth trying is the one that leaves nothing — and the loop was trying
// every split of the subject and asking the group about each, when all but the
// last of them hand the empty pattern a non-empty piece it cannot match. On
// the prompt-theme substitution of #1398 that is 82 arm attempts per trial
// where one was possible.
//
// A floor that is too *low* costs the attempts it failed to skip and changes
// no answer; one too high would skip a split that could have matched. So the
// conversion from a reach in units to a bound in bytes rounds the only way it
// can be wrong safely: a unit is at most four bytes where the subject is read
// as characters, so four times the reach is a length no bounded rest can
// exceed.
func splitFloor(p, s string, pp int, o *patternOpts) int {
	if !patternReachBounds {
		return 0
	}
	n := patternReach(p, pp, o)
	if n == unboundedReach {
		return 0
	}
	if o.chars {
		n *= utf8.UTFMax
	}
	return max(0, len(s)-n)
}

// splitCeiling is the largest split of s, in bytes, that the pattern p could
// still match — the end of s where p's reach is not bounded.
func splitCeiling(p, s string, pp int, o *patternOpts) int {
	if !patternReachBounds {
		return len(s)
	}
	n := patternReach(p, pp, o)
	if n == unboundedReach {
		return len(s)
	}
	i := 0
	for range n {
		if i >= len(s) {
			break
		}
		i += o.unitWidth(s[i:])
	}
	return min(i, len(s))
}

// splitGroup peels a group off the front of a pattern.
//
// quant is the character in front of it, or 0 for a bare group, which the
// dialect with bare groups treats as "exactly one" — the same as `@`.
func splitGroup(p string, pp int, o *patternOpts) (body string, quant byte, rest string, ok bool) {
	if w := o.where; w != nil && w.ready && w.noParen {
		// No `(` anywhere in the pattern, so nothing here opens a group and
		// the quantifier test below cannot fire either.
		return "", 0, "", false
	}
	i := 0
	if o.quantified && len(p) > 1 && p[1] == '(' {
		switch p[0] {
		case '@', '?', '+', '*', '!':
			quant, i = p[0], 1
		}
	}
	if quant == 0 && o.noBareGroup {
		// A parenthesis a count let into the word. See patternOpts.
		return "", 0, "", false
	}
	if quant == 0 {
		// A bare `(`. The dialect with bare groups takes one anywhere; the
		// dialect with quantified ones takes it *inside* a group, which is
		// measured — `@(a|(b))` matches b in ksh93, so the nested group is a
		// group there even though `a(b|c)` at the top level is a syntax
		// error. The lexer is what refuses that one, so by the time text
		// reaches here a bare paren can only have come from somewhere the
		// dialect allows it.
		if !o.group && !o.quantified {
			return "", 0, "", false
		}
		if o.bareParenIsText {
			// Text, not a group — see patternOpts.bareParenIsText.
			return "", 0, "", false
		}
		if p[0] != '(' {
			return "", 0, "", false
		}
	}
	end, found := closingParenAt(o, p[i:], pp+i)
	if !found {
		return "", 0, "", false
	}
	return p[i+1 : i+end], quant, p[i+end+1:], true
}

// closingParen is the offset of the `)` that closes the `(` at the start of p.
//
// A bracket expression is stepped over whole, because a parenthesis inside
// one is a member rather than nesting: `([(])` matches `(` on zsh 5.9.2 and
// had no closing parenthesis at all here, so the group was not a group and
// the text was read as ordinary characters (#3075). The prepared table in
// matchWhere.prepare is built from this, so the two cannot disagree.
//
// A `[` that nothing closes takes the rest of the pattern with it, and the
// group is then never closed at all. This scan is looking for a `)` and a
// bracket is where a `)` is not one — so the question is how far the bracket
// reaches, and a bracket with no `]` reaches the end. Standing still there
// instead let the `)` behind it close a group, which is what made
// `@(ab|[)` match `ab`. Measured 2026-09-22, and unanimous where it can be
// asked:
//
//	                      bash 5.3.20  ksh93u+
//	[[ ab   == @(ab|[)  ]]     no          no
//	[[ a)b  == @(a[)]b) ]]     yes         yes    the bracket took the `)`
//	[[ ab   == @(a[)b]|x) ]]   yes         yes    and `a` then `b`, a member
//	[[ a[   == @(a\[)   ]]     yes         yes    an escaped one is not a
//	                                             bracket and closes nothing
//
// The second and third rows are what say this is the bracket *reaching* past
// the parenthesis rather than the group being poisoned by it: a group whose
// bracket does close is a group, and it holds the `)` the bracket swallowed.
func closingParen(p string, emptyCompiles bool) (int, bool) {
	depth := 0
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '\\':
			i++
		case '[':
			end, ok := bracketEnd(p, i, emptyCompiles)
			if !ok {
				return 0, false
			}
			i = end
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// alternatives splits a group's body on the `|` between its arms, ignoring the
// ones inside a nested group or a bracket expression.
func alternatives(body string, emptyCompiles bool) []string {
	out, _ := alternativesAt(body, 0, emptyCompiles)
	return out
}

// alternativesAt is alternatives with each arm's offset in the pattern, which
// is what a nested group inside an arm needs to know its own number.
//
// The bracket expression is stepped over whole, exactly as [topAlternatives]
// does it and by the same scan — a `|` between two members of a bracket is a
// member and not a split, and so is every `(` and `)` in there. The comment
// above has said so since it was written and the code did not, which cost
// this (#3075): `([a|b])` was split into `([a` and `b])`, the first arm
// carried an unterminated `[`, and the whole pattern was refused. The
// parentheses inside are the half that makes it invisible — in
// `([][()|*?^#~<>])` the `(` and `)` inside the bracket balance, so the depth
// counter is back at nought when the `|` arrives and the split looks correct.
//
// Measured 2026-09-15 with `[[ $s == ${~p} ]]`, each probe in a script file
// of its own. bash 5.3.20 and bash 3.2.57 with `extglob`, ksh93u+ and zsh
// 5.9.2 all match `a` against `@([a|b])` and `|` against `@([]|])`; dash and
// BusyBox ash have neither `[[ ]]` nor the group, so six of seven columns
// agree and the seventh cannot be asked. A plain bug, not an axis.
func alternativesAt(body string, at int, emptyCompiles bool) (arms []string, offsets []int) {
	depth, start := 0, 0
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			depth--
		case '[':
			// An unterminated `[` is not a bracket expression and its text
			// is ordinary, so a `|` behind one still splits — which is what
			// skipBracket answers by standing still.
			i = skipBracket(body, i, emptyCompiles)
		case '|':
			if depth == 0 {
				arms, offsets = append(arms, body[start:i]), append(offsets, at+start)
				start = i + 1
			}
		}
	}
	return append(arms, body[start:]), append(offsets, at+start)
}

// matchGroup matches a group and whatever follows it.
//
// Every arm is tried against every split of the subject, because a group that
// matches more than one length can only be resolved by what comes after it:
// `+(a)b` against `aab` needs the group to stop before the b.
func matchGroup(body string, gp, lead int, quant byte, rest string, rp int, s string, at int, o patternOpts) bool {
	return matchGroupTimes(body, gp, lead, quant, boundOf(quant), true, rest, rp, s, at, o)
}

// matchGroupTimes is matchGroup with how many repetitions of the group are
// still allowed, and whether this is the group's first attempt rather than a
// later round of the same one.
//
// b counts *down* as the recursion goes round: what reaches a recursive call
// is what is left after the repetition the caller just matched, which is what
// makes a finite ceiling terminate. An unbounded one never moves.
//
// first is what keeps the stop-here branch from being asked twice for the
// same text. A caller that is about to recurse has already tried `rest` at
// exactly this position with exactly these captures in hand — that is the
// line above the recursion — so asking again inside it is a repeat of a call
// that has just failed. It was asked twice before this, for `*`, and the
// answer was the same both times.
func matchGroupTimes(body string, gp, lead int, quant byte, b repeatBound, first bool, rest string, rp int, s string, at int, o patternOpts) bool {
	// The body opens one byte past the `(`, and lead is whatever stands in
	// front of that parenthesis: nought for a bare group, one for a
	// quantifier, and the width of the braces for a written count. Derived
	// from `quant != 0` until a count could be five characters wide — a
	// group's own position is what a capture is numbered by, so an arm that
	// thinks it starts five bytes early reports the wrong span.
	bp := gp + lead + 1
	arms, armAt := o.where.armsOf(body, bp, o.emptyBracket)
	// Inside a group a bare `(` is a group again, in both dialects that
	// have groups at all: `@(a|(b))` matches `b` on ksh93u+. Only the arms
	// get this — what follows the group is the branch the count was read
	// in. See patternOpts.noBareGroup.
	armOpts := o
	armOpts.noBareGroup = false
	// `!(…)` is the odd one: it matches any text the arms do *not*, so it is
	// answered by asking the ordinary question and inverting it rather than
	// by trying the arms one at a time.
	if quant == '!' {
		lo, hi := splitFloor(rest, s, rp, &o), len(s)
		if o.periodHere(s, at) {
			// A negation is not a written period either, so the only text
			// it may take here is none.
			lo, hi = 0, 0
		}
		for i := lo; i <= hi; i++ {
			mark := o.where.caps.mark()
			if !matchesAnyArm(arms, armAt, s[:i], at, armOpts) && matchHere(rest, s[i:], rp, at+i, o) {
				// The text the negation consumed is what the group matched,
				// and a surface that records every group wants it: measured
				// 2026-09-19, `[[ abcd == a!(z)cd ]]` records `abcd` and then
				// `b`. Nothing inside the arms can have written a span here —
				// they were asked whether they match, and the answer taken is
				// that they do not.
				o.where.caps.record(gp, at, at+i)
				return true
			}
			o.where.caps.rollback(mark)
		}
		return false
	}
	repeat := b.mayRepeat()
	if first && b.mayStopHere() {
		// No repetition is required, so the rest may start here.
		mark := o.where.caps.mark()
		if matchHere(rest, s, rp, at, o) {
			return true
		}
		o.where.caps.rollback(mark)
	}
	// Arm-major, and the longest split of each arm first. Which combination
	// wins decides nothing about *whether* the pattern matches and
	// everything about what a `(#b)` reports, and both halves are measured
	// on zsh 5.9.2: `[[ abc == (#b)(a|ab)* ]]` reports `a` while
	// `[[ abc == (#b)(ab|a)* ]]` reports `ab`, so a written arm beats a
	// longer one; and `[[ aabab == (#b)(a*)b ]]` reports `aaba`, so within
	// one arm the group takes as much as it can and still leave the rest a
	// match.
	//
	// The split runs down to **zero**, which is what lets one repetition of
	// an arm that matches no text stand for the whole group: `@(|a)b`
	// matches `b` in every shell that has the construct, and so does the
	// unquantified `(|a)b` in the one shell that has *that*. It was a case
	// of its own — "can an arm match nothing" asked before the loop — while
	// the loop started at one character, and folding it in is only safe
	// because the loop counts down: an empty match reached at i == 0 is the
	// last thing tried rather than the first.
	//
	// Every attempt is bracketed by a mark, so a group that matched down a
	// branch the subject later left does not keep its span. That is the
	// invariant the whole file relies on: anything here that can *write* a
	// capture also unwinds it when its own attempt fails, which is what
	// lets a failed matchHere be treated as having written nothing.
	// The smallest split that still leaves `rest` a piece it could fill,
	// asked once for the whole loop below rather than per arm: it is a fact
	// about what follows the group and not about the group. See splitFloor.
	//
	// **Not for a repeating group**, and that is the whole of its
	// precondition: where `*` or `+` lets the group go round again, what
	// follows one repetition is the group *and* the rest, so a split that
	// leaves `rest` more than it can fill is exactly the split another
	// repetition eats the difference out of. `+(a)b` against `aab` is the
	// row that says so — bounding it left one `a` for `b` to match and the
	// pattern stopped matching.
	if !b.mayTakeARepetition() {
		// The ceiling is nought, so the group may not take even one. Only a
		// written count reaches this: `{0,0}(a)` matches the empty subject
		// on the branch above and `a` on no branch at all.
		return false
	}
	floor := 0
	if !repeat {
		floor = splitFloor(rest, s, rp, &o)
	}
	for k, a := range arms {
		// Every split down from the furthest this arm could possibly reach,
		// and no further down than the rest of the pattern can still reach
		// back. Starting at len(s) instead asks about splits no arm can
		// take, and the memo makes each of those cheap rather than free —
		// which is what made a trim over a long value quadratic. See
		// patternReach.
		for i := splitCeiling(a, s, armAt[k], &o); i >= floor; i-- {
			mark := o.where.caps.mark()
			if !matchHere(a, s[:i], armAt[k], at, armOpts) {
				o.where.caps.rollback(mark)
				continue
			}
			// What is left of the bound once this repetition has matched,
			// and whether the rest of the pattern may start here is *its*
			// question rather than b's. Written as an unconditional attempt
			// until a floor above one could exist: `{2}(a)` against `a`
			// matched one repetition and then handed the empty remainder to
			// an empty rest, which is every quantifier's right and no
			// count's. The four spellings all leave a floor of nought here,
			// so nothing that existed before this moves.
			after := b.afterOne()
			if after.mayStopHere() && matchHere(rest, s[i:], rp, at+i, o) {
				o.where.caps.record(gp, at, at+i)
				return true
			}
			// A repetition has to consume something, or the recursion
			// would not terminate.
			if repeat && i > 0 &&
				matchGroupTimes(body, gp, lead, quant, after, false, rest, rp, s[i:], at+i, o) {
				o.where.caps.record(gp, at, at+i)
				return true
			}
			o.where.caps.rollback(mark)
		}
	}
	return false
}

// matchesAnyArm reports whether any arm matches the whole of s. Only the
// negated quantifier needs it — every other branch has to know *which* arm
// and at what split, because that is what a capture reports.
func matchesAnyArm(arms []string, armAt []int, s string, at int, o patternOpts) bool {
	for k, a := range arms {
		if matchHere(a, s, armAt[k], at, o) {
			return true
		}
	}
	return false
}

// matchBracket consumes a bracket expression from p and reports whether c is
// in it, returning what is left of the pattern.
func matchBracket(p string, c string, o *patternOpts) (rest string, ok bool) {
	i := 1
	negate := false
	// `!` is the portable negation, everywhere. `^` is an extension dash
	// does not have, where it is an ordinary character — so whether it
	// negates is the caller's answer rather than this file's. Assuming it
	// did made `[^abc]` match the complement under the dash dialect, where
	// dash matches a literal caret.
	if i < len(p) && (p[i] == '!' || (p[i] == '^' && o.caret)) {
		negate = true
		i++
	}
	matched := false
	// frozen is a bracket whose scan gave up part-way, which is what a class
	// name the shell has not got does in two of the three readings. Nothing
	// after it counts, and the *negation* does not survive it either: see the
	// `]` below and Semantics.UnknownCharacterClass.
	frozen := false
	// sub says the scan consumed a bracket sub-expression — a `[:name:]`, a
	// `[.x.]` or a `[=x=]`. It matters only where the bracket then never
	// closes, and it is the whole question there: the `]` a sub-expression
	// ends with is its own, so a bracket that looks closed to the eye is
	// open, and what the text is instead is a different answer from what a
	// bare `[` is. See Semantics.UnterminatedBracketAfterASubExpression.
	sub := false
	// first is what keeps a `]` written straight away from ending the
	// expression, and in one column it does end it — but only where nothing
	// later would have. Asked of the whole bracket rather than of the byte,
	// because that is the noun: `[]a]` is a two-member set in every column
	// and `[]` is a set with no members in one of them.
	first := !o.emptyBracket || memberReadingCloses(p, 0)
	for i < len(p) {
		if p[i] == ']' && !first {
			i++
			if frozen && !matched {
				// The scan gave up before anything matched, and there is
				// nothing for a `!` to invert: measured, `[!a[:nope:]b]`
				// matches `q` where the name is inert and matches nothing
				// where the scan stops. A match found *before* the name was
				// reached still goes through the negation, which is the
				// other half — `[!a[:nope:]b]` matches no `a` anywhere.
				return p[i:], false
			}
			if negate {
				return p[i:], !matched
			}
			return p[i:], matched
		}
		first = false

		// A character class, [[:digit:]] and friends.
		//
		// The `:]` is looked for **after** the opening `[:`, so the colon
		// that opens one cannot also be the colon that closes it. Searching
		// from the `[` instead made `[[:]` a class whose name ran from
		// offset 3 to offset 2 and panicked the shell — reachable from
		// every surface, `[[ ":" == [[:] ]]` included. It is not a class at
		// all: measured 2026-09-07, `[[ ":" == [[:] ]]` matches and
		// `[[ x == [[:] ]]` does not, in bash 5.3.15 and zsh 5.9.2 alike,
		// so the four characters are a bracket holding `[` and `:`.
		if strings.HasPrefix(p[i:], "[:") {
			sub = true
			if end := strings.Index(p[i+2:], ":]"); end < 0 {
				// Nothing closes the name, so there is no name: `[[:]` holds
				// four characters and no class. What that leaves is the
				// dialect's — see Semantics.UnterminatedCharacterClass for
				// the five readings and #1431 for the panic this used to be.
				switch o.unterminatedClass {
				case UnterminatedClassEndsTheScan:
					frozen = true
				case UnterminatedClassEmptiesTheBracket:
					frozen, matched = true, false
				case UnterminatedClassSwallowsTheClosingBracket:
					// The `]` is taken as part of the name still being
					// looked for, so this bracket expression never ends and
					// the text is whatever an unterminated one is here.
					return unterminatedBracket(p, c, o, sub)
				}
				// UnterminatedClassIsOrdinaryCharacters falls through: the
				// `[` and the `:` are members like any other, which is what
				// the scan below makes of them with no arm of its own.
			} else {
				name := p[i+2 : i+2+end]
				i += 2 + end + 2
				if !classKnown(name, o.classes) {
					// A name this shell has not got — `[[:nope:]]`, and the
					// empty `[[::]]` with it, which every column answers the
					// same way. Three readings, measured 2026-09-12 with
					// `[a[:nope:]b]`: `a` and `b` both match, only `a`
					// matches, or neither does.
					switch o.unknownClass {
					case UnknownClassEndsTheScan:
						frozen = true
					case UnknownClassEmptiesTheBracket:
						frozen, matched = true, false
					}
					// UnknownClassIsInert is the third, and it is the one
					// with nothing to do: the name holds no character and
					// the scan carries on past it.
					continue
				}
				// foldClass and not fold, because the option that folds a
				// literal and the range beside it stops here and only the
				// inline `~(i)` flag carries on — see foldClass for the
				// table. A range is the neighbor that does fold under both,
				// and folds wide: `[[ K == [a-z] ]]` matches, the Kelvin
				// sign by way of its ASCII lower case.
				//
				// The fold here stays ASCII whatever the locale says. The
				// one shell that reaches this arm folds a class by the same
				// C-library predicate it folds a literal with, and widening
				// it on the strength of that would be reading a locale
				// question nobody measured (#2644).
				if !frozen &&
					(inClass(name, c, o.classes) ||
						(o.foldClass && inClass(name, swapUnitCase(c, false), o.classes))) {
					matched = true
				}
				continue
			}
		}

		// One member of the set, which is one unit of the *pattern* — a
		// whole character where the subject's units are — or the unit a
		// backslash protects. A multi-byte character's bytes are all above
		// ASCII, so none of them can be mistaken for the `-` of a range or
		// the `]` that ends the expression, and the scan above stays a byte
		// scan.
		if o.readsCollating() && i+1 < len(p) && p[i] == '[' && (p[i+1] == '.' || p[i+1] == '=') {
			sub = true
		}
		lo, next, read := bracketMember(p, i, o)
		if !read {
			// A `[.x.]` or `[=x=]` this shell cannot read as one collating
			// element, which in the C locale is any body but a single
			// character. It is [Semantics.UnknownCharacterClass]'s question
			// and not a second one: measured 2026-09-16 with
			// `[a[.nosuch.]b]`, each column answers it exactly as it answers
			// a `[:name:]` it has not got.
			switch o.unknownClass {
			case UnknownClassEndsTheScan:
				frozen = true
			case UnknownClassEmptiesTheBracket:
				frozen, matched = true, false
			}
			if next > i {
				// A closed body, so there is an element to step over. The
				// inert column steps over it holding nothing, which is what
				// keeps `[a[.nosuch.]b]` matching a and b and no letter of
				// the body.
				//
				// Stepped over as a *member* and not by leaving the loop,
				// which is the half of #3607 the low bound is: a `-` behind
				// it is still a range's operator there, so `[[.nosuch.]-c]`
				// is a range from nothing and not the two ordinary members
				// `-` and `c`. Measured 2026-09-18, and no column matches
				// either of them.
				lo = ""
			} else {
				// Nothing closed it, so there is no element and no length to
				// skip. Where the reading is inert the delimiter's characters
				// are members like any other — `[[.a]` is the three-member set
				// `[`, `.`, `a` in bash — which is what falls through here, with
				// the frozen columns carrying their freeze past it.
				lo, next, _ = plainBracketMember(p, i, o)
			}
		}
		// A `-` is literal at the end, which is why `[a-]` matches a dash.
		if next+1 < len(p) && p[next] == '-' && p[next+1] != ']' {
			hi, after, read := bracketMember(p, next+1, o)
			if !read {
				// The other half of #3607: a high bound that is not an
				// element is the same unknown body as one standing on its
				// own, and it reaches the dialect's answer for that rather
				// than being handed to the range as the nothing it holds.
				// Measured 2026-09-18 — `[a-[.nosuch.]]` matches nothing at
				// all in ksh93 and dash, `a` included, where this engine
				// built the range anyway and matched every character above
				// the low end in all three columns.
				switch o.unknownClass {
				case UnknownClassEndsTheScan:
					frozen = true
				case UnknownClassEmptiesTheBracket:
					frozen, matched = true, false
				}
				if after == next+1 {
					hi, after, _ = plainBracketMember(p, next+1, o)
				}
			}
			// Ranked rather than compared as text: `[a-é]` has to hold ç,
			// which is between them by code point and is not between them
			// byte for byte.
			from, to := ordOf(lo), ordOf(hi)
			if !frozen && (inRange(ordOf(c), from, to) ||
				(o.fold && inRange(ordOf(swapUnitCase(c, o.foldWide)), from, to))) {
				matched = true
			}
			i = after
			continue
		}
		if !frozen && eqUnit(lo, c, o.fold, o.foldWide) {
			matched = true
		}
		i = next
	}
	// An unterminated bracket is not a bracket expression, and what it is
	// instead is the dialect's answer rather than this file's.
	return unterminatedBracket(p, c, o, sub)
}

// unterminatedBracket is what text that opened a bracket and never closed one
// is instead.
//
// A function of its own because there are two ways to arrive at it: reading to
// the end of the pattern without finding a `]`, and — in one column — meeting
// a `[:` that nothing closes, which takes the `]` as part of the name it is
// still looking for. See Semantics.UnterminatedCharacterClass. Folded rather
// than written twice, because a second copy is how the two answers drift.
func unterminatedBracket(p, c string, o *patternOpts, sub bool) (rest string, ok bool) {
	switch o.bracketPolicy(sub) {
	case BracketLiteral:
		// bash and ksh93: an ordinary `[`, and the rest of the pattern
		// carries on from just after it.
		return p[1:], c == "["
	case BracketBadPattern:
		// zsh: not a pattern at all. Recorded rather than returned, because
		// matchHere recurses and a second result would have to be threaded
		// through every branch of it.
		if o.bad != nil {
			*o.bad = true
		}
		return "", false
	}
	// dash, and the shape the matcher had before any of this: a class that
	// can never match.
	return "", false
}

// bracketMember reads one member of a bracket expression at i and reports
// where it ends.
//
// A backslash protects the unit behind it and is not itself a member: `[\)]`
// is the one-character set `)` in all six panel shells, measured 2026-09-07
// on the two routes that can ask — a member written escaped in the source,
// and a member the source *quoted*, which patternOf hands the matcher as an
// escape because escaping is the only channel quoting has. Reading the
// backslash as a member instead admitted it to every such set, so
// `[[ "a\" == a[\)] ]]` answered yes where real zsh answers no (#1407).
//
// Which characters an escape reaches is the dialect's answer and not this
// function's, so it is asked rather than assumed — the same call matchBranch
// makes outside a bracket expression, which is what keeps one escape rule in
// one place. Where the escape does not reach, the backslash is a member of
// its own and the character behind it takes its own turn, exactly as it does
// in the rest of the pattern.
//
// The protection reaches a range bound too, and that is measured rather than
// assumed either: `[a\-z]` is the three members a, `-` and z everywhere in
// the panel, where `[a-z]` is the range — the escape is what stops the dash
// being read as the operator. A bound that needed no protection keeps its
// range, so `[\a-z]` is still a through z.
// The third result says the member was read. It is false only where the
// pattern opened a `[.` or a `[=` this shell has the construct for and the
// text behind it is not one collating element — a body of more than one
// character, an empty one, or a delimiter nothing closes. The caller decides
// what that does to the bracket around it, which is the dialect's answer and
// not this function's; where nothing closed the delimiter, next comes back
// unmoved, which is how the caller tells the two apart.
func bracketMember(p string, i int, o *patternOpts) (unit string, next int, read bool) {
	if o.readsCollating() && i+1 < len(p) && p[i] == '[' && (p[i+1] == '.' || p[i+1] == '=') {
		// The closer is looked for **after** the opening delimiter, exactly
		// as a class name's `:]` is, so the `.` that opens one cannot also
		// be the `.` that closes it and `[[..]]` is a body of nothing rather
		// than a body of one period.
		closer := string(p[i+1]) + "]"
		end := strings.Index(p[i+2:], closer)
		if end < 0 {
			return "", i, false
		}
		body, after := p[i+2:i+2+end], i+2+end+2
		// One collating element is one character in the C locale, which is
		// every element the column that reads one has. A longer body is a
		// **name** where the dialect reads names and is a body that is not an
		// element everywhere else; the column that finds an element in no
		// body at all takes neither arm.
		switch o.collating {
		case ACollatingElementMayBeNamed:
			if named, ok := collatingElementNamed(body); ok {
				return named, after, true
			}
			fallthrough
		case OneCharacterIsACollatingElement:
			if body != "" && o.unitWidth(body) == len(body) {
				return body, after, true
			}
		}
		return "", after, false
	}
	return plainBracketMember(p, i, o)
}

// readsCollating reports whether the delimiters of a `[.` or a `[=` are read
// as a sub-expression at all, which is what decides whether the `]` inside
// one ends the bracket. True of every reading but the column that has neither
// construct — including the one that reads them and finds an element in no
// body, where the difference from an ordinary bracket is visible without any
// element ever being a member.
func (o *patternOpts) readsCollating() bool {
	return o.collating != NoCollatingElements &&
		o.collating != CollatingElementsUnspecified
}

// plainBracketMember is one ordinary member: the unit at i, or the unit a
// backslash protects.
//
// Split out so that the collating delimiters above can fall back to it
// character by character where the dialect reads an unclosed one as ordinary
// text, rather than a second copy of the escape rule growing beside the
// first.
func plainBracketMember(p string, i int, o *patternOpts) (unit string, next int, read bool) {
	if p[i] == '\\' && i+1 < len(p) && !o.bracketMember && o.escapeReaches(p[i+1]) {
		w := o.unitWidth(p[i+1:])
		return p[i+1 : i+1+w], i + 1 + w, true
	}
	w := o.unitWidth(p[i:])
	return p[i : i+w], i + w, true
}

// hasBracketEscape reports whether a pattern holds a backslash inside a
// bracket expression, which is the only place [Semantics.BracketEscape]
// decides anything — so an ordinary `[a-z]`, and a backslash standing
// anywhere else in the pattern, put no question to the dialect.
func hasBracketEscape(p string, emptyCompiles bool) bool {
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '\\':
			i++
		case '[':
			end, ok := bracketEnd(p, i, emptyCompiles)
			if !ok {
				return false
			}
			if strings.Contains(p[i:end], `\`) {
				return true
			}
			i = end
		}
	}
	return false
}

// hasUnterminatedBracket reports whether a pattern contains a `[` with no
// closing `]`, so the axis is asked only about patterns it applies to.
func hasUnterminatedBracket(p string, emptyCompiles bool) bool {
	for i := 0; i < len(p); i++ {
		if p[i] == '\\' {
			i++
			continue
		}
		if p[i] == '[' && !closesBracket(p, i, emptyCompiles) {
			return true
		}
	}
	return false
}

// bracketAfterSubIsABadPattern reports whether this dialect refuses a pattern
// outright because a bracket in it was left open by a sub-expression.
//
// The axis first and the scan second, which is [Runner.badPatternFromAnOpenGroup]'s
// arrangement and for its reason: the scan costs a walk of the pattern and
// only one of the three readings has anything to do with it. Asked against
// the value rather than through [Runner.bracketAfterSubPolicy] so that a
// dialect which has not answered is told once, by the matcher, about a
// pattern that really poses the question — and not here about every pattern
// with a `[` in it.
func (r *Runner) bracketAfterSubIsABadPattern(p string) bool {
	return r.sem().UnterminatedBracketAfterASubExpression == BracketBadPattern &&
		bracketLeftOpenBySubExpression(p, r.readsCollatingElements())
}

// readsCollatingElements is whether `[.x.]` and `[=x=]` are one element here,
// read without asking — the scan above needs the answer for a pattern that
// may hold neither, and Runner.collatingElements reports an unanswered
// dialect, which is the matcher's to do once.
func (r *Runner) readsCollatingElements() bool {
	p := r.sem().CollatingElements
	return p != NoCollatingElements && p != CollatingElementsUnspecified
}

// bracketLeftOpenBySubExpression reports whether a pattern holds a bracket a
// **sub-expression** left open — a `[:name:]`, a `[.x.]` or a `[=x=]` whose
// own `]` is not the bracket's, so `[[:alpha:]` looks closed and is not.
//
// It is the companion to [hasUnterminatedBracket] and it exists because that
// scan cannot see this shape: `closesBracket` stops at the class's own `]`,
// which is exactly the byte the matcher steps over. Only a scan that reads
// the sub-expression knows, and until one existed the verdict could be asked
// for only from inside the match — so *whether the shell refused depended on
// the value*, which is not a distinction the reference draws (#4659).
//
// collating says whether this dialect reads `[.x.]` and `[=x=]` as one
// element at all. It is a parameter and not a read of the vector because the
// answer is the dialect's and this file names no shell — and it matters:
// measured 2026-09-26 on zsh 5.9.2 (`-f -c`), `v=zzz; ${v#x[[.a.]}` is `zzz`
// at status 0 there, because a shell with no collating elements closes that
// bracket at the `]` it can see. The same text in a column that reads one
// would leave the bracket open.
//
// **A sub-expression nothing closes is not this question**: `x[[:alpha}` has
// no `:]` in it, and what that means is
// [Semantics.UnterminatedCharacterClass]'s four readings rather than this
// one. The scan stops and answers false there, which leaves that axis the
// only thing deciding — measured, the reference refuses it and so do we,
// by the other road.
func bracketLeftOpenBySubExpression(p string, collating bool) bool {
	for i := 0; i < len(p); i++ {
		if p[i] == '\\' {
			i++
			continue
		}
		if p[i] != '[' {
			continue
		}
		open, sub, decided := bracketRunsOff(p, i, collating)
		if decided && open && sub {
			return true
		}
	}
	return false
}

// bracketRunsOff walks the bracket expression opened at i the way the matcher
// walks it, and reports whether it ran off the end, whether a sub-expression
// was stepped over on the way, and whether the scan reached an answer at all.
//
// The third result is what keeps this from speaking for questions that are
// not its own: a sub-expression nothing closes hands the rest of the pattern
// to a different axis, so the scan says so rather than guessing.
func bracketRunsOff(p string, i int, collating bool) (open, sub, decided bool) {
	j := i + 1
	if j < len(p) && (p[j] == '!' || p[j] == '^') {
		j++
	}
	if j < len(p) && p[j] == ']' {
		// The bracket's own spelling rather than its contents — the same
		// first-member rule closesBracket has.
		j++
	}
	for j < len(p) {
		switch {
		case p[j] == '\\':
			j += 2
		case p[j] == ']':
			return false, sub, true
		case strings.HasPrefix(p[j:], "[:"):
			end := strings.Index(p[j+2:], ":]")
			if end < 0 {
				return false, false, false
			}
			sub = true
			j += 2 + end + 2
		case collating && j+1 < len(p) && p[j] == '[' && (p[j+1] == '.' || p[j+1] == '='):
			end := strings.Index(p[j+2:], string(p[j+1])+"]")
			if end < 0 {
				return false, false, false
			}
			sub = true
			j += 2 + end + 2
		default:
			j++
		}
	}
	return true, sub, true
}

// inClass answers one character-class name for one unit of a subject.
//
// The twelve POSIX names first, since they are the ones every shell answers
// and the ones nearly every pattern uses; then whatever else the dialect
// declared. A name in neither set matches nothing and says nothing, at status
// 0, which is measured across the whole panel — see [Semantics.PatternClasses].
// classKnown reports whether the shell has a character class of this name at
// all, which is a different question from whether a character is in it: an
// unknown name is the axis Semantics.UnknownCharacterClass answers, and a
// known one that simply does not hold this character is nothing at all.
//
// The empty name counts as unknown, which is measured: `[a[::]b]` answers
// exactly as `[a[:nope:]b]` does in every column.
func classKnown(name string, extra patternClasses) bool {
	return posixClassName(name) || classDeclared(extra.names, name)
}

// posixClassName is the roster inPosixClass answers, as names. Written out
// beside it rather than derived from it, because "is this a class" and "is
// this character in it" are different questions and only the first can be
// asked without a character to ask it about.
func posixClassName(name string) bool {
	switch name {
	case "alnum", "alpha", "blank", "cntrl", "digit", "graph",
		"lower", "print", "punct", "space", "upper", "xdigit":
		return true
	}
	return false
}

func inClass(name string, unit string, extra patternClasses) bool {
	if inPosixClass(name, unit) {
		return true
	}
	return extra.holds(name, unit)
}

// patternClasses is a dialect's extra character-class names together with the
// shell state the two dynamic ones read.
//
// The state is captured when the pattern is read rather than looked up when a
// name is answered, because the matcher recurses and memoizes: a class whose
// answer moved part-way through one match would make the memo lie.
type patternClasses struct {
	// names is the roster, space separated, exactly as the dialect wrote it.
	names string
	// ifs is `$IFS` as it stands, with the default already applied where the
	// name is unset — `[[:IFS:]]` reads the separators a shell would split
	// on, not a fixed set.
	ifs string
	// word is `$WORDCHARS`: the characters that join letters and digits into
	// one word. `[[:WORD:]]` is the union of the two.
	word string
	// space is the whitespace half of IFS, as the dialect draws it — see
	// Semantics.IFSWhitespaceIsEverySpaceCharacter. Empty is the POSIX
	// three, which is what every caller that never asked gets.
	space string
	// asciiNames says a name is ASCII only, so `[[:IDENT:]]` is too. See
	// Semantics.NamesTakeTheLocalesLetters.
	asciiNames bool
}

// holds answers one of the extra names, and answers false for any name the
// dialect did not declare — including a POSIX name, which never reaches here.
//
// The names are compared byte for byte and are case-sensitive: measured,
// `[[:ident:]]` and `[[:ASCII:]]` match nothing where `[[:IDENT:]]` and
// `[[:ascii:]]` match.
func (c patternClasses) holds(name, unit string) bool {
	if unit == "" || !classDeclared(c.names, name) {
		return false
	}
	switch name {
	case "ascii":
		// One byte below 0x80. A character outside ASCII is not in it in
		// either shell that has the name: measured, `é` and `日` are both a
		// miss where `a` is a hit.
		return len(unit) == 1 && unit[0] < 0x80
	case "IDENT":
		// The characters a parameter name may hold: letters, digits and the
		// underscore. Letters and digits rather than ASCII ones — measured,
		// `é`, `日` and `٣` are all in it, which is the alnum answer this
		// matcher already gives.
		//
		// Unless a name is ASCII only, where the class follows it: measured
		// 2026-10-02 on 5.9.2, `[[ é = [[:IDENT:]] ]]` is false under
		// `setopt posix_identifiers` and true without it (#5153).
		if c.asciiNames && (len(unit) != 1 || unit[0] >= 0x80) {
			return false
		}
		return unit == "_" || inPosixClass("alnum", unit)
	case "WORD":
		// A word to the line editor: the same letters and digits, plus
		// whatever `$WORDCHARS` names. Measured dynamic — `WORDCHARS='@%'`
		// puts `@` and `%` in it and takes `-` and `.` out.
		return inPosixClass("alnum", unit) || unitIn(c.word, unit)
	case "IFS":
		// The field separators as they stand, so `IFS=':x'` puts those two
		// characters in it and takes the space out.
		return unitIn(c.ifs, unit)
	case "IFSSPACE":
		// The separators that are also whitespace, which is the half of IFS
		// a run of counts as one delimiter.
		return len(unit) == 1 && isIFSWhitespace(unit[0], c.space) && unitIn(c.ifs, unit)
	case "INCOMPLETE", "INVALID":
		// A byte that is not a character. A unit is one of these only when
		// it stands alone and is above ASCII: a lead byte that could have
		// begun a character is INCOMPLETE, and anything else — a
		// continuation byte with no lead, an overlong lead, a lead above the
		// last legal one — is INVALID. Measured a byte at a time: 0xC2
		// through 0xF4 are INCOMPLETE and 0x80 through 0xC1 and 0xF5 upward
		// are INVALID.
		if len(unit) != 1 || unit[0] < 0x80 {
			return false
		}
		lead := unit[0] >= 0xC2 && unit[0] <= 0xF4
		return lead == (name == "INCOMPLETE")
	}
	return false
}

// classDeclared reports whether a space-separated roster holds a name.
//
// The comparison is exact, and a mutation that folds case here survives: the
// switch above is the second gate and compares the name exactly too, so
// `[[:ASCII:]]` is still a miss with either. Recorded rather than tightened —
// the roster is what bounds the set, and the switch is what reads a name.
func classDeclared(names, name string) bool {
	if names == "" || name == "" {
		return false
	}
	for rest := names; rest != ""; {
		var one string
		if i := strings.IndexByte(rest, ' '); i >= 0 {
			one, rest = rest[:i], rest[i+1:]
		} else {
			one, rest = rest, ""
		}
		if one == name {
			return true
		}
	}
	return false
}

// unitIn reports whether a set of characters holds one whole unit. Written as
// a walk rather than as strings.Contains so that a multi-byte unit cannot be
// found straddling two characters of the set.
func unitIn(set, unit string) bool {
	for i := 0; i < len(set); {
		n := characterWidth(set[i:], "")
		if set[i:i+n] == unit {
			return true
		}
		i += n
	}
	return false
}

// ifsSpacePosix is the whitespace half of IFS as POSIX draws it, and
// ifsSpaceEvery is the same half as the C locale's `isspace` draws it. Which
// of the two a dialect uses is Semantics.IFSWhitespaceIsEverySpaceCharacter;
// see Runner.ifsSpace, which is the only place that chooses.
const (
	ifsSpacePosix = " \t\n"
	ifsSpaceEvery = " \t\n\v\f\r"
)

// isIFSWhitespace is the whitespace half of IFS: the characters a run of
// which counts as one field separator, and which are discarded at either end
// of a value.
//
// space is the set the dialect draws that half from, and an empty one is the
// POSIX three — so a caller with no dialect to ask behaves as it always did.
// One predicate rather than the five open-coded `c == ' ' || c == '\t' ||
// c == '\n'` tests this rule used to be spelled as: each of those was a
// separate place for the answer to be wrong, and one of them was (#4170).
func isIFSWhitespace(c byte, space string) bool {
	if space == "" {
		space = ifsSpacePosix
	}
	return strings.IndexByte(space, c) >= 0
}

// inPosixClass answers the POSIX character classes, over bytes, in the C
// locale the corpus is measured under. All twelve are here and unanimous
// across the panel.
func inPosixClass(name string, unit string) bool {
	if len(unit) > 1 {
		return inWideClass(name, ordOf(unit))
	}
	c := unit[0]
	switch name {
	case "digit":
		return isDigit(c)
	case "alpha":
		return isLetter(c)
	case "alnum":
		return isLetter(c) || isDigit(c)
	case "upper":
		return c >= 'A' && c <= 'Z'
	case "lower":
		return c >= 'a' && c <= 'z'
	case "space":
		return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
	case "punct":
		return c > ' ' && c < 127 && !isLetter(c) && !isDigit(c)
	case "xdigit":
		return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
	case "blank":
		return c == ' ' || c == '\t'
	case "cntrl":
		return c < ' ' || c == 0x7f
	case "graph":
		return c > ' ' && c < 0x7f
	case "print":
		return c >= ' ' && c < 0x7f
	}
	return false
}

// inWideClass answers a class for a character outside ASCII.
//
// Measured 2026-09-05 under `LC_ALL=C.UTF-8`, and this is where the panel is
// least uniform. bash 5.3, bash 3.2 and zsh agree on every class and every
// character tried: `é` is alpha, alnum, lower, print and graph; `É` swaps
// lower for upper; `日` is alpha, alnum, print and graph; `·` is punct, print
// and graph. dash answers none of them, having no decoder. ksh93 answers
// **alpha and nothing else** for all three letters and nothing at all for the
// punctuation, which is a partial implementation rather than a different
// reading of the classes — this follows the three that agree, and the corpus
// records ksh93's answer beside it.
//
// digit is **false** and not unicode.IsNumber, which is measured and is the
// standard's answer: POSIX XBD 7.3.1 says the digit class holds only the digits
// 0 through 9 in every locale. It reads as a deviation on this machine, where
// `case ٣ in [[:digit:]]` is a hit in bash 5.3 and 3.2 and a miss in ksh93,
// zsh and dash — and that is what #956 filed. It is not one. **The class a
// character falls in outside ASCII is the host C library's table**, and the
// same probe run against glibc has bash answering no with everybody else.
// Measured 2026-09-18 on bash 5.2.37 and zsh 5.9 in a Debian container against
// bash 5.3.20 and zsh 5.9.2 here, each cell macOS · glibc:
//
//	                        bash    zsh     ksh93   here
//	digit, xdigit  ٣ ５   Y · n   n · n   n · n   n
//	alnum          ٣ ５   Y · Y   Y · Y   Y · Y   Y
//	alpha          ٣ ５   n · Y   n · Y   Y · Y   n
//	alnum, alpha   Ⅷ      n · Y   n · Y   n · Y   n
//
// bash's and zsh's rows move with the C library and ksh93's alpha does not,
// which is the tell: the two that ask iswctype are the two that changed their
// answer when the library changed. So an axis here would record which machine
// the oracle ran on, and the tables below are this shell's own answer in every
// locale — glibc bash's on the class the panel was said to split over, macOS
// bash's on the rest, and the standard's throughout. docs/spec/semantics.md
// carries the measurement under *The classes a character falls in outside
// ASCII* (#956).
//
// **xdigit splits identically and this comment used to say it could not.**
// Measured 2026-09-12: `case ٣ in [[:xdigit:]]` is a hit in bash 5.3 and 3.2
// and a miss in ksh93, zsh and dash — so the sentence below claiming no
// character outside ASCII is in it "in the shells measured" was true of three
// columns and never checked against the fourth. It is still absent from the
// table, which keeps it answering false and keeps it agreeing with digit; the
// two classes are one question and the paragraph above is its answer. blank and cntrl are
// absent for the original reason, which does hold: Go's unicode tables would
// put characters in cntrl that no shell here does.
//
// alnum is **IsLetter or IsDigit** and deliberately not IsLetter or IsNumber.
// IsNumber is Nd, Nl and No together, so every roman numeral, vulgar fraction
// and superscript digit was alphanumeric here (#2465). Measured 2026-09-12
// under LC_ALL=C.UTF-8, one character per Unicode category, and the two
// categories do not answer alike:
//
//	              bash 5.3/3.2  ksh93  zsh  dash  ash
//	`½` U+00BD No    no          no     no   no    no
//	`Ⅷ` U+2167 Nl    no          no     no   no    YES
//	`٣` U+0663 Nd    alnum       no     alnum no   alnum
//
// So No is unanimous and Nl is five columns to one — BusyBox ash is the
// exception, and it is the column no hand-run probe on this machine can
// reach, which is why it was found by the corpus row and not by the five
// shells that are here. IsDigit is Nd alone, which matches every column on No,
// six of seven on Nl, and leaves the Nd row exactly where it was: still alnum,
// which is bash's, zsh's and ash's answer and is the residue #956 owns.
//
// It also stopped this shell disagreeing with itself in a way none of the
// panel does — alpha said no to `Ⅷ`, digit said no, and alnum said yes.
//
// ksh93 and ash are both wider than the rest on alpha rather than merely
// narrower on digit, which is worth having written down before anyone reads
// either as the conservative column: `Ⅷ` and `٣` are alpha in both and alpha
// in no other member.
//
// Corpus: `pat/alnum-outside-ascii-is-a-letter-or-a-decimal-digit`.
//
// graph excludes a space where print does not, which is the one place the two
// part company: a non-breaking space is print in bash and zsh and graph in
// neither, and Go counts it Graphic, so the space has to be taken back out.
func inWideClass(name string, c rune) bool {
	switch name {
	case "alpha":
		return unicode.IsLetter(c)
	case "digit":
		return false
	case "alnum":
		return unicode.IsLetter(c) || unicode.IsDigit(c)
	case "upper":
		return unicode.IsUpper(c)
	case "lower":
		return unicode.IsLower(c)
	case "space":
		return unicode.IsSpace(c)
	case "punct":
		return unicode.IsPunct(c) || unicode.IsSymbol(c)
	case "graph":
		return unicode.IsGraphic(c) && !unicode.IsSpace(c)
	case "print":
		return unicode.IsGraphic(c)
	}
	return false
}

// inRange reports whether a unit ranks inside a bracket range.
func inRange(c, lo, hi rune) bool { return c >= lo && c <= hi }

// swapUnitCase is the other case of a whole unit, or the unit itself.
//
// wide is patternOpts.foldWide: with it off this is swapCase over one byte,
// which is every unit a byte-counting locale has and the only fold this
// matcher used to do. With it on a multi-byte unit swaps too, which is what
// `[[ ÉTÉ == été ]]` needs under a UTF-8 locale.
//
// A unit that is not one whole character is handed back as itself. That is
// the undecodable byte that characters() passes through, and folding it would
// be inventing a letter where the subject holds a byte.
func swapUnitCase(unit string, wide bool) string {
	if len(unit) == 1 {
		return string(swapCase(unit[0]))
	}
	if !wide {
		return unit
	}
	c, size := utf8.DecodeRuneInString(unit)
	if size != len(unit) || c == utf8.RuneError {
		return unit
	}
	swapped := swapRuneCase(c)
	if swapped == c {
		return unit
	}
	return string(swapped)
}

func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// patternOpts resolves the dialect's pattern answers for one pattern.
//
// The bracket axis is deliberately not resolved here, and the reason is not
// the one this said for a long time. It said an unterminated bracket is
// literal in every shell this path serves, which is false and was disproved
// by a table twenty lines above Semantics.UnterminatedBracket itself: the
// panel splits three ways over `${w#[}`. What is true is narrower — the
// **callers** differ over which moment resolves it. `case` and `[[ ]]` ask it
// through matchPatternR, a parameter expansion's operand through
// operandPatternOpts, and pathname expansion refuses the whole field before
// it ever gets here, with the one exception that keeps `[ a = a ]` working.
// So the axis is resolved by whoever knows which of those this is, and the
// literal here is what is left when nobody asked (#4646).
//
// subjects are the strings this pattern is about to be matched against, and
// they are wanted for one reason: whether a unit is a character is a question
// about both sides. `?` is one ASCII byte and still consumes a whole
// character of the subject, so a caller that named only the pattern would
// leave the axis unasked in exactly the case that needs it.
func (r *Runner) patternOpts(pattern string, subjects ...string) patternOpts {
	// Every surface that reaches here — pathname expansion, the pattern
	// operators of parameter expansion, and the builtins that take one —
	// exits 1 where the shell rejects a pattern. Measured on all three.
	return r.tildeModifierOpts(r.extendedPatternOpts(patternOpts{
		caret:             r.caretNegates(pattern),
		bracket:           BracketLiteral,
		unknownClass:      r.unknownClassPolicy(pattern),
		unterminatedClass: r.unterminatedClassPolicy(pattern),
		chars:             r.patternMatchCountsCharacters(pattern, subjects...),
		group:             r.lang().PatternAlternation,
		bareParenIsText:   r.lang().BarePatternGroupInsideAWord && !r.lang().PatternAlternation,
		topGroup:          r.lang().PatternTopLevelAlternation.ReadsATopLevelBar(false),
		quantified:        r.readsQuantifiedGroups(false),
		counted:           r.lang().CountedPatternGroup,
		// Pathname expansion reads a `~(K)` group and its escapes:
		// `~(K)a\db` names `a1b` in that shell. The parameter-expansion
		// operand turns this off again and `%` turns it back on — see
		// operandPatternOpts and trimWith.
		tildeGlobRead: true,
		numericRange:  r.numericRanges(),
		escapes:       r.sem().PatternEscapeReaches,
		bracketMember: r.bracketEscapeIsOnlyAMember(pattern),
		classes:       r.patternClasses(pattern),
		collating:     r.collatingElements(pattern),
		// The bracket axis above is deliberately not resolved on this path
		// and this one is, because the two are not the same question here.
		// A bare `[` reaching pathname expansion or a trim is literal in
		// every column this path serves; a bracket a sub-expression left
		// open is not — measured with a prefix trim, `${w#[[:alpha:]}` on
		// `[a` is empty in bash and `[a` in ksh93 and dash, where
		// `${w#[}` takes the `[` in bash and in ksh93 alike.
		askBracketAfterSub: r.bracketAfterSubPolicy,
		// And whether a bracket the member reading cannot close is read as
		// an empty set rather than left open. Resolved here rather than
		// inside the matcher because it is the same answer the scan that
		// decides whether a word is a pattern at all already needs.
		emptyBracket: r.emptyBracketCompiles(),
	}, pattern, 1), pattern)
}

// hasCollatingDelimiter reports whether a pattern opens a `[.` or a `[=`
// inside a bracket expression.
//
// The bracket has to be found first: `a[.b` outside one is an ordinary `[`
// followed by a period in every column, and asking the axis about it would
// record a measurement the pattern never took.
func hasCollatingDelimiter(p string, emptyCompiles bool) bool {
	for i := 0; i < len(p); i++ {
		if p[i] == '\\' {
			i++
			continue
		}
		if p[i] != '[' {
			continue
		}
		// To the bracket's close, or to the end of the pattern where nothing
		// closes it — an unclosed `[.` is one of the shapes the axis
		// decides, so the scan must reach text no `]` stands behind.
		end := len(p)
		if e, ok := bracketEnd(p, i, emptyCompiles); ok {
			end = e
		}
		for j := i + 1; j+1 < end; j++ {
			if p[j] == '[' && (p[j+1] == '.' || p[j+1] == '=') {
				return true
			}
		}
		i = end
	}
	return false
}

// bracketEscapeIsOnlyAMember resolves [Semantics.BracketEscape] for the
// matcher, and only for a pattern that really holds a backslash inside a
// bracket expression.
func (r *Runner) bracketEscapeIsOnlyAMember(pattern string) bool {
	if !hasBracketEscape(pattern, r.emptyBracketCompiles()) {
		return false
	}
	return r.bracketEscape() == BracketEscapeIsOnlyAMember
}

// readsQuantifiedGroups reports whether `@(a|b)` is a group where this pattern
// stands, rather than a literal `@` and some parentheses.
//
// The context matters and cannot be folded away: one dialect reads them inside
// `[[ ]]` and nowhere else, so an expanded `(b)` is a group in a condition
// there and two ordinary characters in a `case`. Answering it globally made
// `p="(b)"; case b in $p` match, which that shell does not.
func (r *Runner) readsQuantifiedGroups(condition bool) bool {
	d := r.dialect()
	return d.ExtendedPattern || (condition && d.ExtendedPatternInCondition)
}

// patternClasses resolves the dialect's extra character-class roster, and the
// shell state two of the names read, for one pattern.
//
// Nothing is looked up unless the pattern actually opens a class. Asking for
// `$IFS` on every glob would be a variable lookup per directory listing for a
// question almost no pattern asks, and the guard is exact: `[:` is how a class
// begins and there is no other spelling.
func (r *Runner) patternClasses(pattern string) patternClasses {
	names := r.sem().PatternClasses
	if names == "" || !strings.Contains(pattern, "[:") {
		return patternClasses{}
	}
	c := patternClasses{names: names}
	if classDeclared(names, "IFS") || classDeclared(names, "IFSSPACE") {
		c.ifs, _ = r.ifs()
		c.space = r.ifsSpace(c.ifs)
	}
	if classDeclared(names, "WORD") {
		c.word, _ = r.getVar("WORDCHARS")
	}
	if classDeclared(names, "IDENT") {
		c.asciiNames = r.sem().NamesTakeTheLocalesLetters != Yes
	}
	return c
}

// unknownClassPolicy resolves Semantics.UnknownCharacterClass, and asks the
// axis only where the pattern actually holds a class name this shell has not
// got — the shape caretNegates uses, and for the same reason: a shell with no
// answer must not be refused over a question the pattern never poses.
//
// The roster is the dialect's, so the same name can be known in one shell and
// unknown in another. That is what makes this a question about the *pair* and
// not about a fixed list of names.
func (r *Runner) unknownClassPolicy(pattern string) UnknownClassPolicy {
	if !patternHasAnUnknownClass(pattern, r.patternClasses(pattern)) {
		return r.sem().UnknownCharacterClass
	}
	return r.unknownCharacterClass()
}

// unterminatedClassPolicy resolves Semantics.UnterminatedCharacterClass, and
// asks the axis only where the pattern actually holds a `[:` nothing closes —
// the shape unknownClassPolicy uses, and for the same reason: a shell with no
// answer must not be refused over a question the pattern never poses.
func (r *Runner) unterminatedClassPolicy(pattern string) UnterminatedClassPolicy {
	if !patternHasAnUnterminatedClass(pattern) {
		return r.sem().UnterminatedCharacterClass
	}
	return r.unterminatedCharacterClass()
}

// patternHasAnUnterminatedClass reports whether pattern holds a `[:` with no
// `:]` after it.
//
// It asks nothing about the roster, which is what separates it from
// patternHasAnUnknownClass: a name that never ends is not a name, so whether
// this shell has it cannot arise.
func patternHasAnUnterminatedClass(pattern string) bool {
	for i := 0; i+1 < len(pattern); i++ {
		if pattern[i] != '[' || pattern[i+1] != ':' {
			continue
		}
		end := strings.Index(pattern[i+2:], ":]")
		if end < 0 {
			return true
		}
		i += 2 + end + 1
	}
	return false
}

// patternHasAnUnknownClass reports whether pattern holds a closed `[:name:]`
// whose name is not one this shell has.
//
// The `:]` is looked for after the opening `[:`, which is matchBracket's own
// rule and has to be, or the two would disagree about which text is a name —
// see the comment there and #1431, which is the unterminated case this
// deliberately does not reach.
func patternHasAnUnknownClass(pattern string, classes patternClasses) bool {
	for i := 0; i+1 < len(pattern); i++ {
		if pattern[i] != '[' || pattern[i+1] != ':' {
			continue
		}
		end := strings.Index(pattern[i+2:], ":]")
		if end < 0 {
			continue
		}
		if !classKnown(pattern[i+2:i+2+end], classes) {
			return true
		}
		i += 2 + end + 1
	}
	return false
}

// liveMarkedPattern is a value holding live marks as pattern text: each marked
// character is left live and the rest is escaped. See liveMark.
func (r *Runner) liveMarkedPattern(v string) string {
	var b strings.Builder
	for {
		i := strings.Index(v, liveMark)
		if i < 0 || i+len(liveMark) >= len(v) {
			b.WriteString(escapePatternMetaIn(stripLiveMarks(v), r.markedMeta()))
			return b.String()
		}
		b.WriteString(escapePatternMetaIn(v[:i], r.markedMeta()))
		b.WriteByte(v[i+len(liveMark)])
		v = v[i+len(liveMark)+1:]
	}
}
