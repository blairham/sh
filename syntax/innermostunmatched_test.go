// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"testing"
)

// When constructs nest and the input runs out inside them, which one is
// blamed is a dialect question — and it is answered by the *order* the
// reports arrive in rather than by anything looking around.
//
// The scanners recurse, so the innermost construct to run out reports first
// and the enclosing ones follow it outwards. Keeping the first report blames
// the innermost; letting each replace the last blames the outermost.
//
// Before this the skippers — the delimiter scans that step over a quote or a
// substitution without building a span — returned **quietly** at end of
// input. So the innermost never reported at all and the enclosing construct
// was blamed in every dialect, which is right for one of the four and wrong
// for three (#1151).
//
// The rows nest both ways round on purpose. A construct inside a quote and a
// quote inside a construct give opposite answers under the two readings, so
// neither can pass for the other; a table of only one shape would be
// satisfied by "always the quote" or "always the substitution".
func TestWhichNestedConstructIsBlamedWhenTheInputRunsOut(t *testing.T) {
	inner := Core()
	outer := Core()
	outer.UnmatchedBlamesTheOutermost = true

	for _, tc := range []struct {
		why, src           string
		innermost, outmost string
	}{
		{
			why:       "a substitution inside a double quote",
			src:       `echo "$( echo hi`,
			innermost: "$(",
			outmost:   `"`,
		},
		{
			// #1151's own row: the skip is two deep here, through the
			// expansion's body and then through the quote inside it.
			why:       "a substitution inside a quote inside an expansion",
			src:       `echo "${x:-"$( echo hi`,
			innermost: "$(",
			outmost:   `"`,
		},
		{
			why:       "a double quote inside a substitution — the other way round",
			src:       `echo $( echo "hi`,
			innermost: `"`,
			outmost:   "$(",
		},
		{
			why:       "a single quote inside a substitution inside a quote",
			src:       `echo "$( echo 'hi`,
			innermost: "'",
			outmost:   `"`,
		},
		{
			why:       "a backquote inside a substitution",
			src:       "echo $( echo `echo hi",
			innermost: "`",
			outmost:   "$(",
		},
		{
			why:       "a backquote inside an expansion's body",
			src:       "echo ${x:-`echo hi",
			innermost: "`",
			outmost:   "${",
		},
		{
			why:       "a substitution inside an expansion's body",
			src:       "echo ${x:-$( echo hi",
			innermost: "$(",
			outmost:   "${",
		},
	} {
		t.Run(tc.why, func(t *testing.T) {
			if got := unmatchedOpener(t, tc.src, inner); got != tc.innermost {
				t.Errorf("blaming the innermost: opener %q, want %q", got, tc.innermost)
			}
			if got := unmatchedOpener(t, tc.src, outer); got != tc.outmost {
				t.Errorf("blaming the outermost: opener %q, want %q", got, tc.outmost)
			}
		})
	}
}

// A construct with nothing nested inside it answers the same under both, and
// these are the rows that say the flag does not simply move every answer:
// there is only one construct, so the innermost and the outermost are it.
func TestOneConstructAloneIsBlamedTheSameEitherWay(t *testing.T) {
	inner := Core()
	outer := Core()
	outer.UnmatchedBlamesTheOutermost = true
	for _, tc := range []struct{ src, opener string }{
		{`echo "abc`, `"`},
		{`echo 'abc`, "'"},
		{`echo $(echo hi`, "$("},
		{`echo ${x`, "${"},
		{"echo `echo hi", "`"},
	} {
		for _, d := range []struct {
			name string
			d    Dialect
		}{{"innermost", inner}, {"outermost", outer}} {
			if got := unmatchedOpener(t, tc.src, d.d); got != tc.opener {
				t.Errorf("%s: %q: opener %q, want %q", d.name, tc.src, got, tc.opener)
			}
		}
	}
}

// Only an unmatched construct may be replaced. A dialect that blames the
// outermost must not turn some *other* refusal into a report about a
// delimiter that merely happened to be open when it was raised.
func TestBlamingTheOutermostDoesNotReplaceADifferentRefusal(t *testing.T) {
	d := Core()
	d.UnmatchedBlamesTheOutermost = true
	// A bad substitution operator inside a quote: the expansion is refused
	// for what it says rather than for running out, and the quote around it
	// closes.
	const src = "echo \"${x@ZZZ}\"\n"
	_, err := Parse(src, d)
	var se *Error
	if errors.As(err, &se) && se.Kind == ErrUnmatched {
		t.Errorf("%q was reported as unmatched %q — a refusal of another kind has been replaced", src, se.Token)
	}
}

func unmatchedOpener(t *testing.T, src string, d Dialect) string {
	t.Helper()
	_, err := Parse(src, d)
	var se *Error
	if !errors.As(err, &se) {
		t.Fatalf("%q: got %v, want a *syntax.Error", src, err)
	}
	if se.Kind != ErrUnmatched {
		t.Fatalf("%q: kind %v, want ErrUnmatched", src, se.Kind)
	}
	return se.Token
}

// What a *prompt* is still waiting on is a different question from what a
// diagnostic blames, and the skippers must not answer it.
//
// Lexer.openWord — which Parser.Open reports, and which a continuation prompt
// reads — names the construct the reader is inside, and it is recorded by the
// scanners rather than by the delimiter scans: `echo $( echo 'x` is a
// substitution that is still open, and the quote within it is not what a
// second prompt is for. So the skippers report the *failure* and deliberately
// do not call ranOut, and these are the values from before this change,
// asserted unchanged.
func TestTheOpenWordIsUnchangedByTheSkippersReporting(t *testing.T) {
	for _, tc := range []struct{ src, word string }{
		{"echo $( echo 'x\n", "$("},
		{"echo \"$( echo hi\n", "$("},
		{"echo \"${x:-\"$( echo hi\n", "${"},
		{"echo `echo hi\n", "`"},
		{"echo \"abc\n", "\""},
	} {
		p := NewParser(tc.src, Core())
		p.Parse()
		open := p.Open()
		if len(open) != 1 || open[0].Word != tc.word {
			t.Errorf("%q: open = %v, want the one word %q", tc.src, open, tc.word)
		}
	}
}
