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
func escapePatternMeta(text string) string {
	const meta = patternMeta + extendedPatternMeta + bracketMeta
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
	spans := r.patternTilde(w, &b)
	for i, s := range spans {
		r.expandingSpan = i
		text, live := r.patternSpan(s)
		if live {
			b.WriteString(text)
			continue
		}
		// Quoted text, and the result of an expansion the dialect does not
		// re-read as a pattern, are literal: every metacharacter is escaped.
		b.WriteString(escapePatternMeta(text))
	}
	return b.String()
}

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
	dir, tail, ok := r.tildeSplit(s.Value)
	if !ok {
		return w.Spans
	}
	b.WriteString(escapePatternMeta(dir))
	spans := slices.Clone(w.Spans)
	spans[0].Value = tail
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
		s, tail := r.unreadBareSubscript(s)
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
		text, live := r.expansionPattern(r.expandParam(s.Param), s.Quoting, r.globSubstAnswer(s))
		if tail == nil {
			return text, live
		}
		// Two provenances in one span now, and only one flag to report them
		// with: whatever the value was worth is settled here, and what comes
		// back is the finished pattern.
		if !live {
			text = escapePatternMeta(text)
		}
		return text + r.bareSubscriptPattern(tail), true
	case syntax.CommandSubst:
		return r.expansionPattern(r.commandSubst(r.ctx, s), s.Quoting, r.sem().GlobExpansionResults)
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
		path, ok := r.procSub(r.ctx, s.Kind, s.Value)
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
// [Semantics.BracketEscapeIsAlsoAMember] records, and what one column does.
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
	if !r.sem().BracketEscapeIsAlsoAMember || !strings.Contains(v, `\`) {
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
		case v[i] == '[' && closesBracket(v, i):
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
	caret   bool
	bracket BracketPolicy
	// unknownClass is what a `[:name:]` the shell has never heard of does to
	// the bracket around it — see Semantics.UnknownCharacterClass. Read only
	// when a pattern actually holds one, so the zero value here is "no
	// bracket in this pattern asked".
	unknownClass UnknownClassPolicy
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
	// topGroup reads a `|` standing outside every group and bracket as an
	// alternation of the whole pattern, which one dialect does and only for
	// a bar that arrived live — see matchTopLevel. Separate from group for
	// the reason group and quantified are separate: the dialect that has
	// bare groups is not the only one that could have this, and the written
	// spelling is a parse error in every shell measured, so nothing but a
	// value can put one here.
	topGroup bool
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
	// extended says the operators one shell keeps behind an option of its
	// own are live: `(#…)` flag groups, the `#` and `##` closures, the `^`
	// negation and the `~` exclusion. Off, all four are ordinary characters,
	// which is measured — see interp/patternflags.go.
	extended bool
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
	return characterWidth(s)
}

// eqByte compares two bytes, without case when fold says so. ASCII only: the
// folding a shell does inside a pattern is `nocasematch` and its kin, which
// this implementation has never taken past ASCII, and which is its own
// measurement rather than this one's.
func eqByte(a, b byte, fold bool) bool {
	return a == b || (fold && swapCase(a) == b)
}

// eqUnit compares two whole units — one byte each, or one character each.
// Equal bytes are equal characters, so the multi-byte case needs nothing of
// its own beyond comparing the whole run.
func eqUnit(a, b string, fold bool) bool {
	if len(a) == 1 && len(b) == 1 {
		return eqByte(a[0], b[0], fold)
	}
	return a == b
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

func matchPattern(pattern, s string, o patternOpts) bool {
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
	w := o.where
	if w == nil {
		// A caller that built its options by hand rather than through
		// Runner.patternOpts. It asks for no flag, so the plan is empty and
		// this allocates once for the whole match rather than per trial.
		w = &matchWhere{}
		o.where = w
	}
	w.total, w.caps = len(subject), newCaptures(w.plan)
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
			w.prepare(pattern)
		}
	}
	if !matchTopLevel(pattern, piece, base, o) {
		return false, matchReport{}
	}
	span := capSpan{begin: base, end: base + len(piece), set: true}
	return true, w.caps.report(subject, span, w.plan.whole)
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
	arms, armAt := topAlternatives(pattern)
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
func topAlternatives(pattern string) (arms []string, offsets []int) {
	depth, start := 0, 0
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			depth--
		case '[':
			// Past the whole bracket expression, class names included:
			// measured, `L='[a|b]'` matches `a` and matches `|`, so a bar
			// between two members is not a split. An unterminated `[` is not
			// a bracket expression and its text is ordinary, which is what
			// the second answer says.
			if end, ok := bracketEnd(pattern, i); ok {
				i = end
			}
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
			if body, rest, ok := splitPatternFlags(p); ok {
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
		if body, quant, rest, ok := splitGroup(p, pp, &o); ok {
			return matchGroup(body, pp, quant, rest, pp+len(p)-len(rest), s, at, o)
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
			for len(p) > 0 && p[0] == '*' {
				p, pp = p[1:], pp+1
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
			if s == "" {
				return false
			}
			w := o.unitWidth(s)
			p, s, pp, at = p[1:], s[w:], pp+1, at+w

		case '[':
			if s == "" {
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
			if s == "" || !o.eqPatternByte(p[0], s[0]) {
				return false
			}
			p, s, pp, at = p[1:], s[1:], pp+1, at+1
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
func closingParen(p string) (int, bool) {
	depth := 0
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '\\':
			i++
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
func alternatives(body string) []string {
	out, _ := alternativesAt(body, 0)
	return out
}

// alternativesAt is alternatives with each arm's offset in the pattern, which
// is what a nested group inside an arm needs to know its own number.
func alternativesAt(body string, at int) (arms []string, offsets []int) {
	depth, start := 0, 0
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			depth--
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
func matchGroup(body string, gp int, quant byte, rest string, rp int, s string, at int, o patternOpts) bool {
	// The body opens one byte past the `(`, or two past it when a quantifier
	// stands in front of one.
	bp := gp + 1
	if quant != 0 {
		bp = gp + 2
	}
	arms, armAt := o.where.armsOf(body, bp)
	// `!(…)` is the odd one: it matches any text the arms do *not*, so it is
	// answered by asking the ordinary question and inverting it rather than
	// by trying the arms one at a time.
	if quant == '!' {
		for i := splitFloor(rest, s, rp, &o); i <= len(s); i++ {
			mark := o.where.caps.mark()
			if !matchesAnyArm(arms, armAt, s[:i], at, o) && matchHere(rest, s[i:], rp, at+i, o) {
				return true
			}
			o.where.caps.rollback(mark)
		}
		return false
	}
	repeat := quant == '*' || quant == '+'
	if quant == '?' || quant == '*' {
		// Zero repetitions is allowed, so the rest may start here.
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
			if !matchHere(a, s[:i], armAt[k], at, o) {
				o.where.caps.rollback(mark)
				continue
			}
			if matchHere(rest, s[i:], rp, at+i, o) {
				o.where.caps.record(gp, at, at+i)
				return true
			}
			// A repetition has to consume something, or the recursion
			// would not terminate.
			if repeat && i > 0 && matchGroup(body, gp, quant, rest, rp, s[i:], at+i, o) {
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
	first := true
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
			if end := strings.Index(p[i+2:], ":]"); end >= 0 {
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
				if !frozen &&
					(inClass(name, c, o.classes) ||
						(o.fold && inClass(name, swapUnitCase(c), o.classes))) {
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
		lo, next := bracketMember(p, i, o)
		// A `-` is literal at the end, which is why `[a-]` matches a dash.
		if next+1 < len(p) && p[next] == '-' && p[next+1] != ']' {
			hi, after := bracketMember(p, next+1, o)
			// Ranked rather than compared as text: `[a-é]` has to hold ç,
			// which is between them by code point and is not between them
			// byte for byte.
			from, to := ordOf(lo), ordOf(hi)
			if !frozen && (inRange(ordOf(c), from, to) ||
				(o.fold && inRange(ordOf(swapUnitCase(c)), from, to))) {
				matched = true
			}
			i = after
			continue
		}
		if !frozen && eqUnit(lo, c, o.fold) {
			matched = true
		}
		i = next
	}
	// An unterminated bracket is not a bracket expression, and what it is
	// instead is the dialect's answer rather than this file's.
	switch o.bracket {
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
func bracketMember(p string, i int, o *patternOpts) (unit string, next int) {
	if p[i] == '\\' && i+1 < len(p) && o.escapeReaches(p[i+1]) {
		w := o.unitWidth(p[i+1:])
		return p[i+1 : i+1+w], i + 1 + w
	}
	w := o.unitWidth(p[i:])
	return p[i : i+w], i + w
}

// hasUnterminatedBracket reports whether a pattern contains a `[` with no
// closing `]`, so the axis is asked only about patterns it applies to.
func hasUnterminatedBracket(p string) bool {
	for i := 0; i < len(p); i++ {
		if p[i] == '\\' {
			i++
			continue
		}
		if p[i] == '[' && !closesBracket(p, i) {
			return true
		}
	}
	return false
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
		return len(unit) == 1 && isIFSWhitespace(unit[0]) && unitIn(c.ifs, unit)
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
		n := characterWidth(set[i:])
		if set[i:i+n] == unit {
			return true
		}
		i += n
	}
	return false
}

// isIFSWhitespace is the whitespace half of IFS: the three characters a run of
// which counts as one field separator.
func isIFSWhitespace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' }

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
// digit is **false** and not unicode.IsNumber, which is measured and is where
// the panel splits again: `case ٣ in [[:digit:]]` is a hit in bash 5.3 and 3.2
// and a miss in ksh93, zsh and dash. POSIX says the digit class holds only the
// digits 0 through 9 in every locale, so the standard and three of the four
// agree, and bash is the one out. `[[:alnum:]]` still holds it, which is bash
// and zsh together. That leaves the bash dialect deviating from bash on this
// one class, which is #956 — either an axis or a decision written down, and
// not something to inherit from a comment.
//
// **xdigit splits identically and this comment used to say it could not.**
// Measured 2026-09-12: `case ٣ in [[:xdigit:]]` is a hit in bash 5.3 and 3.2
// and a miss in ksh93, zsh and dash — so the sentence below claiming no
// character outside ASCII is in it "in the shells measured" was true of three
// columns and never checked against the fourth. It is still absent from the
// table, which keeps it answering false and keeps it agreeing with digit; both
// classes are one question and #956 is where it is asked. blank and cntrl are
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

// swapUnitCase is swapCase over a whole unit. ASCII only, like swapCase: the
// folding in a pattern is `nocasematch`, which this implementation has never
// taken past ASCII.
func swapUnitCase(unit string) string {
	if len(unit) != 1 {
		return unit
	}
	return string(swapCase(unit[0]))
}

func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// patternOpts resolves the dialect's pattern answers for one pattern.
//
// The bracket axis is deliberately not resolved here: this is the path used by
// parameter expansion and by globbing, where an unterminated bracket is
// literal in every shell measured. Only `case` and `[[ ]]` ask it, through
// matchPatternR.
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
	return r.extendedPatternOpts(patternOpts{
		caret:        r.caretNegates(pattern),
		bracket:      BracketLiteral,
		unknownClass: r.unknownClassPolicy(pattern),
		chars:        r.patternCountsCharacters(append([]string{pattern}, subjects...)...),
		group:        r.dialect().PatternAlternation,
		topGroup:     r.dialect().PatternTopLevelAlternation,
		quantified:   r.readsQuantifiedGroups(false),
		numericRange: r.dialect().NumericRangePattern,
		escapes:      r.sem().PatternEscapeReaches,
		classes:      r.patternClasses(pattern),
	}, pattern, 1)
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
	}
	if classDeclared(names, "WORD") {
		c.word, _ = r.getVar("WORDCHARS")
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
