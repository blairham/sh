// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"
	"strconv"
	"strings"

	"github.com/blairham/sh/syntax"
)

// braceExpand turns one word into the words its braces produce.
//
// It runs *before* parameter expansion, which is measured and is why
// `a=1; echo {$a,2}` yields two words rather than one: the braces are
// resolved against the literal text `$a,2`, and only then does `$a` become 1.
//
// A *range* is where that ordering is a disagreement rather than a fact.
// bash keeps it — `n=3; echo {1..$n}` is the literal `{1..3}`, the range
// already gone by the time the variable exists — and zsh and ksh93 expand a
// range's endpoints first and read the range afterwards, so the same line is
// `1 2 3`. `BraceRangeEndpointsExpanded` is that axis, asked only where a
// range is actually written with an expansion in it.
//
// Apart from that it is purely textual: it never consults the filesystem and
// never fails. An unmatched or malformed brace is left alone, which is why
// `echo {a}` prints `{a}`.
//
// Only unquoted literal spans take part in the brace *syntax*. `"{a,b}"` is
// one word, because quoting is what decides whether text is syntax — the same
// rule that decides whether a `*` is a pattern. A quoted endpoint inside a
// range is not that, and is measured: `{1..'3'}` is `1 2 3` in both shells
// that expand endpoints at all, so the quotes hide the text from the brace
// scanner and not from the range.
//
// What comes out then goes back into the word, and *how* is the second
// disagreement: one shell hands it to the rest of word expansion as ordinary
// text, where `$var{x,y}` is the two names `$varx` and `$vary`, and the other
// two substitute the produced spans into the word the parse cut. See
// rereadBraceOutput and Semantics.BraceOutputRereadAsText — and note that
// braceCount below is unaffected either way, since the two readings differ
// about what the words are and never about how many.
func (r *Runner) braceExpand(w *syntax.Word) []*syntax.Word {
	return r.rereadBraceOutput(w, r.braceWords(w, true))
}

// braceCount is braceExpand asked only how many words the braces make, with
// a range's endpoints left unexpanded.
//
// A redirection needs the count, and it needs it about a word whose
// expansions have already been run once — avoiding a second run is the whole
// reason `expandRedirectTargetViews` exists — so a `> {1..$(f)}` counted with
// the endpoints live would run `f` twice. The suppressed count is also the
// measured answer: ksh93 expands endpoints in a word and *not* in a
// redirection target, writing to a file named `{1..2}`.
func (r *Runner) braceCount(w *syntax.Word) int {
	return len(r.braceWords(w, false))
}

// braceRangeShaped reports whether the word holds a matched group whose body
// is range-shaped and is *not* literal — the one shape braceCount is blind
// to, since it leaves a range's endpoints unexpanded and a `{1..$n}` counted
// that way is one word however many the range would make.
//
// It answers "could these braces make words", which is what a caller asks
// before it is worth running anything: it reads the spans and expands
// nothing, and a false positive costs only the questions the caller was
// already going to ask about a group. The scan enters every failed group
// rather than consulting BraceRescanEntersFailedGroup, because a predicate
// that refused a script over where to resume would be answering a question
// its caller has not reached yet.
func (r *Runner) braceRangeShaped(w *syntax.Word) bool {
	if w == nil {
		return false
	}
	from := cursor{0, 0}
	for {
		open, ok := findBraceFrom(w.Spans, from, '{')
		if !ok {
			return false
		}
		if close, matched := matchBraceAcross(w.Spans, open); matched {
			body := sliceSpans(w.Spans, next(open), close)
			if _, literal := literalBody(body); !literal && rangeShaped(body) {
				return true
			}
		}
		from = next(open)
	}
}

