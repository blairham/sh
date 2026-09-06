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
const patternMeta = `*?[\()<`

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
// They are escaped even where they are *not* metacharacters, and that is not
// an oversight: an escaped ordinary character is that character, so the two
// spellings match the same text and no test could tell a guard here from its
// absence.
func escapePatternMeta(text string) string {
	if !strings.ContainsAny(text, patternMeta) {
		return text
	}
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		if strings.IndexByte(patternMeta, text[i]) >= 0 {
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
	if !strings.ContainsAny(v, patternMeta) {
		return v, true
	}
	return v, r.ask(glob, "globbing the result of an expansion")
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
}

// unitWidth is how many bytes of a non-empty subject one `?` consumes, one
// bracket matches, and one step of a `*` passes over.
func (o patternOpts) unitWidth(s string) int {
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
	return matchHere(pattern, s, o)
}

func matchHere(p, s string, o patternOpts) bool {
	for len(p) > 0 {
		if body, quant, rest, ok := splitGroup(p, o); ok {
			return matchGroup(body, quant, rest, s, o)
		}
		if lo, hi, rest, ok := splitNumericRange(p, o); ok {
			return matchNumericRange(lo, hi, rest, s, o)
		}
		switch p[0] {
		case '*':
			// Collapse a run of stars, then try every split point. The
			// shortest-first order does not matter: this answers whether a
			// match exists, not where it ends.
			for len(p) > 0 && p[0] == '*' {
				p = p[1:]
			}
			if p == "" {
				return true
			}
			// The split points are between units, not between bytes: a `*`
			// that stopped inside a character would hand the rest of the
			// pattern a subject beginning with a continuation byte, which a
			// following `?` would then take for a character of its own.
			for i := 0; ; i += o.unitWidth(s[i:]) {
				if matchHere(p, s[i:], o) {
					return true
				}
				if i == len(s) {
					return false
				}
			}

		case '?':
			if s == "" {
				return false
			}
			w := o.unitWidth(s)
			p, s = p[1:], s[w:]

		case '[':
			if s == "" {
				return false
			}
			w := o.unitWidth(s)
			rest, ok := matchBracket(p, s[:w], o)
			if !ok {
				return false
			}
			p, s = rest, s[w:]

		case '\\':
			// An escaped metacharacter is an ordinary character.
			if len(p) < 2 {
				return s == "\\"
			}
			if s == "" || !eqByte(p[1], s[0], o.fold) {
				return false
			}
			p, s = p[2:], s[1:]

		default:
			if s == "" || !eqByte(p[0], s[0], o.fold) {
				return false
			}
			p, s = p[1:], s[1:]
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
func splitNumericRange(p string, o patternOpts) (lo, hi int64, rest string, ok bool) {
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
func matchNumericRange(lo, hi int64, rest, s string, o patternOpts) bool {
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
		if matchHere(rest, s[k:], o) {
			return true
		}
	}
	return false
}

// splitGroup peels a group off the front of a pattern.
//
// quant is the character in front of it, or 0 for a bare group, which the
// dialect with bare groups treats as "exactly one" — the same as `@`.
func splitGroup(p string, o patternOpts) (body string, quant byte, rest string, ok bool) {
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
	var out []string
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
				out = append(out, body[start:i])
				start = i + 1
			}
		}
	}
	return append(out, body[start:])
}

// matchGroup matches a group and whatever follows it.
//
// Every arm is tried against every split of the subject, because a group that
// matches more than one length can only be resolved by what comes after it:
// `+(a)b` against `aab` needs the group to stop before the b.
func matchGroup(body string, quant byte, rest, s string, o patternOpts) bool {
	arms := alternatives(body)
	// `!(…)` is the odd one: it matches any text the arms do *not*, so it is
	// answered by asking the ordinary question and inverting it rather than
	// by trying the arms one at a time.
	if quant == '!' {
		for i := 0; i <= len(s); i++ {
			if !matchesAnyArm(arms, s[:i], o) && matchHere(rest, s[i:], o) {
				return true
			}
		}
		return false
	}
	if quant == '?' || quant == '*' {
		// Zero repetitions is allowed, so the rest may start here.
		if matchHere(rest, s, o) {
			return true
		}
	}
	// One repetition of an arm that matches no text is also no text, so a
	// group with such an arm may stand for nothing however it is quantified
	// — including not at all. `@(|a)b` matches `b` in every shell that has
	// the construct, and so does the unquantified `(|a)b` in the one shell
	// that has *that*; the split loop below starts at one character and
	// could never reach it.
	//
	// Asked as "can an arm match nothing" rather than "is an arm empty",
	// because `@(*)b` matches `b` too and the arm there is `*`. It is not
	// folded into the `?`/`*` branch above: those two allow zero
	// repetitions whatever the arms are, and this allows one repetition
	// that happens to consume nothing. Recursing here would not terminate,
	// which is the other reason it is a check and not an iteration.
	if matchesAnyArm(arms, "", o) && matchHere(rest, s, o) {
		return true
	}
	repeat := quant == '*' || quant == '+'
	for i := 1; i <= len(s); i++ {
		if !matchesAnyArm(arms, s[:i], o) {
			continue
		}
		if matchHere(rest, s[i:], o) {
			return true
		}
		if repeat && matchGroup(body, quant, rest, s[i:], o) {
			return true
		}
	}
	return false
}

func matchesAnyArm(arms []string, s string, o patternOpts) bool {
	for _, a := range arms {
		if matchHere(a, s, o) {
			return true
		}
	}
	return false
}

// matchBracket consumes a bracket expression from p and reports whether c is
// in it, returning what is left of the pattern.
func matchBracket(p string, c string, o patternOpts) (rest string, ok bool) {
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
	return patternOpts{
		caret:        r.caretNegates(pattern),
		bracket:      BracketLiteral,
		chars:        r.patternCountsCharacters(append([]string{pattern}, subjects...)...),
		group:        r.dialect().PatternAlternation,
		quantified:   r.readsQuantifiedGroups(false),
		numericRange: r.dialect().NumericRangePattern,
	}
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
