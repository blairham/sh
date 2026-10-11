// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// braceRangeStrings is brace expansion for the one shape of word whose
// products need nothing but their text: a word written entirely as plain
// unquoted literal text, holding a single range — `{1..200000}`,
// `file{1..9}.txt`, `{a..z}` — whose elements and surrounding text are all
// inert characters. Each product is then exactly its string, and that string
// is the field it comes to.
//
// It exists for the footprint and not for a new answer. The general road
// builds every element as a span inside a word, re-reads each word's text as
// shell in the dialect that reads brace output that way, and expands each
// through the whole field pipeline, all before the first field is used:
// about 630 bytes held per six-character word in the bash dialect and 350 in
// zsh's, against 45 and 11 MB for `{1..200000}` in the real shells (#6359).
//
// It is behavior-preserving because every question the general road would
// put to such a word has one answer:
//
//   - the text holds nothing a later stage reads — no quote, escape, `$`,
//     backquote, pattern character, tilde, `=` or parenthesis — so the field
//     pipeline gives each product back as itself, matching finds no pattern
//     in it, and re-reading it as shell text cuts the one literal it already
//     is (BraceOutputRereadAsText is not asked of inert output on the
//     general road either);
//   - the body is a range that is asked nothing on the way: two plain
//     decimal numbers, or two single letters, with no step, no sign but a
//     minus, and no leading zero — padding, a written step, a plus and a
//     range that cannot be counted are the places a range asks an axis, and
//     each of those goes the general way;
//   - there is one brace pair and no other `{` or `}`, so there is no second
//     group to rescan for, no failed group to step over, and no pattern
//     group's count behind it, which needs a parenthesis.
//
// The second result is false for any other word, and the caller then takes
// the general road. Nothing here has a side effect before the answer is
// known, so declining costs only the scan.
func (r *Runner) braceRangeStrings(w *syntax.Word) ([]string, bool) {
	if r.noBraceExpand || r.sem().BraceExpansion != Yes || r.braceFieldRoute {
		return nil, false
	}
	text, ok := plainLiteralText(w)
	if !ok {
		return nil, false
	}
	open := strings.IndexByte(text, '{')
	if open < 0 {
		return nil, false
	}
	shut := strings.IndexByte(text[open:], '}')
	if shut < 0 {
		return nil, false
	}
	shut += open
	head, body, tail := text[:open], text[open+1:shut], text[shut+1:]
	if !inertText(head) || !inertText(tail) || !rangeAskedNothing(body) {
		return nil, false
	}
	alts, outcome := r.braceRange(body)
	if outcome != braceRangeCounted || !inertElements(alts) {
		return nil, false
	}
	if head == "" && tail == "" {
		return alts, true
	}
	for i, a := range alts {
		alts[i] = head + a + tail
	}
	return alts, true
}

// plainLiteralText is the text of a word written entirely as unquoted
// literal spans that carry no note of any kind, or false.
func plainLiteralText(w *syntax.Word) (string, bool) {
	if w == nil || len(w.Spans) == 0 {
		return "", false
	}
	for _, s := range w.Spans {
		s.Value, s.Pos = "", syntax.Pos{}
		if s != (syntax.Span{Kind: syntax.Literal, Quoting: syntax.Unquoted}) {
			return "", false
		}
	}
	if len(w.Spans) == 1 {
		return w.Spans[0].Value, true
	}
	var b strings.Builder
	for _, s := range w.Spans {
		b.WriteString(s.Value)
	}
	return b.String(), true
}

// inertText reports whether every character of s stands for itself in an
// unquoted word in every dialect. See inertInAWord.
func inertText(s string) bool {
	return !strings.ContainsFunc(s, func(c rune) bool { return !inertInAWord(c) })
}

// rangeAskedNothing reports whether a brace body is a range that braceRange
// counts without putting a question to the vector: `lo..hi` where both are
// plain decimal numbers or both single letters. See braceRangeStrings.
func rangeAskedNothing(body string) bool {
	lo, hi, ok := strings.Cut(body, "..")
	if !ok || strings.Contains(hi, "..") {
		return false
	}
	if plainNumber(lo) && plainNumber(hi) {
		return true
	}
	return len(lo) == 1 && len(hi) == 1 && isRangeLetter(lo[0]) && isRangeLetter(hi[0])
}

// plainNumber is a decimal number with at most a leading minus and no leading
// zero, which is the endpoint that pads nothing.
func plainNumber(s string) bool {
	s = strings.TrimPrefix(s, "-")
	if s == "" || len(s) > 18 || (len(s) > 1 && s[0] == '0') {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
