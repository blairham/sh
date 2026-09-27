// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// producedPatternLeavesABraceMarks is the characters a written brace makes
// ordinary in the text an expansion produced behind it.
//
// `?` is deliberately **not** among them, and that is the whole of what this
// rule is keyed on rather than an omission — see
// braceGlobStopSpans for the grid. `*` and `[` are the leaves whose match the
// brace takes away; the third leaf keeps its.
//
// `@` is the fourth and is not a leaf at all: it is ordinary text wherever it
// stands alone, so marking it is invisible except in front of a `(`, where it
// is the difference between a pattern group and four characters. It earns its
// place only beside the rule that frees a produced group's parentheses — with
// `za` and `z@a` in the directory, `g='@(a)'; f {z,y}$g` is `[z@(a)]
// [y@(a)]` in ksh93u+, matching neither — and it is spelled here rather than
// there because it is the same mark on the same run. See freeGroupSyntaxIn.
const producedPatternLeavesABraceMarks = "*[@"

// braceGlobStopSpans marks the spans a written `{` stands in front of, for
// the column where the brace takes the match away from a `*` or a `[` an
// expansion produced behind it.
//
// Measured 2026-09-27 against /bin/ksh `Version AJM 93u+ 2012-08-01` and
// /opt/homebrew/bin/bash 5.3.20, each case in a directory of its own holding
// `za`, `zb` and `zq` created by the shell under test and confirmed with
// `ls`, `f` printing its count and its arguments:
//
//	                         ksh93u+                    bash 5.3.20
//	g='*'; f z$g   (control) 3 | [za] [zb] [zq]         same
//	f {z,y}*       (control) 4 | [za] [zb] [zq] [y*]    same
//	g='*'; f {z,y}$g         2 | [z*] [y*]              4 | [za] [zb] [zq] [y*]
//	g='[ab]'; f {z,y}$g      2 | [z[ab]] [y[ab]]        3 | [za] [zb] [y[ab]]
//	g='?'; f {z,y}$g         4 | [za] [zb] [zq] [y?]    same
//
// The two controls are why the rest is readable: the probe can see a
// *produced* `*` match and a *written* one match, and it saw the two rows
// under them not match. Every row has files the pattern can hit, which the
// claim this replaces did not — `g='a*'; f {p}$g` in a directory with no
// `a…` file matches nothing in any column, and shows a pattern passing
// through unchanged rather than a pattern never tried.
//
// **The last row is the rule's shape and not an exception to it.** A rule
// stated as "a brace makes the rest of the word text" predicts `[z?] [y?]`
// and is wrong; the set is measured, a character at a time, and it is `*`
// and `[` and not `?`. Measured over the leaves and over the bracket's
// spellings — `[!a]`, `[^a]`, `[a-b]` and `[[:alpha:]]` all go the way
// `[ab]` does, and `?` keeps its match wherever in the value it stands:
// `g='a?'; f {z,y}$g` matches where `g='a*'` does not.
//
// The keys are BraceStopsFieldSplitting's, which is why the extent is that
// rule's own — spansBehindAWrittenBrace:
//
//	g='*'; f {z}$g           1 | [{z}*]     the character, not a group
//	g='*'; f "{z}"$g         1 | [{z}q]     a quoted brace is exempt
//	g='*'; f \{z\}$g         1 | [{z}q]     and so is an escaped one
//	b='{'; g='*'; f ${b}z,y}$g               a produced brace is exempt
//	g='*'; f $g{a,b}                         and what stands in front matches
//
// (the last three measured in a directory also holding `{z}q`, `ya` and
// `yb`, so that each has a file it could have matched).
//
// nil wherever the question does not arise, which is four columns out of
// five and every word with no brace in it.
func (r *Runner) braceGlobStopSpans(spans []syntax.Span) []bool {
	// Two rules stand on this extent now, and either one arms it: the marks
	// this file puts on, and the marks
	// Semantics.BraceFreesProducedGroupSyntax takes off. Each asks its own
	// axis where it is applied, so a vector that answered one of them and
	// not the other gets exactly the one it answered.
	if r.sem().BraceMakesAProducedStarOrBracketText != Yes &&
		r.sem().BraceFreesProducedGroupSyntax != Yes {
		return nil
	}
	return spansBehindAWrittenBrace(spans)
}

// markPatternLeavesBehindABrace makes a produced `*` or `[` ordinary, for a
// span standing behind a written unquoted brace.
//
// esc is the escaped form a field carries, so the walk steps over a `\c`
// pair exactly as markGroupSyntaxFromTheValue's does: a backslash the value
// itself held has already been marked, and marking the byte behind one would
// be a mark on text that was never a metacharacter.
//
// Read rather than asked, as BraceStopsFieldSplitting is and for the same
// reason: braceGlobStopSpans arms this only where the axis says yes, so a
// dialect that cannot be asked — one with no brace expansion, or one that
// never matches an expansion's result at all — never reaches it and is never
// refused over a field with nothing wrong with it.
func (r *Runner) markPatternLeavesBehindABrace(esc string) string {
	if !r.braceStopsGlob || r.sem().BraceMakesAProducedStarOrBracketText != Yes {
		return esc
	}
	if !hasLiveByteOf(esc, producedPatternLeavesABraceMarks) {
		return esc
	}
	var b strings.Builder
	b.Grow(len(esc))
	for i := 0; i < len(esc); i++ {
		if esc[i] == '\\' && i+1 < len(esc) {
			b.WriteByte(esc[i])
			b.WriteByte(esc[i+1])
			i++
			continue
		}
		if strings.IndexByte(producedPatternLeavesABraceMarks, esc[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(esc[i])
	}
	return b.String()
}