// braceWords is the whole of brace expansion, with a switch for whether a
// range's endpoints may be expanded on the way.
func (r *Runner) braceWords(w *syntax.Word, endpoints bool) []*syntax.Word {
	if w == nil {
		return nil
	}
	// A group that does not expand does not end the word. `{x}` has no comma
	// and is literal text in every shell, and the `{a,b}` behind it is still
	// a list: `@{x}{a,b}@` is two words everywhere braces expand at all. The
	// scan therefore carries on past a failed group rather than abandoning
	// the word, and how far it carries is the one thing the panel disagrees
	// about — BraceRescanEntersFailedGroup.
	from := cursor{0, 0}
	for {
		open, ok := findBraceFrom(w.Spans, from, '{')
		if !ok {
			return []*syntax.Word{w}
		}
		close, matched := matchBraceAcross(w.Spans, open)
		if matched {
			if alts, ok := r.alternativesAcross(w, open, close, endpoints); ok {
				return r.braceProduct(w, open, close, alts, endpoints)
			}
		}
		// The two readings only part where there is a brace *inside* the
		// group that failed — that is the whole of what one reading reaches
		// and the other steps over. A `{` behind the group is found either
		// way, and a word with no further brace at all is finished either
		// way, so the axis is not asked about `{a}` or `{a..5}`: a question
		// whose two answers are the same answer must not be the thing that
		// refuses a script no shell disagrees about.
		inner, hasInner := findBraceFrom(w.Spans, next(open), '{')
		diverges := hasInner && (!matched || before(inner, close))
		if diverges && r.askBrace(r.sem().BraceRescanEntersFailedGroup,
			"the scan entering a brace group that did not expand") {
			// bash and zsh resume one byte past the open brace, so a list
			// nested inside the failed group is still found.
			from = next(open)
			continue
		}
		if r.unspecified {
			return []*syntax.Word{w}
		}
		// ksh93 steps over the whole group instead, and has nowhere to
		// resume when the group was never closed.
		if !matched {
			return []*syntax.Word{w}
		}
		from = next(close)
	}
}

// braceProduct substitutes each alternative of the group between open and
// close back into the word, and expands what comes out.
//
// The alternative and the text behind the group are each expanded **on their
// own**, and the three pieces are joined afterwards. Joining first and
// re-reading the result would expand text that no brace in the script wrote:
// `{{1,2,3}..4}` is `{1..4} {2..4} {3..4}` in bash and zsh alike, where the
// group that produced `1` and the `..4}` behind it are both from the file but
// the `{1..4}` they spell between them is not. Measured 2026-09-22, and the
// same for `{6..{7,8,9}}` and `{{1,2,3}..{7,8,9}}` — this is not an axis, it
// is one pass over the word in every shell that expands braces at all.
func (r *Runner) braceProduct(w *syntax.Word, open, close cursor, alts [][]syntax.Span, endpoints bool) []*syntax.Word {
	before := sliceSpans(w.Spans, cursor{0, 0}, open)
	after := sliceSpans(w.Spans, next(close), cursor{len(w.Spans), 0})

	// The tail is read once rather than once per alternative: it is the same
	// text every time, and `{a,b}{c,d}` is four words either way.
	tails := r.braceWords(&syntax.Word{Spans: after, Start: w.Start, Stop: w.Stop}, endpoints)

	var out []*syntax.Word
	for _, alt := range alts {
		// The alternative is still read, so a group nested inside one — the
		// `{b,c}` of `{a,{b,c}}` — is a list and not text.
		for _, head := range r.braceWords(&syntax.Word{Spans: alt, Start: w.Start, Stop: w.Stop}, endpoints) {
			for _, tail := range tails {
				spans := append([]syntax.Span(nil), before...)
				spans = append(spans, head.Spans...)
				spans = append(spans, tail.Spans...)
				out = append(out, &syntax.Word{Spans: spans, Start: w.Start, Stop: w.Stop})
			}
		}
	}
	return out
}

// cursor is a position in a span list: which span, and how far into it.
//
// Braces are scanned across spans rather than within one, because a word is a
// sequence of them and a brace can straddle several: `{$a,2}` is a literal, a
// parameter expansion and another literal, and its braces are in the first and
// last. Scanning span by span would miss it, which is what made `echo {$a,2}`
// print `{1,2}` where every shell prints `1 2`.
type cursor struct{ span, off int }

