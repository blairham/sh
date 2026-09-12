// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runBareDiag is runBare with a wording table of the dialect's, for the one
// test whose subject is the sentence rather than the refusal.
func runBareDiag(t *testing.T, src string, subscript Answer, d Diagnostics) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(dl *syntax.Dialect) {
		dl.ArraySubscript = true
		dl.BareSubscript = true
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.BareSubscriptIsASubscript = subscript
		sem.ArrayBaseIsZero = Yes
		sem.ArrayScalarIsTheWholeArray = No
		sem.ArrayNameWithoutSubscriptIsTheList = No
		r.Semantics = &sem
		r.Diagnostics = &d
	})
}

// A `[` behind an unbraced `$name` that the word never closes, which the same
// axis decides as the closed one: where the brackets are a subscript, an
// unfinished one is refused, and where they are text, text with a `[` in it
// is a word like any other (#1757).
//
// Named for the axis and not for a shell, as everything in this package is.
// What the *sentence* says is a dialect's and is asserted in dialect/zsh.

// The pair, on one source, which is what says the axis decides it. A refusal
// that produced the text anyway, or text that also complained, would each
// fail exactly one of these two and neither could be seen from the other side
// alone.
func TestAnUnclosedBareSubscriptAnswersByAxis(t *testing.T) {
	const src = bareArray + `printf "[%s]" $a[1 2]` + "\nprintf after\n"

	t.Run("read as a subscript, it is unfinished", func(t *testing.T) {
		out, st := runBare(t, src, Yes)
		if !strings.Contains(out, "invalid subscript") {
			t.Errorf("= %q, want the unfinished-subscript sentence", out)
		}
		if st == 0 || strings.Contains(out, "after") {
			t.Errorf("= %q (status %d), want the input to end", out, st)
		}
	})

	t.Run("read as text, it is an ordinary word", func(t *testing.T) {
		out, st := runBare(t, src, No)
		if st != 0 || out != "[xx[1][2]]after" {
			t.Errorf("= %q (status %d), want [xx[1][2]]after", out, st)
		}
	})
}

// The blank is not what makes it unfinished — the end of the word is. `$a[`
// is the same refusal with nothing between the bracket and the word's end,
// and it is the shape a `for` list or an argument most often takes.
func TestABareSubscriptEndingWithTheWordIsUnfinished(t *testing.T) {
	out, st := runBare(t, bareArray+`printf "[%s]" $a[`+"\nprintf after\n", Yes)
	if !strings.Contains(out, "invalid subscript") || st == 0 || strings.Contains(out, "after") {
		t.Errorf("= %q (status %d), want the refusal and the input ended", out, st)
	}
}

// Said once for one span, not once per reading of it.
//
// expandAt asks the list shapes first and falls through to the scalar path for
// a span that is not one of them, so a quoted expansion and a name nothing
// declared each come through the seam twice. Two sentences for one `[` is what
// a report placed at the seam gets wrong, and no other case in this file can
// see it: the shapes that report once report once either way.
func TestAnUnclosedBareSubscriptIsReportedOncePerSpan(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"quoted", `printf "[%s]" "$a[1"`},
		{"a name nothing declared", `printf "[%s]" $nosuch[1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runBare(t, bareArray+tc.src, Yes)
			if n := strings.Count(out, "invalid subscript"); n != 1 {
				t.Errorf("= %q, said it %d times, want once", out, n)
			}
		})
	}
}

// What "unclosed" is measured against is the *word*, and three shapes say so
// from three directions. None of them is an error in the shell this was
// measured on.
func TestWhatDoesNotCountAsAnUnclosedBareSubscript(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// Closed, so it is the ordinary subscript this file's neighbor
		// already covers — and the control for every row below it.
		{"a subscript the word closes", `printf "[%s]" $a[1]`, "[yy]"},
		// Braced, so the braces say where the expansion ends and the
		// brackets were never the expansion's to begin with.
		{"braced", `printf "[%s]" ${a}[1`, "[xx[1]"},
		// Nothing directly behind the name, so no subscript was started.
		{"not directly behind the name", `printf "[%s]" "$a]1["`, "[xx]1[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBare(t, bareArray+tc.src, Yes)
			if st != 0 || out != tc.want {
				t.Errorf("= %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// The dialect's own sentence, through the field rather than through the
// fallback — the same shape the empty-subscript refusal is asserted in, and
// for the same reason: a wording hard-coded at the report site would pass
// every test above.
func TestAnUnclosedBareSubscriptTakesTheDialectsOwnSentence(t *testing.T) {
	out, st := runBareDiag(t, bareArray+`printf "[%s]" $a[1 2]`, Yes,
		Diagnostics{BareSubscriptUnclosed: "no such subscript, and this is the dialect's words"})
	if !strings.Contains(out, "no such subscript, and this is the dialect's words") {
		t.Errorf("= %q, want the dialect's sentence", out)
	}
	if st == 0 {
		t.Errorf("status %d, want the input to end", st)
	}
}
