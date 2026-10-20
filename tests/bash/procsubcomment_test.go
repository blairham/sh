// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// The user-visible half of #1397 was not the misparse. It was that the
// refusal named a line **214 lines** from the fault: a `#` comment inside a
// process substitution was read as code, the apostrophe in `it's` opened a
// single quote, and the quote ran to the end of the file — so the parser
// blamed the last apostrophe it found, which was correctly inside double
// quotes on a line with nothing wrong with it.
//
// Anyone debugging that starts where they are told to and finds nothing. So
// the fix has two halves, and this file is the second: a comment is a comment
// *and* a genuine unterminated quote is still blamed near its cause.
//
// Whole rendered lines and the location, because the location is the entire
// point. A Contains or HasSuffix assertion cannot catch an added prefix, and
// a message compared without its line cannot catch this bug at all.
func TestAnUnterminatedQuoteIsBlamedWhereItOpened(t *testing.T) {
	const want = "unexpected EOF while looking for matching `''"

	// This shell blames the line the quote *opened* on. Measured: bash 5.3,
	// that build called `sh`, bash 3.2 and ksh93 all name line 2 here, while
	// dash and zsh name the last line instead. There is no single answer, so
	// borrowing the other one would be wrong for this dialect — and the
	// distance between the two is exactly the confusion the issue reports.
	for _, tc := range []struct {
		name, src string
		line      int
	}{
		{
			"a few lines after it",
			"echo one\nx='never closed\necho two\necho three\necho four\n",
			2,
		},
		{
			// The issue's own distance. If the blame followed the end of the
			// input rather than the opener this would read 216 and the
			// row above would read 6, so the pair is what pins the rule
			// rather than a happy coincidence at short range.
			"two hundred lines after it",
			"echo one\nx='never closed\n" + strings.Repeat("echo filler\n", 212) + "echo last\n",
			2,
		},
		{
			// And inside the construct this issue is about, which is the
			// arrangement the old bug produced: the quote is real this time,
			// so it is still refused, and it is still blamed at its opener
			// rather than at the parenthesis or at the end of the file.
			"inside a process substitution body",
			"cat < <(\n\techo 'never closed\n)\necho after\n",
			2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := syntax.Parse(tc.src, bash.Dialect())
			if err == nil {
				t.Fatalf("parsed, want a refusal")
			}
			d := bash.Diagnostics()
			// One string, so a wrong line cannot pass by matching the
			// sentence and a wrong sentence cannot pass by matching the
			// line.
			got := fmt.Sprintf("line %d: %s", d.ParseFailureLine(err), d.ParseFailure(err))
			if exp := fmt.Sprintf("line %d: %s", tc.line, want); got != exp {
				t.Errorf("\n got %q\nwant %q", got, exp)
			}
		})
	}
}

// And the shape that says the comment rule stops at the newline: the `)` is
// inside the comment, so nothing ever closes the substitution.
//
// It is the refusal a paren-counting scanner without the comment rule does
// *not* produce — it finds that `)` and calls the construct closed, which is
// a silent acceptance of a file every shell with the construct rejects. This
// dialect puts an unmatched construct that holds a program on the line after
// the input's last, which is line 3 here and is what real bash 5.3 says.
func TestAClosingParenthesisInsideACommentClosesNothing(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string
		line int
	}{
		{"cat < <(echo hi # cmt )\necho after\n", "unexpected EOF while looking for matching `)'", 3},
		{"echo x > >(cat # cmt )\necho after\n", "unexpected EOF while looking for matching `)'", 3},
		{"cat < <(echo hi # it's a comment )\necho after\n", "unexpected EOF while looking for matching `)'", 3},
	} {
		_, err := syntax.Parse(tc.src, bash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		d := bash.Diagnostics()
		got := fmt.Sprintf("line %d: %s", d.ParseFailureLine(err), d.ParseFailure(err))
		if exp := fmt.Sprintf("line %d: %s", tc.line, tc.want); got != exp {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, exp)
		}
	}
}