func next(c cursor) cursor { return cursor{c.span, c.off + 1} }

// before reports whether one cursor comes strictly before another.
func before(a, b cursor) bool {
	return a.span < b.span || (a.span == b.span && a.off < b.off)
}

// braceable reports whether a span's text takes part in brace syntax. Only
// unquoted literals do: `"{a,b}"` is one word, because quoting decides whether
// text is syntax.
func braceable(s syntax.Span) bool {
	return s.Kind == syntax.Literal && s.Quoting == syntax.Unquoted
}

// findBraceFrom finds the next occurrence of c at or after the cursor from.
//
// It takes a cursor rather than a span index because resuming after a group
// that did not expand can land in the middle of a span: `{x}{a,b}` is one
// literal, and a scan that could only resume at the next span would find
// nothing after the first `{`.
func findBraceFrom(spans []syntax.Span, from cursor, c byte) (cursor, bool) {
	for i := from.span; i < len(spans); i++ {
		if !braceable(spans[i]) {
			continue
		}
		off := 0
		if i == from.span {
			off = from.off
		}
		if off > len(spans[i].Value) {
			continue
		}
		if j := strings.IndexByte(spans[i].Value[off:], c); j >= 0 {
			return cursor{i, off + j}, true
		}
	}
	return cursor{}, false
}

// matchBraceAcross finds the brace closing the one at open.
func matchBraceAcross(spans []syntax.Span, open cursor) (cursor, bool) {
	depth := 0
	for i := open.span; i < len(spans); i++ {
		if !braceable(spans[i]) {
			continue
		}
		start := 0
		if i == open.span {
			start = open.off
		}
		v := spans[i].Value
		for j := start; j < len(v); j++ {
			switch v[j] {
			case '\\':
				j++
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return cursor{i, j}, true
				}
			}
		}
	}
	return cursor{}, false
}

// alternativesAcross splits the body between open and close on top-level
// commas, returning each alternative as its own span list.
func (r *Runner) alternativesAcross(w *syntax.Word, open, close cursor, endpoints bool) ([][]syntax.Span, bool) {
	spans := w.Spans
	if alts, ok := r.rangeAcross(w, open, close, endpoints); ok {
		return alts, true
	}
	if spans[open.span].PidBrace {
		// The outer pair of a `{ … }` run written immediately after `$$` is
		// a *list* nowhere and a range wherever one is written, which is why
		// this stands below the range and not above it. Measured 2026-09-15
		// on zsh 5.9.2, the one column that has the construct, each probe in
		// a script file of its own:
		//
		//	$${a,b}        {a,b}                  one word
		//	$${1,2}        {1,2}                  one word
		//	$${1..3}       <pid>1 <pid>2 <pid>3   the range fired
		//	$${a..c}       <pid>a <pid>b <pid>c
		//	$${1..5..2}    <pid>1 <pid>3 <pid>5   a step too
		//	$${a,1..3}     {a,1..3}               a comma is still no range
		//	$${1..2 3}     {1..2 3}
		//
		// A reading that made the whole pair inert passed the first two rows
		// and lost the next three, and it was a test of this that found it.
		// The *contents* are ordinary spans either way, so the `{1..3}` and
		// the `{a,b}` nested inside `$${x{…}y}` both still expand — the note
		// is on the outer pair and on nothing else. See
		// [syntax.Span.PidBrace].
		return nil, false
	}
	var out [][]syntax.Span
	depth := 0
	from := next(open)
	for i := open.span; i <= close.span; i++ {
		if !braceable(spans[i]) {
			continue
		}
		v := spans[i].Value
		start, end := 0, len(v)
		if i == open.span {
			start = open.off + 1
		}
		if i == close.span {
			end = close.off
		}
		for j := start; j < end; j++ {
			switch v[j] {
			case '\\':
				j++
			case '{':
				depth++
			case '}':
				depth--
			case ',':
				if depth == 0 {
					out = append(out, sliceSpans(spans, from, cursor{i, j}))
					from = cursor{i, j + 1}
				}
			}
		}
	}
	if len(out) == 0 {
		// `{a}` is not a brace expression and is left alone.
		return nil, false
	}
	return append(out, sliceSpans(spans, from, close)), true
}

