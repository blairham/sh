// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// What `<<-` ends at when the delimiter was written with leading tabs, which
// only a quoted delimiter can be. All three values are reached, over the four
// end lines that tell them apart — see syntax.HeredocDelimiterTabs for the
// panel each row was measured against.
func TestWhatAStrippedDocumentWithATabbedDelimiterEndsAt(t *testing.T) {
	t.Parallel()
	shapes := []struct{ name, src string }{
		{"a tabbed delimiter, a tabbed end line", "cat <<-'\tEOF'\n\tbody\n\tEOF\nrest\n"},
		{"a tabbed delimiter, a bare end line", "cat <<-'\tEOF'\n\tbody\nEOF\nrest\n"},
		{"a tabbed delimiter, a doubly tabbed end line", "cat <<-'\tEOF'\n\tbody\n\t\tEOF\nrest\n"},
		{"a doubly tabbed delimiter, a doubly tabbed end line", "cat <<-'\t\tEOF'\n\tbody\n\t\tEOF\nrest\n"},
	}
	const ends, runsOut = "body\n", "body\nEOF\nrest\n"
	for _, tc := range []struct {
		name  string
		tabs  syntax.HeredocDelimiterTabs
		wants [4]string
	}{
		{"kept", syntax.HeredocDelimiterTabsAreKept, [4]string{runsOut, runsOut, runsOut, runsOut}},
		{"the line as written", syntax.HeredocLineAsWrittenMeetsTheDelimiter, [4]string{ends, runsOut, runsOut, ends}},
		{"stripped too", syntax.HeredocDelimiterTabsAreStrippedToo, [4]string{ends, ends, ends, ends}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := syntax.Core()
			d.StrippedHeredocDelimiter = tc.tabs
			for i, s := range shapes {
				if got := firstHeredocBody(t, s.src, d); got != tc.wants[i] {
					t.Errorf("%s: body = %q, want %q", s.name, got, tc.wants[i])
				}
			}
		})
	}
}

// The controls: shapes every value answers alike, so the axis is asked only
// where a delimiter opens with a tab under the operator that strips.
func TestATabbedDelimiterIsOnlyAQuestionUnderTheStrippingOperator(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, body string }{
		{"no dash: the line as written is the only line", "cat <<'\tEOF'\nbody\nEOF\n\tEOF\n", "body\nEOF\n"},
		{"a tab inside the delimiter rather than in front", "cat <<-'E\tOF'\n\tbody\n\tE\tOF\nrest\n", "body\n"},
		{"an ordinary delimiter", "cat <<-EOF\n\tbody\n\t\tEOF\nrest\n", "body\n"},
	} {
		for _, tabs := range []syntax.HeredocDelimiterTabs{
			syntax.HeredocDelimiterTabsAreKept,
			syntax.HeredocLineAsWrittenMeetsTheDelimiter,
			syntax.HeredocDelimiterTabsAreStrippedToo,
		} {
			d := syntax.Core()
			d.StrippedHeredocDelimiter = tabs
			if got := firstHeredocBody(t, tc.src, d); got != tc.body {
				t.Errorf("%s (value %d): body = %q, want %q", tc.name, tabs, got, tc.body)
			}
		}
	}
}

// firstHeredocBody is onlyHeredocBody for a source whose here-document may
// run to the end of the input, which is a remark rather than an error.
func firstHeredocBody(t *testing.T, src string, d syntax.Dialect) string {
	t.Helper()
	p := syntax.NewParser(src, d)
	f := p.Parse()
	if len(f.Stmts) == 0 {
		t.Fatalf("parse %q: no statements (%v)", src, p.Err())
	}
	sc := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd)
	if len(sc.Redirs) != 1 || sc.Redirs[0].Heredoc == nil {
		t.Fatalf("no here-document in %q", src)
	}
	return sc.Redirs[0].Heredoc.Literal()
}
