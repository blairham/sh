// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// joined is what the spans spell, with an expansion written as `<kind>`
// rather than as its text: what this pair of readers differ about is which
// bytes land in a *literal*, and printing a span's own value back would make
// an expansion that should not have been read look like one that was.
func joined(t *testing.T, spans []syntax.Span) string {
	t.Helper()
	out := ""
	for _, s := range spans {
		if s.Kind == syntax.Literal {
			out += s.Value
			continue
		}
		out += "<expansion>"
	}
	return out
}

// [syntax.ArithTextSpans] differs from [syntax.HeredocSpans] in one place: a
// backslash inside brackets keeps both of its bytes.
//
// The two readers are otherwise the same scan, so this is the whole of the
// difference and it is asserted as a difference rather than as two separate
// expectations — a change that moved both would leave a pair of tests passing
// and the distinction gone.
//
// Why it exists: a here-document's body is finished text and the escape has
// done its work by the time the body is written, but an arithmetic
// expression's text is read *twice*, and a backslash that quoted a `$` has to
// survive the first read or the second one expands what the first was told
// not to. See the panel rows on ArithTextSpans.
func TestArithTextKeepsABackslashInsideBrackets(t *testing.T) {
	// The core grammar rather than a shell's, which is this package's rule
	// and costs nothing here: what parts the two readers is the escape and
	// the brackets, and no dialect flag reaches either.
	d := syntax.Core()
	for _, tc := range []struct{ name, text, arith, heredoc string }{
		{
			// The row the difference is for.
			name: "an escaped dollar inside brackets",
			text: `a[\$k]`, arith: `a[\$k]`, heredoc: `a[$k]`,
		},
		{
			name: "and inside apostrophes inside brackets",
			text: `a['\$k']`, arith: `a['\$k']`, heredoc: `a['$k']`,
		},
		{
			// Outside brackets both readers take the backslash off, which
			// is what keeps the rule to the subscripts it was measured in.
			name: "an escaped dollar outside brackets",
			text: `\$k + 1`, arith: `$k + 1`, heredoc: `$k + 1`,
		},
		{
			// After the brackets close, too: the depth is the rule and not
			// "anywhere after a `[`".
			name: "after the subscript closes",
			text: `a[1]+\$k`, arith: `a[1]+$k`, heredoc: `a[1]+$k`,
		},
		{
			// An unescaped `$` is still an expansion under both, which is
			// the control that says the escape is what moved.
			name: "an unescaped dollar inside brackets",
			text: `a[$k]`, arith: `a[<expansion>]`, heredoc: `a[<expansion>]`,
		},
		{
			// A backslash before an ordinary character was always kept by
			// both, so nothing here moved it.
			name: "a backslash before an ordinary character",
			text: `a[\q]`, arith: `a[\q]`, heredoc: `a[\q]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spans, err := syntax.ArithTextSpans(tc.text, d)
			if err != nil {
				t.Fatalf("ArithTextSpans(%q): %v", tc.text, err)
			}
			if got := joined(t, spans); got != tc.arith {
				t.Errorf("ArithTextSpans(%q) = %q, want %q", tc.text, got, tc.arith)
			}
			spans, err = syntax.HeredocSpans(tc.text, d)
			if err != nil {
				t.Fatalf("HeredocSpans(%q): %v", tc.text, err)
			}
			if got := joined(t, spans); got != tc.heredoc {
				t.Errorf("HeredocSpans(%q) = %q, want %q", tc.text, got, tc.heredoc)
			}
		})
	}
}
