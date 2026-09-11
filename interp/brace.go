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
func (r *Runner) braceExpand(w *syntax.Word) []*syntax.Word {
	return r.braceWords(w, true)
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

// braceWords is the whole of brace expansion, with a switch for whether a
// range's endpoints may be expanded on the way.
func (r *Runner) braceWords(w *syntax.Word, endpoints bool) []*syntax.Word {
	if w == nil {
		return nil
	}
	open, ok := findBraceByte(w.Spans, 0, '{')
	if !ok {
		return []*syntax.Word{w}
	}
	close, ok := matchBraceAcross(w.Spans, open)
	if !ok {
		return []*syntax.Word{w}
	}
	alts, ok := r.alternativesAcross(w, open, close, endpoints)
	if !ok {
		return []*syntax.Word{w}
	}

	before := sliceSpans(w.Spans, cursor{0, 0}, open)
	after := sliceSpans(w.Spans, next(close), cursor{len(w.Spans), 0})

	var out []*syntax.Word
	for _, alt := range alts {
		spans := append([]syntax.Span(nil), before...)
		spans = append(spans, alt...)
		spans = append(spans, after...)
		// Recur, so `{a,b}{c,d}` and nested braces both work.
		out = append(out, r.braceWords(&syntax.Word{Spans: spans, Start: w.Start, Stop: w.Stop}, endpoints)...)
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

// braceable reports whether a span's text takes part in brace syntax. Only
// unquoted literals do: `"{a,b}"` is one word, because quoting decides whether
// text is syntax.
func braceable(s syntax.Span) bool {
	return s.Kind == syntax.Literal && s.Quoting == syntax.Unquoted
}

// findBraceByte finds the next occurrence of c at or after from.
func findBraceByte(spans []syntax.Span, fromSpan int, c byte) (cursor, bool) {
	for i := fromSpan; i < len(spans); i++ {
		if !braceable(spans[i]) {
			continue
		}
		if j := strings.IndexByte(spans[i].Value, c); j >= 0 {
			return cursor{i, j}, true
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
		alts, ok := r.braceRange(text)
		if !ok {
			return nil, false
		}
		return rangeSpans(alts, pos), true
	}
	if !endpoints || !rangeShaped(body) {
		return nil, false
	}
	if !r.askRange(r.sem().BraceRangeEndpointsExpanded,
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
	alts, ok := r.braceRange(text)
	if !ok {
		return [][]syntax.Span{{{
			Kind: syntax.Literal, Quoting: syntax.SingleQuoted,
			Value: "{" + text + "}", Pos: pos,
		}}}, true
	}
	return rangeSpans(alts, pos), true
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
func rangeSpans(alts []string, pos syntax.Pos) [][]syntax.Span {
	out := make([][]syntax.Span, 0, len(alts))
	for _, a := range alts {
		out = append(out, []syntax.Span{{Kind: syntax.Literal, Value: a, Pos: pos}})
	}
	return out
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

// braceRange expands `{n..m}` and `{a..z}`, counting either way, with an
// optional `..step`. A step of zero means one, so a typo cannot hang the
// shell.
//
// The shells that expand braces at all disagree twice inside a range, and
// each disagreement is a named axis asked where it is reached: whether an
// endpoint's leading zeros pad the range (BraceRangePadsToEndpointWidth),
// and what a written step's sign means (BraceRangeStepSignHonored, then
// BraceRangeNegativeStepReverses). The corpus cases behind the answers are
// `expand/brace-range-zero-padded` and the step-sign pair.
func (r *Runner) braceRange(body string) ([]string, bool) {
	lo, rest, ok := strings.Cut(body, "..")
	if !ok {
		return nil, false
	}
	hi, stepText, hasStep := strings.Cut(rest, "..")
	step := 1
	negStep := false
	if hasStep {
		n, err := strconv.Atoi(stepText)
		if err != nil {
			return nil, false
		}
		negStep = n < 0
		if n < 0 {
			n = -n
		}
		if n != 0 {
			step = n
		}
	}

	if len(lo) == 1 && len(hi) == 1 && isRangeLetter(lo[0]) && isRangeLetter(hi[0]) {
		// A letter range walks bytes, which is also what makes `{a..C}`
		// produce the punctuation between the cases — measured, not chosen.
		return r.walkRange(int(lo[0]), int(hi[0]), step, hasStep, negStep,
			func(i int) string { return string(rune(i)) })
	}

	from, err1 := strconv.Atoi(lo)
	to, err2 := strconv.Atoi(hi)
	if err1 != nil || err2 != nil {
		return nil, false
	}
	width := 0
	if paddedEndpoint(lo) || paddedEndpoint(hi) {
		if r.askRange(r.sem().BraceRangePadsToEndpointWidth, "an endpoint's leading zeros padding the range") {
			width = max(len(lo), len(hi))
		} else if r.unspecified {
			return nil, false
		}
	}
	return r.walkRange(from, to, step, hasStep, negStep,
		func(i int) string { return padNumber(i, width) })
}

// walkRange produces a range's elements from one endpoint to the other. The
// endpoints decide the direction and the step contributes magnitude alone —
// except where a written sign says otherwise, which is a measured
// disagreement asked here rather than resolved by a constant.
func (r *Runner) walkRange(from, to, step int, hasStep, negStep bool, render func(int) string) ([]string, bool) {
	desc := to < from
	if hasStep && negStep != desc &&
		r.askRange(r.sem().BraceRangeStepSignHonored, "a step's sign overriding the endpoints' direction") {
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
		r.askRange(r.sem().BraceRangeNegativeStepReverses, "a negative step reversing the range") {
		slices.Reverse(out)
	}
	if r.unspecified {
		return nil, false
	}
	return out, true
}

// askRange asks a brace-range axis, but only in a dialect whose braces
// expand at all. When BraceExpansion is off or unanswered, whatever a range
// produced is put back or refused by that outer axis, so a range that will
// never be used must not be the thing that refuses the script — dash prints
// `{01..3}` as written and is never asked what the zeros mean.
func (r *Runner) askRange(a Answer, axis string) bool {
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