// rangeAcross expands `{n..m}`, either from the literal text between the
// braces or — where the endpoints are written as expansions and the dialect
// says so — from what those expansions come to.
func (r *Runner) rangeAcross(w *syntax.Word, open, close cursor, endpoints bool) ([][]syntax.Span, bool) {
	body := sliceSpans(w.Spans, next(open), close)
	pos := w.Spans[open.span].Pos
	if text, ok := literalBody(body); ok {
		alts, outcome := r.braceRange(text)
		switch outcome {
		case braceRangeCounted:
			return r.rangeSpans(alts, pos), true
		case braceRangeCollapsed:
			return collapsedRange(text, pos), true
		}
		return nil, false
	}
	if !endpoints || !rangeShaped(body) {
		return nil, false
	}
	if !r.askBrace(r.sem().BraceRangeEndpointsExpanded,
		"a brace range's endpoints expanding before the range is read") {
		return nil, false
	}
	// Once, whatever comes of it. A range that fails to form is put back as
	// the text the endpoints came to rather than as the word that produced
	// it, which is measured twice over: `{1..$(f)}` with a non-numeric `f`
	// runs `f` a single time, and the text it leaves is neither split nor
	// matched — ksh93 splits `$sp` on its own and leaves `{1..$sp}` whole.
	text := strings.Join(r.expandWordNoSplit(&syntax.Word{
		Spans: body, Start: w.Start, Stop: w.Stop,
	}), "")
	alts, outcome := r.braceRange(text)
	switch outcome {
	case braceRangeCounted:
		return r.rangeSpans(alts, pos), true
	case braceRangeCollapsed:
		return collapsedRange(text, pos), true
	}
	return [][]syntax.Span{{{
		Kind: syntax.Literal, Quoting: syntax.SingleQuoted,
		Value: "{" + text + "}", Pos: pos,
	}}}, true
}

// collapsedRange is the one alternative a range that lost its braces leaves:
// the body, as ordinary unquoted text.
//
// Unquoted, which is measured rather than tidy: with a file named `1..x` in
// the directory, `echo {1..}*` prints `1..x`, so what the braces left is a
// word like any other and is still a pattern. It is the half that separates
// this from the text a failed *expanded* endpoint leaves above, which is
// neither split nor matched.
func collapsedRange(text string, pos syntax.Pos) [][]syntax.Span {
	return [][]syntax.Span{{{Kind: syntax.Literal, Value: text, Pos: pos}}}
}

// literalBody gives the text of a brace body written entirely as unquoted
// literals, which is the only body a range can be read from without expanding
// anything. A quoted or substituted span makes it the other case.
func literalBody(body []syntax.Span) (string, bool) {
	var b strings.Builder
	for _, s := range body {
		if !braceable(s) {
			return "", false
		}
		b.WriteString(s.Value)
	}
	return b.String(), true
}

// rangeShaped reports whether a brace body is *written* as a range: a `..`
// outside any nested brace, and no comma out there.
//
// The comma decides, because a list beats a range wherever both readings fit:
// `{1..3,5}` is the two words `1..3` and `5` in bash, ksh93 and zsh alike.
// Asking first is also what keeps a list's expansions from being run twice —
// nothing here may expand a body that a range will not read.
func rangeShaped(body []syntax.Span) bool {
	depth, dots := 0, false
	for _, s := range body {
		if !braceable(s) {
			continue
		}
		v := s.Value
		for j := 0; j < len(v); j++ {
			switch v[j] {
			case '\\':
				j++
			case '{':
				depth++
			case '}':
				depth--
			case ',':
				if depth == 0 {
					return false
				}
			case '.':
				if depth == 0 && j+1 < len(v) && v[j+1] == '.' {
					dots = true
					j++
				}
			}
		}
	}
	return dots
}

