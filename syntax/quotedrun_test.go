// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// Two runs of the same quoting are two pairs of quotes, and the printer has to
// write the boundary back: `"$e""$@"` reprinted as `"$e$@"` is a different
// program, because a shell reading what an empty `"$@"` takes with it is reading
// one quoted **string** rather than the word. See
// interp.Semantics.EmptyListTakesTheWord.
func TestTwoQuotedRunsOfOneQuotingArePrintedApart(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo "$e""$@"`, `echo "$e""$@"`},
		{`echo "$@""$e"`, `echo "$@""$e"`},
		{`echo "$e""$f""$g"`, `echo "$e""$f""$g"`},
		// A trailing empty pair was dropped outright before this.
		{`echo "$@"""`, `echo "$@"""`},
		// The same expansions inside one pair, which is the contrast: without
		// it the rows above would pass for a printer that split every span.
		{`echo "$e$@"`, `echo "$e$@"`},
		{`echo "$e$f$g"`, `echo "$e$f$g"`},
		// A run whose first span the boundary cannot be measured from is left
		// joined, which is what it was before and is the safe direction for a
		// printer — see syntax.QuotedRunBoundary.
		{`echo "a""b"`, `echo "ab"`},
		// And a pair of quotes with different quoting is split by the quoting
		// itself, with nothing here needed.
		{`echo "$@"''`, `echo "$@"''`},
	} {
		t.Run(tc.src, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			if got := syntax.Print(f); got != tc.want {
				t.Errorf("printed %q as %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// QuotedRunBoundary says three things, and the third is what keeps its two
// callers honest: whether the span opens a new pair of quotes, and whether that
// could be worked out at all.
func TestQuotedRunBoundarySaysWhenItCannotTell(t *testing.T) {
	spansOf := func(t *testing.T, src string) []syntax.Span {
		t.Helper()
		f, err := syntax.Parse("echo "+src, syntax.Core())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		return f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd).Args[1].Spans
	}
	for _, tc := range []struct {
		src          string
		at           int
		opens, known bool
	}{
		// The first span of a word has nothing in front of it, which is a fact
		// rather than something that could not be measured.
		{`"$e$@"`, 0, false, true},
		{`"$e$@"`, 1, false, true},
		{`"$e""$@"`, 1, true, true},
		// A braced spelling is four characters wide rather than two, so a
		// reading off a span count would get this pair the wrong way round.
		{`"$e${@}"`, 1, false, true},
		{`"$e""${@}"`, 1, true, true},
		{`"${e}${@}"`, 1, false, true},
		{`"${e}""${@}"`, 1, true, true},
		// A literal is not measured, and says so.
		{`"a""b"`, 1, false, false},
		{`"a$@"`, 1, false, false},
	} {
		t.Run(tc.src, func(t *testing.T) {
			opens, known := syntax.QuotedRunBoundary(spansOf(t, tc.src), tc.at)
			if opens != tc.opens || known != tc.known {
				t.Errorf("QuotedRunBoundary(%q, %d) = (%v, %v), want (%v, %v)",
					tc.src, tc.at, opens, known, tc.opens, tc.known)
			}
		})
	}
}
