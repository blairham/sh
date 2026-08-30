// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"

	"github.com/blairham/sh/syntax"
)

// braceExpand turns one word into the words its braces produce.
//
// It runs *before* parameter expansion, which is measured and is why
// `a=1; echo {$a,2}` yields two words rather than one: the braces are
// resolved against the literal text `$a,2`, and only then does `$a` become 1.
// A brace range whose endpoints are variables therefore cannot work, in any
// shell.
//
// It is purely textual: it never consults the filesystem and never fails. An
// unmatched or malformed brace is left alone, which is why `echo {a}` prints
// `{a}`.
//
// Only unquoted literal spans take part. `"{a,b}"` is one word, because
// quoting is what decides whether text is syntax — the same rule that decides
// whether a `*` is a pattern.
func braceExpand(w *syntax.Word) []*syntax.Word {
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
	alts, ok := alternativesAcross(w.Spans, open, close)
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
		out = append(out, braceExpand(&syntax.Word{Spans: spans, Start: w.Start, Stop: w.Stop})...)
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
func alternativesAcross(spans []syntax.Span, open, close cursor) ([][]syntax.Span, bool) {
	if r, ok := rangeAcross(spans, open, close); ok {
		return r, true
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

// rangeAcross expands `{n..m}`, which only makes sense when the whole body is
// literal — a range with a variable endpoint cannot work in any shell,
// because braces resolve before the variable exists.
func rangeAcross(spans []syntax.Span, open, close cursor) ([][]syntax.Span, bool) {
	if open.span != close.span {
		return nil, false
	}
	body := spans[open.span].Value[open.off+1 : close.off]
	alts, ok := braceRange(body)
	if !ok {
		return nil, false
	}
	out := make([][]syntax.Span, 0, len(alts))
	for _, a := range alts {
		out = append(out, []syntax.Span{{Kind: syntax.Literal, Value: a, Pos: spans[open.span].Pos}})
	}
	return out, true
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

// braceRange expands `{n..m}`, counting either way.
func braceRange(body string) ([]string, bool) {
	lo, hi, ok := strings.Cut(body, "..")
	if !ok {
		return nil, false
	}
	from, err1 := strconv.Atoi(lo)
	to, err2 := strconv.Atoi(hi)
	if err1 != nil || err2 != nil {
		return nil, false
	}
	step := 1
	if to < from {
		step = -1
	}
	var out []string
	for i := from; ; i += step {
		out = append(out, strconv.Itoa(i))
		if i == to {
			break
		}
		// A range is bounded so a typo cannot hang the shell.
		if len(out) > 10000 {
			return nil, false
		}
	}
	return out, true
}