// rangeSpans turns a range's elements into one-span alternatives.
//
// Quoted, which is the measured difference between a range and a list and is
// reached the moment a character range is wider than the letters. On zsh
// 5.9.2, 2026-09-14, in a directory holding `q` and `z`:
//
//	{=..?}   [=][>][?]     a range's `?` is a character
//	{?,x}    [q][z][x]     a list's `?` is a pattern
//
// So what a *range* counted is data and what a list held is text the word
// still has to expand — in the shells that put the output back as spans.
//
// This comment used to say bash answers the same way, on the same probe, and
// it is wrong about the backslash. Measured 2026-09-22 on bash 5.3.20:
//
//	printf '[%s]' {A..z}   …[Z][[][][]][^][_][`][a]…   the backslash is empty
//	                       zsh keeps it: …[Z][[][\][]]…
//
// bash rereads what the braces produced as shell **text**, so its elements are
// raw characters and the backslash reaches quote removal like any other
// unquoted one. That is BraceOutputRereadAsText, and it is why the quoting
// here is a question rather than a constant (#4200).
func (r *Runner) rangeSpans(alts []string, pos syntax.Pos) [][]syntax.Span {
	q := syntax.SingleQuoted
	if !inertElements(alts) && r.askBrace(r.sem().BraceOutputRereadAsText,
		"brace expansion's output re-entering the word as shell text") {
		// Except where the word re-reads what the braces produced, which is
		// the same question from the other end: there a range's elements are
		// raw characters in a string rather than data in a span, so the
		// backslash `{Z..a}` counts is an unquoted backslash and reaches
		// quote removal. See Semantics.BraceOutputRereadAsText.
		q = syntax.Unquoted
	}
	out := make([][]syntax.Span, 0, len(alts))
	for _, a := range alts {
		out = append(out, []syntax.Span{{
			Kind: syntax.Literal, Quoting: q, Value: a, Pos: pos,
		}})
	}
	return out
}

// inertElements reports whether every element of a counted range is text that
// means the same thing quoted and unquoted.
//
// It is what keeps Semantics.BraceOutputRereadAsText out of the way of the
// ranges anybody writes. `{1..10}`, `{a..z}` and `{01..12}` are the same words
// however their elements re-enter the word, so a dialect that has not answered
// that axis must not be refused over one; `{A..z}` counts the six characters
// between the cases, and that is where the readings part.
//
// The inert set is written out rather than the active one, because the two
// are not complements here: a character nobody has thought about has to land
// on the side that asks the question, not on the side that answers it.
func inertElements(alts []string) bool {
	for _, a := range alts {
		if strings.ContainsFunc(a, func(c rune) bool { return !inertInAWord(c) }) {
			return false
		}
	}
	return true
}

// inertInAWord reports whether a character stands for itself in an unquoted
// word in every dialect: a letter, a digit, and the punctuation that opens
// nothing, quotes nothing, matches nothing and ends no word.
func inertInAWord(c rune) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	return strings.ContainsRune("-+._/,", c)
}

// sliceSpans copies the spans between two cursors, splitting the ones at the
// ends.
func sliceSpans(spans []syntax.Span, from, to cursor) []syntax.Span {
	var out []syntax.Span
	for i := from.span; i <= to.span && i < len(spans); i++ {
		s := spans[i]
		if !braceable(s) {
			if i > from.span || from.off == 0 {
				out = append(out, s)
			}
			continue
		}
		start, end := 0, len(s.Value)
		if i == from.span {
			start = from.off
		}
		if i == to.span {
			end = to.off
		}
		if start > end {
			continue
		}
		if start == 0 && end == len(s.Value) {
			out = append(out, s)
			continue
		}
		if end > start {
			c := s
			c.Value = s.Value[start:end]
			out = append(out, c)
		}
	}
	return out
}

// braceOutcome is what a brace body read as a range came to. Three answers
// rather than two, because one shell takes the braces off a range it could
// not count and leaves the body standing as text — `{1..}` is `1..` — which
// is neither "the word as written" nor a list of elements.
type braceOutcome uint8

