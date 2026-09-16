// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `#` or a `%` after the **global** `//` of a span replacement, which one
// column reads as an anchor and the rest as the pattern's own first byte
// (#3307).
//
// The parser reads the anchor in both spellings and this axis decides whether
// it counts, which is where ReplacementAnchors already put the single
// spelling's question and for the same measured reason: every column
// *accepts* `${v//#a/X}`, so acceptance is not what differs and a grammar
// flag would have had to claim one shell's reading for the core.
//
// Every row is asserted both ways round. A row that only has the value
// `abcabc` in it cannot separate the two readings — it is the answer both
// when the anchor is honored and `a` does not start the value, and when the
// pattern is `#a` and is not in the value — so the rows that matter are the
// ones over a value that *holds* the anchor character.
func TestWhetherTheGlobalSpellingTakesAnAnchor(t *testing.T) {
	for _, tc := range []struct{ name, src, anchored, plain string }{
		{
			// The discriminating pair, at the start.
			"a value holding the character",
			`w='x#ay%bz'; printf "[%s]" "${w//#a/Q}"`,
			"[x#ay%bz]", "[xQy%bz]",
		},
		{
			"and at the end",
			`w='x#ay%bz'; printf "[%s]" "${w//%b/Q}"`,
			"[x#ay%bz]", "[x#ayQz]",
		},
		{
			// The row the issue was filed on, which the pair above is what
			// makes readable.
			"the filed row",
			`v=abcabc; printf "[%s]" "${v//#a/X}"`,
			"[Xbcabc]", "[abcabc]",
		},
		{
			"the end anchor's",
			`v=abcabc; printf "[%s]" "${v//%c/Y}"`,
			"[abcabY]", "[abcabc]",
		},
		{
			// Exactly one character is taken, so a doubled one leaves a
			// pattern byte behind — and the two readings still differ,
			// because unanchored the pattern is `##a` rather than `#a`.
			"a doubled character",
			`w='x#ay%bz'; printf "[%s]" "${w//##a/Q}"`,
			"[x#ay%bz]", "[x#ay%bz]",
		},
		{
			// A real pattern behind the anchor, so the claim is not about
			// one-character patterns: anchored, `a*` takes the whole value.
			"a pattern behind the anchor",
			`v=abcabc; printf "[%s]" "${v//#a*/X}"`,
			"[X]", "[abcabc]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := testSemantics()
			sem.GlobalReplacementAnchors = Yes
			out, st := runGrammar(t, tc.src, patternGrammar, withSem(sem))
			if out != tc.anchored || st != 0 {
				t.Errorf("yes: %s = %q (status %d), want %q at 0", tc.src, out, st, tc.anchored)
			}
			sem.GlobalReplacementAnchors = No
			out, st = runGrammar(t, tc.src, patternGrammar, withSem(sem))
			if out != tc.plain || st != 0 {
				t.Errorf("no: %s = %q (status %d), want %q at 0", tc.src, out, st, tc.plain)
			}
			sem.GlobalReplacementAnchors = Unspecified
			out, st = runGrammar(t, tc.src, patternGrammar, withSem(sem))
			if st != 2 || !strings.Contains(out, "an anchor after the global spelling") {
				t.Errorf("unspecified: %s = %q (status %d), want a refusal naming the axis at 2",
					tc.src, out, st)
			}
		})
	}
}

// The **single** spelling never reaches the axis, which is the boundary it
// was placed at: a shell that reads an anchor after one `/` reads it whatever
// this answers, so asking there would put a question to a dialect about a
// spelling no column disagrees over.
func TestTheSingleSpellingAsksNoGlobalAxis(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"anchored at the front", `w='x#ay%bz'; printf "[%s]" "${w/#a/Q}"`, "[x#ay%bz]"},
		{"anchored at the end", `w='x#ay%bz'; printf "[%s]" "${w/%b/Q}"`, "[x#ay%bz]"},
		{"and an unanchored global one", `v=abcabc; printf "[%s]" "${v//b/X}"`, "[aXcaXc]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := testSemantics()
			sem.GlobalReplacementAnchors = Unspecified
			out, st := runGrammar(t, tc.src, patternGrammar, withSem(sem))
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// Nor does a column with no anchors at all, which has already answered the
// question one axis earlier: the character is the pattern's first byte there
// whichever way this one points, so BusyBox ash leaves it unanswered on
// purpose.
func TestAColumnWithNoAnchorsAsksNoGlobalAxis(t *testing.T) {
	sem := testSemantics()
	sem.ReplacementAnchors = No
	sem.GlobalReplacementAnchors = Unspecified
	const src = `w='x#ay%bz'; printf "[%s]" "${w//#a/Q}"`
	out, st := runGrammar(t, src, patternGrammar, withSem(sem))
	if out != "[xQy%bz]" || st != 0 {
		t.Errorf("%s = %q (status %d), want [xQy%%bz] at 0", src, out, st)
	}
}

// The empty pattern behind a global anchor is the anchored-empty axis and not
// a fourth one — measured, zsh answers `${v//#/X}` exactly as it answers
// `${v/#/X}`, so nothing here splits the two spellings.
func TestAnEmptyPatternBehindAGlobalAnchor(t *testing.T) {
	for _, tc := range []struct{ name, src, fires, declines string }{
		{"at the front", `v=abc; printf "[%s]" "${v//#/X}"`, "[Xabc]", "[abc]"},
		{"at the end", `v=abc; printf "[%s]" "${v//%/X}"`, "[abcX]", "[abc]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := testSemantics()
			sem.GlobalReplacementAnchors = Yes
			sem.EmptyReplacementPattern = EmptyReplacementPatternUnspecified
			sem.AnchoredEmptyReplacementPattern = Yes
			out, st := runGrammar(t, tc.src, patternGrammar, withSem(sem))
			if out != tc.fires || st != 0 {
				t.Errorf("yes: %s = %q (status %d), want %q at 0", tc.src, out, st, tc.fires)
			}
			sem.AnchoredEmptyReplacementPattern = No
			out, st = runGrammar(t, tc.src, patternGrammar, withSem(sem))
			if out != tc.declines || st != 0 {
				t.Errorf("no: %s = %q (status %d), want %q at 0", tc.src, out, st, tc.declines)
			}
		})
	}
}
