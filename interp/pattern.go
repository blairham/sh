// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"math"
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
	const meta = patternMeta + extendedPatternMeta
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
	var b strings.Builder
	for _, s := range w.Spans {
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

// patternSpan expands one span of a pattern, reporting whether the
// metacharacters in what comes back are live — a pattern — or ordinary text.
func (r *Runner) patternSpan(s syntax.Span) (text string, live bool) {
	switch s.Kind {
	case syntax.ParamExp:
		// The `${~spec}` flag reaches here too, and that is measured rather
		// than assumed: `p='a*'; [[ abc == ${~p} ]]` is true in the shell
		// that has the construct where `${p}` alone is false, and
		// `${v#${~p}}` trims where `${v#${p}}` does not. It is the same
		// question GlobExpansionResults answers, so it is the same override.
		return r.expansionPattern(r.expandParam(s.Param), s.Quoting, r.globSubstAnswer(s))
	case syntax.CommandSubst:
		return r.expansionPattern(r.commandSubst(r.ctx, s), s.Quoting, r.sem().GlobExpansionResults)
	case syntax.ArithSubst:
		v, ok := r.arithSpanValue(s)
		if !ok {
			return "", false
		}
		return r.expansionPattern(v, s.Quoting, r.sem().GlobExpansionResults)
	case syntax.ProcSubstIn, syntax.ProcSubstOut:
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
	return v, r.ask(glob, "globbing the result of an expansion")
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
	// A trial's answers are about *this* subject, so they are dropped with
	// the captures rather than carried into the next one.
	w.asked, w.dead = 0, nil
	if !matchHere(pattern, piece, 0, base, o) {
		return false, matchReport{}
	}
	span := capSpan{begin: base, end: base + len(piece), set: true}
	return true, w.caps.report(subject, span, w.plan.whole)
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
	k := matchKey{pp: pp, plen: len(p), at: at, slen: len(s), fold: o.litFold}
	if _, dead := w.dead[k]; dead {
		return false
	}
	if matchBranch(p, s, pp, at, o) {
		return true
	}
	if w.dead == nil {
		w.dead = make(map[matchKey]struct{})
	}
	w.dead[k] = struct{}{}
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
			if item, rest, ok := splitClosableItem(p, &o); ok {
				if lo, hi, after, isClosure := closureBounds(rest, &o); isClosure {
					return matchRepeat(item, pp, lo, hi, after, pp+len(p)-len(after), s, at, o)
				}
			}
		}
		if body, quant, rest, ok := splitGroup(p, &o); ok {
			return matchGroup(body, pp, quant, rest, pp+len(p)-len(rest), s, at, o)
		}
		if lo, hi, rest, ok := splitNumericRange(p, &o); ok {
			return matchNumericRange(lo, hi, rest, pp+len(p)-len(rest), s, at, o)
		}
		switch p[0] {
		case '*':
			// Collapse a run of stars, then try every split point. The
			// shortest-first order does not matter: this answers whether a
			// match exists, not where it ends.
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
			for i := 0; ; i += o.unitWidth(s[i:]) {
				mark := o.where.caps.mark()
				if matchHere(p, s[i:], pp, at+i, o) {
					return true
				}
				o.where.caps.rollback(mark)
				if i == len(s) {
					return false
				}
			}

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

// splitGroup peels a group off the front of a pattern.
//
// quant is the character in front of it, or 0 for a bare group, which the
// dialect with bare groups treats as "exactly one" — the same as `@`.
func splitGroup(p string, o *patternOpts) (body string, quant byte, rest string, ok bool) {
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
	end, found := closingParen(p[i:])
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
	arms, armAt := alternativesAt(body, bp)
	// `!(…)` is the odd one: it matches any text the arms do *not*, so it is
	// answered by asking the ordinary question and inverting it rather than
	// by trying the arms one at a time.
	if quant == '!' {
		for i := 0; i <= len(s); i++ {
			mark := o.where.caps.mark()
			if !matchesAnyArm(arms, armAt, s[:i], at, o) && matchHere(rest, s[i:], rp, at+i, o) {
				return true
			}
			o.where.caps.rollback(mark)
		}
		return false
	}
	if quant == '?' || quant == '*' {
		// Zero repetitions is allowed, so the rest may start here.
		mark := o.where.caps.mark()
		if matchHere(rest, s, rp, at, o) {
			return true
		}
		o.where.caps.rollback(mark)
	}
	repeat := quant == '*' || quant == '+'
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
	for k, a := range arms {
		for i := len(s); i >= 0; i-- {
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
	first := true
	for i < len(p) {
		if p[i] == ']' && !first {
			i++
			if negate {
				return p[i:], !matched
			}
			return p[i:], matched
		}
		first = false

		// A character class, [[:digit:]] and friends.
		if strings.HasPrefix(p[i:], "[:") {
			end := strings.Index(p[i:], ":]")
			if end >= 0 {
				if inClass(p[i+2:i+end], c) ||
					(o.fold && inClass(p[i+2:i+end], swapUnitCase(c))) {
					matched = true
				}
				i += end + 2
				continue
			}
		}

		// One unit of the *pattern*, which is a whole character where the
		// subject's units are. A multi-byte character's bytes are all above
		// ASCII, so none of them can be mistaken for the `-` of a range or
		// the `]` that ends the expression, and the scan above stays a byte
		// scan.
		lw := o.unitWidth(p[i:])
		lo := p[i : i+lw]
		// A `-` is literal at the end, which is why `[a-]` matches a dash.
		if i+lw+1 < len(p) && p[i+lw] == '-' && p[i+lw+1] != ']' {
			hw := o.unitWidth(p[i+lw+1:])
			hi := p[i+lw+1 : i+lw+1+hw]
			// Ranked rather than compared as text: `[a-é]` has to hold ç,
			// which is between them by code point and is not between them
			// byte for byte.
			from, to := ordOf(lo), ordOf(hi)
			if inRange(ordOf(c), from, to) ||
				(o.fold && inRange(ordOf(swapUnitCase(c)), from, to)) {
				matched = true
			}
			i += lw + 1 + hw
			continue
		}
		if eqUnit(lo, c, o.fold) {
			matched = true
		}
		i += lw
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

// inClass answers the POSIX character classes, over bytes, in the C locale
// the corpus is measured under. All twelve are here and unanimous across the
// panel. A name outside the twelve matches nothing, silently — the answer of
// every panel shell but bash 3.2, which falls back to reading the characters
// literally (see docs/spec/grammar/patterns.md).
func inClass(name string, unit string) bool {
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
// xdigit, blank and cntrl are deliberately absent: no character outside ASCII
// is in any of them in the shells measured, and Go's unicode tables would put
// characters in cntrl that none of them do.
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
		return unicode.IsLetter(c) || unicode.IsNumber(c)
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
		chars:        r.patternCountsCharacters(append([]string{pattern}, subjects...)...),
		group:        r.dialect().PatternAlternation,
		quantified:   r.readsQuantifiedGroups(false),
		numericRange: r.dialect().NumericRangePattern,
		escapes:      r.sem().PatternEscapeReaches,
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
