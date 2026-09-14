// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "strings"

// ElementSubscript splits one element of a compound literal into the
// subscript it names and the value it gives — `[sub]=value`, or `[sub]+=value`
// for the spelling that joins what the element already holds.
//
// The split is on the word **as written**, before any expansion, and that is
// the whole reason it lives here rather than in the interpreter: expanding
// first would hand `[k]=v` to the pattern matcher, where `[k]` is a character
// class. Both halves come back as words for the caller to expand as an
// assignment's value, which is what keeps `[k]=$x` whole and `[k]=*` a star.
//
// ok is false for a plain word, which is every element that does not open
// with an unquoted `[` and close it with a `]=` or a `]+=`. A quoted head is
// text: `a=("[1]=A" p)` is two ordinary words in every shell that reads the
// bracketed spelling at all.
//
// One scan serves two callers on purpose. The parser asks only whether the
// element is subscripted — one dialect decides a literal's whole shape on
// that answer for the first element, see
// [Dialect.ArrayLiteralShapeFollowsTheFirstElement] — and the interpreter
// asks for the halves. A second scan beside this one is how the two would
// come to disagree about what a subscripted element *is*, and then a literal
// would parse under one reading and store under the other.
func ElementSubscript(w *Word) (sub, value *Word, appends, ok bool) {
	if w == nil {
		return nil, nil, false, false
	}
	return elementSubscriptSpans(w.Spans)
}

// SubscriptedElement reports whether a compound literal's element names the
// subscript its value goes to, rather than being an ordinary word.
func SubscriptedElement(w *Word) bool {
	_, _, _, ok := ElementSubscript(w)
	return ok
}

// subscriptedElementSpans is SubscriptedElement before the spans are a word.
//
// The parser needs the answer one token early — the shape of the literal
// decides how the *next* element is lexed, and [Parser.word] lexes it on the
// way out — so the question is asked of the token's spans rather than of a
// word built from them. The same scan either way; see
// [Dialect.ArrayLiteralShapeFollowsTheFirstElement] for what turns on it.
func subscriptedElementSpans(spans []Span) bool {
	_, _, _, ok := elementSubscriptSpans(spans)
	return ok
}

func elementSubscriptSpans(spans []Span) (sub, value *Word, appends, ok bool) {
	if len(spans) == 0 {
		return nil, nil, false, false
	}
	head := spans[0]
	if head.Kind != Literal || head.Quoting != Unquoted ||
		!strings.HasPrefix(head.Value, "[") {
		return nil, nil, false, false
	}
	// The `]=` that closes the subscript is looked for across the spans, not
	// only in the first: `["c d"]=v` and `[$k]=v` put quoting or an expansion
	// between the brackets, so the subscript is a word of its own that ends
	// where an *unquoted* `]=` appears — a quoted one is part of it.
	for i, s := range spans {
		if s.Kind != Literal || s.Quoting != Unquoted {
			continue
		}
		text := s.Value
		if i == 0 {
			text = text[1:]
		}
		j, width, appended := elementTerminator(text)
		if j < 0 {
			continue
		}
		subWord := &Word{Spans: append([]Span(nil), spans[:i]...)}
		if i == 0 {
			subWord.Spans = nil
		} else {
			subWord.Spans[0].Value = strings.TrimPrefix(subWord.Spans[0].Value, "[")
		}
		subWord.Spans = append(subWord.Spans, Span{
			Kind: Literal, Value: text[:j], Quoting: s.Quoting, Pos: s.Pos,
		})
		valueWord := &Word{Spans: append([]Span{{
			Kind: Literal, Value: text[j+width:], Quoting: s.Quoting, Pos: s.Pos,
		}}, spans[i+1:]...)}
		return subWord, valueWord, appended, true
	}
	return nil, nil, false, false
}

// elementTerminator finds where a literal element's subscript ends: the first
// `]` that an `=` or a `+=` follows. It reports the offset, how many
// characters the terminator takes, and whether it is the append spelling.
//
// One scan for both spellings rather than two searches compared, because the
// two starts are the same character: at any given `]` only one of them can
// match, so "the first `]=`" and "the first `]+=`" are never in a race and
// the earlier terminator is simply the earlier `]`. Searching for `]=` alone
// and then for `]+=` would read `[a]=b]+=c` as an append, since the second
// pattern occurs in the *value* the first one already delimited.
func elementTerminator(text string) (at, width int, appends bool) {
	for i := 0; i < len(text); i++ {
		if text[i] != ']' {
			continue
		}
		switch {
		case strings.HasPrefix(text[i+1:], "="):
			return i, 2, false
		case strings.HasPrefix(text[i+1:], "+="):
			return i, 3, true
		}
	}
	return -1, 0, false
}