const (
	// braceNotARange leaves the word exactly as it was written.
	braceNotARange braceOutcome = iota
	// braceRangeCounted produced the elements below.
	braceRangeCounted
	// braceRangeCollapsed produced no elements and took the braces off.
	braceRangeCollapsed
)

// braceRange expands `{n..m}` and `{a..z}`, counting either way, with an
// optional `..step`.
//
// Three readings are tried in the order a shell reaches them, and the order
// is measured rather than chosen. The **numeric** reading comes first, which
// is what keeps `{1..2..08}` padded to `01` in the shell that pads: its
// endpoints are also single characters, so a character range tried first
// would have answered `1` and lost the width. The **character** reading comes
// next, which is what makes `{1...}` the range `1` to `.` rather than a body
// with a component missing. What is left is a body that is shaped like a
// range and cannot be counted, and that is where the panel parts three ways.
//
// The shells that expand braces at all disagree five times inside a range,
// and each disagreement is a named axis asked where it is reached: whether an
// endpoint's leading zeros pad the range (BraceRangePadsToEndpointWidth),
// what a written step's sign means (BraceRangeStepSignHonored, then
// BraceRangeNegativeStepReverses), what a range between two single characters
// spans (BraceCharRangeSpansAnyCharacter), and what a range that cannot be
// counted leaves behind (BraceRangeMissingEndCountsFromZero,
// BraceRangeZeroStepCountsAsOne and BraceRangeThatCannotBeCounted).
func (r *Runner) braceRange(body string) ([]string, braceOutcome) {
	lo, rest, ok := strings.Cut(body, "..")
	if !ok {
		return nil, braceNotARange
	}
	hi, stepText, hasStep := strings.Cut(rest, "..")

	if holdsADigit(lo) && holdsADigit(hi) && (!hasStep || holdsADigit(stepText)) {
		return r.numericRange(lo, hi, stepText, hasStep)
	}
	if alts, ok := r.charRange(body, lo, hi, stepText, hasStep); ok {
		return alts, braceRangeCounted
	}
	return r.rangeMissingAComponent(lo, hi, stepText, hasStep)
}

// numericRange counts between two written numbers.
func (r *Runner) numericRange(lo, hi, stepText string, hasStep bool) ([]string, braceOutcome) {
	if strings.ContainsRune(lo+hi+stepText, '+') &&
		!r.askBrace(r.sem().BraceRangeNumberMayCarryAPlus, "a range's number carrying a leading plus") {
		return nil, braceNotARange
	}
	if r.unspecified {
		return nil, braceNotARange
	}
	from, err1 := strconv.Atoi(lo)
	to, err2 := strconv.Atoi(hi)
	if err1 != nil || err2 != nil {
		return nil, braceNotARange
	}
	step, negStep := 1, false
	if hasStep {
		n, err := strconv.Atoi(stepText)
		if err != nil {
			return nil, braceNotARange
		}
		negStep = n < 0
		if n < 0 {
			n = -n
		}
		switch {
		case n != 0:
			step = n
		case r.askBrace(r.sem().BraceRangeZeroStepCountsAsOne, "a written step of zero counting as one"):
			// A step of zero is a walk that never arrives, so no shell takes
			// it at its word. One of them reads it as one and counts the
			// range anyway; the other two treat the body as a range they
			// could not count, and part again over what that leaves.
			negStep = false
		default:
			if r.unspecified {
				return nil, braceNotARange
			}
			return r.rangeThatCannotBeCounted()
		}
		if r.unspecified {
			return nil, braceNotARange
		}
	}
	width := 0
	if paddedEndpoint(lo) || paddedEndpoint(hi) {
		if r.askBrace(r.sem().BraceRangePadsToEndpointWidth, "an endpoint's leading zeros padding the range") {
			width = max(len(lo), len(hi))
		} else if r.unspecified {
			return nil, braceNotARange
		}
	}
	alts, ok := r.walkRange(from, to, step, hasStep, negStep,
		func(i int) string { return padNumber(i, width) })
	if !ok {
		return nil, braceNotARange
	}
	return alts, braceRangeCounted
}

