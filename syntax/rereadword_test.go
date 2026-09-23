// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// RereadWord reads text a run produced as the source of one word, which is
// what a shell that hands brace expansion's output back as characters needs.
//
// The rows are the shapes that separate it from reading the spans a parse
// already cut: a name whose text is longer than the one the file wrote, and a
// character that means something.
func TestRereadWordReadsProducedTextAsOneWord(t *testing.T) {
	at := syntax.Pos{Line: 1, Col: 1}
	for _, tc := range []struct {
		name, text string
		want       []syntax.Span
	}{
		{
			"a name the text continued", "$varx",
			[]syntax.Span{{Kind: syntax.ParamExp, Value: "varx", Bare: true}},
		},
		{
			"and one the braces ended", "${var}x",
			[]syntax.Span{
				{Kind: syntax.ParamExp, Value: "var"},
				{Kind: syntax.Literal, Value: "x"},
			},
		},
		{
			"plain text is one literal", "abd",
			[]syntax.Span{{Kind: syntax.Literal, Value: "abd"}},
		},
		{
			// A word is one word, so what a word scan stops at is a
			// character of it rather than a token — the rule an expansion's
			// operand is read under, and for the same reason.
			"an operator in it is text", "a>b",
			[]syntax.Span{
				{Kind: syntax.Literal, Value: "a"},
				{Kind: syntax.Literal, Value: ">"},
				{Kind: syntax.Literal, Value: "b"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spans, err := syntax.RereadWord(tc.text, at, syntax.Core())
			if err != nil {
				t.Fatalf("RereadWord(%q): %v", tc.text, err)
			}
			if len(spans) != len(tc.want) {
				t.Fatalf("RereadWord(%q) gave %d spans, want %d: %+v", tc.text, len(spans), len(tc.want), spans)
			}
			for i, w := range tc.want {
				if spans[i].Kind != w.Kind || spans[i].Value != w.Value ||
					spans[i].Quoting != w.Quoting || spans[i].Bare != w.Bare {
					t.Errorf("span %d of %q: got kind=%v value=%q quoting=%v bare=%v, want kind=%v value=%q quoting=%v bare=%v",
						i, tc.text, spans[i].Kind, spans[i].Value, spans[i].Quoting, spans[i].Bare,
						w.Kind, w.Value, w.Quoting, w.Bare)
				}
			}
		})
	}
}

// Text that will not read is an error belonging to the run rather than to the
// file, exactly as RereadWordTail's is: the program read cleanly under the
// characters it holds, and it is what the produced text spells that fails.
func TestRereadWordReturnsTheRunsOwnFailure(t *testing.T) {
	at := syntax.Pos{Line: 1, Col: 1}
	spans, err := syntax.RereadWord("x`y", at, syntax.Core())
	if err == nil {
		t.Fatalf("RereadWord read a substitution nothing closes: %+v", spans)
	}
	// The position is what a diagnostic names the unread tail from, so a
	// failure with nowhere in it would leave the sentence naming the whole
	// word.
	var se *syntax.Error
	if !errors.As(err, &se) {
		t.Fatalf("got %T, want a *syntax.Error carrying a position", err)
	}
	if se.Pos.Offset != 1 {
		t.Errorf("failed at offset %d, want the backtick at 1", se.Pos.Offset)
	}
}
