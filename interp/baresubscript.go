// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// Whether the `[…]` an unbraced `$name` carries is a subscript at all.
//
// The grammar that has the construct can be told at run time that it does
// not, and then the same word is the parameter followed by the characters
// between the brackets. Both readings are in the tree — the parser records
// the subscript and its text side by side, see syntax.ParamExpr.BareIndexText
// — because the choice is the *run's* and not the reading's: a function body
// written under one answer and called under the other takes the caller's,
// measured, and so does the same text passed to `eval`.
//
// The seam is here rather than in the word loops because there are four of
// those and one of them is a pattern's. Every span that expands goes through
// expandSpan or expandAt, and every span that becomes a pattern goes through
// patternSpan, so those three ask and nothing else has to.

// unreadBareSubscript answers a span whose unbraced subscript is not read as
// one: the span with the subscript taken off, and the word its brackets are
// as text. The word is nil wherever the subscript stands, which is every
// braced expansion, every dialect without the construct, and the dialect that
// has it with the answer left as it comes.
func (r *Runner) unreadBareSubscript(s syntax.Span) (syntax.Span, *syntax.Word) {
	e := s.Param
	if s.Kind != syntax.ParamExp || e == nil {
		return s, nil
	}
	if e.BareIndexUnclosed {
		// A `[` the word never closed. There is no subscript here and no
		// bracket text either — the lexer gave the characters back and they
		// are a literal span of the word — so the reading that takes the
		// brackets for a subscript has nothing left to take, and says so
		// (#1757).
		//
		// The same axis decides it, because it is the same question: where
		// the brackets are not a subscript they are text, and text with an
		// unclosed `[` in it is a word like any other. Measured on zsh
		// 5.9.2, 2026-09-12, `setopt noglob` so a bad pattern cannot be what
		// is reported, `a=(xx yy zz)`: `$a[1 2]` is `invalid subscript` and
		// the same line under `setopt ksharrays` prints `xx[` and `2]`.
		//
		// Asked here rather than at the three callers for the reason the
		// file's own note gives: every span that expands or matches comes
		// through one of them, and a fourth would get none of this.
		//
		// Once per command, not once per reading of the span: expandAt asks
		// the list shapes first and falls through to expandSpan for a span
		// that is not one of them, so a quoted `"$a[1"` and an unset
		// `$nosuch[1` come through here twice and said it twice.
		//
		// expandErr is what says one has already been said, rather than a
		// marker of this seam's own. It is the same fact — an expansion this
		// command cannot perform — it is already reset per command, and it is
		// what the *first* of the two reports sets. A marker keyed on the node
		// would have needed clearing somewhere and nothing could observe
		// whether the clearing happened, because a refusal here ends the
		// input: measured, the second call of a function holding one is never
		// reached, and neither is the second turn of a loop.
		if !r.expandErr && r.ask(r.sem().BareSubscriptIsASubscript,
			"the `[…]` after an unbraced `$name` being a subscript") {
			r.diagf("%s\n", Wording(r.diag().BareSubscriptUnclosed, "invalid subscript"))
			r.expandErr = true
		}
		return s, nil
	}
	if e.BareIndexText == nil {
		return s, nil
	}
	if r.ask(r.sem().BareSubscriptIsASubscript,
		"the `[…]` after an unbraced `$name` being a subscript") {
		return s, nil
	}
	// A copy: the node is the parse tree's and the same node is expanded
	// again on the next pass through the loop it sits in, possibly under the
	// other answer.
	plain := *e
	plain.Index, plain.IndexFlags, plain.Leading = nil, nil, nil
	plain.BareIndexText = nil
	s.Param = &plain
	return s, e.BareIndexText
}

// bareSubscriptText expands the brackets a run has decided are text.
//
// Through expandSpan, span by span, which is what makes them the word's text
// rather than a second kind of it: an expansion written between the brackets
// is performed, an unquoted `[` stays live for the glob stage, and a value
// that splits reports that it did so the field it lands in splits with it.
func (r *Runner) bareSubscriptText(tail *syntax.Word, sp splitPolicy) (string, bool) {
	var b strings.Builder
	split := false
	for _, ts := range tail.Spans {
		// head is false throughout: something stands in front of these
		// brackets by construction — the parameter they were written behind.
		text, s := r.expandSpan(ts, sp, false)
		b.WriteString(text)
		split = split || s
	}
	return b.String(), split
}

// bareSubscriptPattern is bareSubscriptText for a `case` arm or a condition,
// where a span is worth its metacharacters rather than its fields.
func (r *Runner) bareSubscriptPattern(tail *syntax.Word) string {
	var b strings.Builder
	for _, ts := range tail.Spans {
		text, live := r.patternSpan(ts)
		if live {
			b.WriteString(text)
			continue
		}
		b.WriteString(escapePatternMeta(text))
	}
	return b.String()
}