// charRange counts between two single characters, which is two different
// readings rather than one with a wider edge — see
// Semantics.BraceCharRangeSpansAnyCharacter for the measurements.
//
// The two agree about a range between two single *letters* with no step
// written, which is how nearly every character range in the wild is spelled,
// so the axis is not asked there: a question whose two answers are the same
// answer must not be the thing that refuses a script.
func (r *Runner) charRange(body, lo, hi, stepText string, hasStep bool) ([]string, bool) {
	letters := len(lo) == 1 && len(hi) == 1 && isRangeLetter(lo[0]) && isRangeLetter(hi[0])
	loRune, hiRune, twoChars := r.charRangeEndpoints(body)
	renderRune := func(i int) string { return string(rune(i)) }
	if letters && !hasStep {
		// A letter range walks the code points, which is also what makes
		// `{a..C}` produce the punctuation between the cases — measured, not
		// chosen.
		return r.walkRange(int(lo[0]), int(hi[0]), 1, false, false, renderRune)
	}
	if !letters && !twoChars {
		// Neither reading reaches it, so neither has an opinion to ask for.
		return nil, false
	}
	if r.askBrace(r.sem().BraceCharRangeSpansAnyCharacter,
		"a range counting between two characters that are not both letters") {
		// The wider reading takes no step: the body is the two characters
		// and the `..` between them and nothing else, so `{a..z..2}` is not
		// a character range at all in the shell that reads it this way.
		if !twoChars {
			return nil, false
		}
		return r.walkRange(int(loRune), int(hiRune), 1, false, false, renderRune)
	}
	if r.unspecified || !letters {
		return nil, false
	}
	// The narrower reading is letters only, and it does take a step.
	n, err := strconv.Atoi(stepText)
	if err != nil {
		return nil, false
	}
	negStep := n < 0
	if n < 0 {
		n = -n
	}
	if n == 0 {
		n = 1
	}
	return r.walkRange(int(lo[0]), int(hi[0]), n, true, negStep, renderRune)
}

// charRangeEndpoints reads a body spelled as exactly one character, `..`, and
// one more character. Counting the characters rather than cutting at the
// first `..` is what the measurements say: `{....}` is the range from `.` to
// `.` and `{.....}` is left alone, which a cut at the first `..` gets the
// wrong way round.
//
// A *character* is the locale's, which is the same question a pattern asks
// and is asked the same way: on zsh 5.9.2, `{α..γ}` is `α β γ` under a UTF-8
// locale and the word as written under `LC_ALL=C`, where the endpoints are
// two bytes each and neither is one character.
func (r *Runner) charRangeEndpoints(body string) (lo, hi rune, ok bool) {
	if !isASCII(body) && !r.patternCountsCharacters(body) {
		return 0, 0, false
	}
	rs := []rune(body)
	if len(rs) != 4 || rs[1] != '.' || rs[2] != '.' {
		return 0, 0, false
	}
	return rs[0], rs[3], true
}

// rangeMissingAComponent answers a body whose components are not all numbers,
// which is where a range stops being arithmetic and becomes a question about
// what the shell leaves behind.
func (r *Runner) rangeMissingAComponent(lo, hi, stepText string, hasStep bool) ([]string, braceOutcome) {
	// One shell counts from zero when the *second* endpoint is the missing
	// one, and only then: a missing first endpoint or a missing step leaves
	// it with no range at all. Asked only where the first endpoint is a
	// number, since `{a..}` is left alone in every column.
	if hi == "" && (!hasStep || stepText != "") {
		if _, err := strconv.Atoi(lo); err == nil {
			if r.askBrace(r.sem().BraceRangeMissingEndCountsFromZero, "a missing second endpoint counting from zero") {
				return r.numericRange(lo, "0", stepText, hasStep)
			}
			if r.unspecified {
				return nil, braceNotARange
			}
		}
	}
	// What is left is the shape of a range with a gap in it, and the shape
	// is narrow: the first endpoint is a run of digits with no sign, and the
	// second endpoint and the step may each carry one. `{+1..2}` and
	// `{1..2..x}` are outside it and are left alone everywhere.
	last := hi
	if hasStep {
		last = stepText
	}
	if !digitRun(lo, false) || !digitRun(hi, true) || (hasStep && !digitRun(stepText, true)) {
		return nil, braceNotARange
	}
	// And a body with no digit at either end is left alone everywhere too,
	// so the axis is not asked about `{..}`, `{..2..}` or `{......}`.
	if !holdsADigit(lo) && !holdsADigit(last) {
		return nil, braceNotARange
	}
	return r.rangeThatCannotBeCounted()
}

