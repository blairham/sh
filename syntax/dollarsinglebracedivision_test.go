// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// [RereadWordTail]'s decode parameter: a `$'…'` written in a word operand puts
// its *value* where the escape was written, and the division is read off that
// text (#4207).
//
// The rows name [Dialect.QuoteProtectsTheClosingBrace] and no shell. What the
// one column with the construct prints is in
// dialect/bash/dollarsingleinbracequoted_test.go.
//
// decode here is a stand-in table rather than the escape table, which lives in
// interp: this package's question is where the produced bytes put the closing
// brace, not what `\x27` means.
func TestRereadWordTailReadsADollarSingleValue(t *testing.T) {
	t.Parallel()
	decode := func(s string) string {
		switch s {
		case `\x27`:
			return "'"
		case `\x22`:
			return `"`
		case `\x5c`:
			return `\`
		case `\x7d`:
			return "}"
		}
		return s
	}
	for _, tc := range []struct {
		name string
		// tail is a ParamExpr.RawTail: the source from the `${` to the end of
		// the word, the closing double quote included.
		tail string
		// cut is the reading the word was cut with and run the reading the
		// division is read under. They differ only where a mode moved one.
		cut, run BraceQuotePolicy
		// text is what the division was read from, and joined the literal text
		// the spans it produced hold. An empty want means the scan ran out.
		text, joined string
		ranOut       bool
	}{
		{
			// A value holding a `}` ends the expansion, and the rest of the
			// value falls out into the word behind it.
			name: "a value holding the brace",
			tail: `${u:-$'a}b'}"`,
			cut:  BraceQuoteProtectsEveryOperand, run: BraceQuoteProtectsEveryOperand,
			text: `${u:-a}b}"`, joined: "b}",
		},
		{
			// A produced quote protects the brace behind it, and the next one
			// ends the expansion: a division the written text does not have.
			name: "a produced quote that something closes",
			tail: `${u:-$'\x27'A}B'}"`,
			cut:  BraceQuoteProtectsEveryOperand, run: BraceQuoteProtectsEveryOperand,
			text: `${u:-'A}B'}"`, joined: "",
		},
		{
			// Nothing closes it, so the scan runs out of the expansion. That
			// belongs to the run: the file read cleanly under the reading it
			// was parsed with.
			name: "a produced quote that nothing closes",
			tail: `${u:-x$'\x27'}"`,
			cut:  BraceQuoteProtectsEveryOperand, run: BraceQuoteProtectsEveryOperand,
			text: `${u:-x'}"`, ranOut: true,
		},
		{
			name: "a produced double quote",
			tail: `${u:-x$'\x22'}"`,
			cut:  BraceQuoteProtectsEveryOperand, run: BraceQuoteProtectsEveryOperand,
			text: `${u:-x"}"`, ranOut: true,
		},
		{
			// A produced backslash quotes the brace rather than opening a run.
			name: "a produced backslash",
			tail: `${u:-x$'\x5c'}"`,
			cut:  BraceQuoteProtectsEveryOperand, run: BraceQuoteProtectsEveryOperand,
			text: `${u:-x\}"`, ranOut: true,
		},
		{
			// The reading that protects nothing reaches the brace whatever the
			// value is, so the same conversion divides and does not refuse.
			name: "a produced quote under the reading that protects nothing",
			tail: `${u:-x$'\x27'}"`,
			cut:  BraceQuoteProtectsEveryOperand, run: BraceQuoteProtectsNothing,
			text: `${u:-x'}"`, joined: "",
		},
		{
			// A pattern operand is not converted, so the text is the tail and
			// the division is the one the reading alone gives.
			name: "a pattern operand is left as it was written",
			tail: `${v#$'\x27'}"`,
			cut:  BraceQuoteProtectsEveryOperand, run: BraceQuoteProtectsEveryOperand,
			text: `${v#$'\x27'}"`, joined: "",
		},
		{
			// Nor a replacement.
			name: "a replacement operand is left as it was written",
			tail: `${v/Q/$'\x27'}"`,
			cut:  BraceQuoteProtectsEveryOperand, run: BraceQuoteProtectsEveryOperand,
			text: `${v/Q/$'\x27'}"`, joined: "",
		},
		{
			// Located under the cut reading: the reading that has no `$'…'`
			// inside a double-quoted brace read no escape, so there is nothing
			// to convert however the division reads the result.
			name: "cut under a reading with no such construct",
			tail: `${u:-x$'\x27'}"`,
			cut:  BraceQuoteProtectsAPatternOnly, run: BraceQuoteProtectsEveryOperand,
			text: `${u:-x$'\x27'}"`, joined: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := Core()
			d.DollarSingleQuote = true
			d.QuoteProtectsTheClosingBrace = tc.run
			at := Pos{Offset: 0, Line: 1, Col: 1}
			spans, text, err := RereadWordTail(tc.tail, at, d, tc.cut, decode)
			if text != tc.text {
				t.Errorf("read the division from %q, want %q", text, tc.text)
			}
			if tc.ranOut {
				if err == nil {
					t.Fatalf("the scan reached the end of %q, want it to run out", text)
				}
				return
			}
			if err != nil {
				t.Fatalf("dividing %q: %v", text, err)
			}
			var b strings.Builder
			for _, s := range spans {
				if s.Kind == Literal {
					b.WriteString(s.Value)
				}
			}
			if b.String() != tc.joined {
				t.Errorf("the literal text behind the expansion is %q, want %q", b.String(), tc.joined)
			}
		})
	}
}

// decode is asked nothing where a tail holds no `$'…'` at all, and a nil decode
// is the division on its own — the shape every dialect but one is in.
func TestRereadWordTailWithNoConversionToMake(t *testing.T) {
	t.Parallel()
	d := Core()
	d.QuoteProtectsTheClosingBrace = BraceQuoteProtectsEveryOperand
	at := Pos{Offset: 0, Line: 1, Col: 1}
	tail := `${v-'a}b'}"`
	for _, name := range []string{"a nil decode", "a decode nothing reaches"} {
		t.Run(name, func(t *testing.T) {
			var decode func(string) string
			if name == "a decode nothing reaches" {
				decode = func(string) string {
					t.Fatal("decode was asked about a tail holding no $'…'")
					return ""
				}
			}
			spans, text, err := RereadWordTail(tail, at, d, BraceQuoteProtectsEveryOperand, decode)
			if err != nil {
				t.Fatalf("dividing %q: %v", tail, err)
			}
			if text != tail {
				t.Errorf("read the division from %q, want the tail %q", text, tail)
			}
			if len(spans) != 1 || spans[0].Kind != ParamExp {
				t.Errorf("spans = %#v, want the one expansion", spans)
			}
		})
	}
}
