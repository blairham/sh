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
	if s.Kind != syntax.ParamExp || e == nil || e.BareIndexText == nil {
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