// rangeThatCannotBeCounted resolves what a range-shaped body that produced no
// elements leaves behind.
func (r *Runner) rangeThatCannotBeCounted() ([]string, braceOutcome) {
	switch r.braceRangeFailure() {
	case BraceRangeFailureDropsTheBraces:
		return nil, braceRangeCollapsed
	default:
		return nil, braceNotARange
	}
}

// digitRun reports whether a range component is a run of decimal digits,
// possibly empty, with a leading `-` allowed where signed says so.
func digitRun(s string, signed bool) bool {
	if signed {
		s = strings.TrimPrefix(s, "-")
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// holdsADigit reports whether a range component has a decimal digit in it at
// all, which is what separates a component that is missing from one that is
// merely a sign.
func holdsADigit(s string) bool {
	return strings.ContainsFunc(s, func(c rune) bool { return c >= '0' && c <= '9' })
}

// walkRange produces a range's elements from one endpoint to the other. The
// endpoints decide the direction and the step contributes magnitude alone —
// except where a written sign says otherwise, which is a measured
// disagreement asked here rather than resolved by a constant.
func (r *Runner) walkRange(from, to, step int, hasStep, negStep bool, render func(int) string) ([]string, bool) {
	desc := to < from
	if hasStep && negStep != desc &&
		r.askBrace(r.sem().BraceRangeStepSignHonored, "a step's sign overriding the endpoints' direction") {
		// The walk leaves the first endpoint the way the sign says, which
		// here is away from the far one, so the range holds one element.
		return []string{render(from)}, true
	}
	if r.unspecified {
		return nil, false
	}
	dir := 1
	if desc {
		dir = -1
	}
	var out []string
	for i := from; (dir > 0 && i <= to) || (dir < 0 && i >= to); i += dir * step {
		out = append(out, render(i))
		// A range is bounded so a typo cannot hang the shell.
		if len(out) > 10000 {
			return nil, false
		}
	}
	if hasStep && negStep &&
		r.askBrace(r.sem().BraceRangeNegativeStepReverses, "a negative step reversing the range") {
		slices.Reverse(out)
	}
	if r.unspecified {
		return nil, false
	}
	return out, true
}

// askBrace asks a brace axis, but only in a dialect whose braces expand at
// all. When BraceExpansion is off or unanswered, whatever brace expansion
// produced is put back or refused by that outer axis, so a question that will
// never change an answer must not be the thing that refuses the script — dash
// prints `{01..3}` as written and is never asked what the zeros mean.
func (r *Runner) askBrace(a Answer, axis string) bool {
	// And the same for a shell whose braces are switched off at run time:
	// what the range came to is put back by the caller, so a range that will
	// never be used must not be the thing that refuses the script.
	if r.sem().BraceExpansion != Yes || r.noBraceExpand {
		return false
	}
	return r.ask(a, axis)
}

// isRangeLetter reports whether a byte can stand as a letter endpoint.
func isRangeLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// paddedEndpoint reports whether an endpoint was written with leading zeros,
// which is what turns the whole range on to padding.
func paddedEndpoint(s string) bool {
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimPrefix(s, "+")
	return len(s) > 1 && s[0] == '0'
}

// padNumber renders n at the given total width, zeros after the sign.
func padNumber(n, width int) string {
	s := strconv.Itoa(n)
	if len(s) >= width {
		return s
	}
	sign := ""
	if s[0] == '-' {
		sign, s = "-", s[1:]
	}
	return sign + strings.Repeat("0", width-len(sign)-len(s)) + s
}
