// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// braceIsAGroupsCount reports whether the brace the scan has just matched is
// a pattern group's repetition count rather than a list of alternatives.
//
// It is the brace-expansion half of syntax.Dialect.CountedPatternGroup. The
// lexer half let the `(` into the word; this is what stops the `{…}` in front
// of it being expanded, and the two have to land together — a word the lexer
// keeps whole and the brace pass then fans out is `{2,3}(a)` coming back as
// the two words `2(a)` and `3(a)`, which is one wrong answer traded for
// another.
//
// **The discriminator is the character after the `}` and nothing about the
// brace itself.** That is #4933's row and it is the whole of why this is not
// written as "does the brace hold a count": `f {z,y}(a)` is `1 | [{z,y}(a)]`
// on ksh93u+ — the word entire, one field — where `f {z,y}a` one character
// short of it is the two words `za` and `ya`. A `{z,y}` is a perfectly good
// list and in front of a `(` it is not one, so an implementation keyed on the
// contents expands this and looks exactly like the list it is not.
//
// Three things have to be true and each has a measured row behind it, all on
// `/bin/ksh` `Version AJM 93u+ 2012-08-01`, 2026-09-27, under
// `env -i PATH=/usr/bin:/bin` in a directory holding `a`, `aa`, `aaa`, `aaaa`:
//
//   - the `{` is **written**: `g='{2,3}(a)'; f $g` is `2 | [2(a)] [3(a)]`,
//     so a brace an expansion produced is an ordinary list however the `(`
//     behind it was written. This is the row that says the look-ahead is
//     something a written brace *gains* rather than something the word is
//     re-scanned for (#4934).
//   - the `(` is **unquoted**, however it arrived: `f {z,y}"(a)"` is
//     `2 | [z(a)] [y(a)]` and `f {2,3}\(a\)` is `2 | [2(a)] [3(a)]`, so a
//     quoted or escaped parenthesis leaves the list alone, and
//     `g='(a)'; f {2,3}"$g"` is `2 | [2(a)] [3(a)]` for the same reason a
//     quoted expansion produces no brace syntax either.
//   - the `(` is the **next character**: `g='a(b'; f {z,y}$g` is
//     `2 | [za(b] [ya(b]`, and `f {2,3}(a){x,y}` is
//     `2 | [{2,3}(a)x] [{2,3}(a)y]` — only the brace standing in front of the
//     parenthesis is the count, and a second brace behind the group is still
//     a list.
//
// What the count then *means* is not asked here. A brace whose contents are
// not a count is suppressed exactly as one whose contents are, and the
// matcher is where the two part.
func (r *Runner) braceIsAGroupsCount(spans []syntax.Span, open, close cursor) bool {
	if !r.lang().CountedPatternGroup {
		return false
	}
	// A produced `{` is never a count. The scan that found it may read
	// produced runs — see braceScanReads — so the span it landed in is worth
	// asking about rather than assumed.
	if open.span >= len(spans) || !braceable(spans[open.span]) {
		return false
	}
	// And the brace has to hold something: `echo A{}(a)B` is a syntax error
	// in that shell where `echo A{,}(a)B` is the word, so an empty brace
	// never reaches here at all — the lexer refused the parenthesis. The
	// test is kept so that this side and the lexer's cannot disagree about
	// which braces the word could hold.
	if !before(next(open), close) {
		return false
	}
	return liveParenAt(spans, next(close))
}

// groupEndAfter is the cursor just past the `)` closing the group that opens
// at c, or c itself where nothing closes it.
//
// It is where the brace scan resumes behind a count, and resuming *there*
// rather than at the parenthesis is what keeps a brace inside the group's
// body out of the list: `f {2,3}(a{x,y})` is a refusal on ksh93u+ — the
// group's body is not a place a list may stand there — and fanning it out to
// `{2,3}(ax)` and `{2,3}(ay)` would be two words the script never wrote.
// Stepping over the group leaves the word entire instead, which is the
// nearest answer this side can give to a refusal it has no grounds to raise.
// A brace *behind* the group is still found, which is the panel's
// `f {2,3}(a){x,y}`.
//
// Only written unquoted text can hold the parentheses, so a span the brace
// scan does not read is stepped over whole rather than counted: a `(` an
// expansion produced is not a group's, which is the same rule
// braceIsAGroupsCount reads at the other end.
func groupEndAfter(spans []syntax.Span, c cursor) cursor {
	depth := 0
	for i := c.span; i < len(spans); i++ {
		if !braceable(spans[i]) {
			continue
		}
		off := 0
		if i == c.span {
			off = c.off
		}
		v := spans[i].Value
		for ; off < len(v); off++ {
			switch v[off] {
			case '\\':
				off++
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					return cursor{i, off + 1}
				}
			}
		}
	}
	return c
}

// liveParenAt reports whether the word's next character from c is an
// unquoted `(` — one the script wrote, or one an expansion produced.
//
// **The produced half is the whole of #4934 and it is not symmetric with the
// brace.** A `{` an expansion produced is never a count, and a `(` one
// produced is one all the same, so provenance is asked about the brace and
// not about the parenthesis. Measured 2026-09-27 on `/bin/ksh`
// `Version AJM 93u+ 2012-08-01`, in the directory of #4927's panel:
//
//	g=x;     f {2,3}$g     2 | [2x] [3x]      the brace expands
//	g='(a)'; f {2,3}$g     2 | [aa] [aaa]     the same written brace does not
//	f {2,3}$(echo '(a)')   2 | [aa] [aaa]     so it is not a variable-only path
//	g='(a)'; f {z,y}$g     1 | [{z,y}(a)]     and a count that is not one
//	                                          leaves the word entire
//	g='{2,3}(a)'; f $g     2 | [2(a)] [3(a)]  a produced *brace* is a list
//	g='(a)'; f {2,3}"$g"   2 | [2(a)] [3(a)]  and a quoted expansion
//	                                          produces no `(` either
//
// The first two rows are the whole of it: the same written `{2,3}`, expanded
// in one and suppressed in the other, and the only difference is what `$g`
// holds.
//
// **Nothing here runs an expansion or looks at a word twice**, which is what
// a row like the third one seems to demand. It does not, because ksh93 is
// also the column that finds a word's braces in the *fields* it expanded to
// rather than in the word the parse cut — `Semantics.BraceScanReadsProducedText`
// and the road at [Runner.expandFieldsThenBraces] — so on that road the
// produced text is already there to be read when this is asked, and the
// substitution has run exactly once. `n=0; f {2,3}$(n=1; echo '(a)'; echo x >&2)`
// prints its `x` once in the reference and once here.
//
// That the parenthesis then *is* a group rather than text is
// `Semantics.BraceFreesProducedGroupSyntax`, which ksh93 already answered
// Yes and which reads the same written brace one stage later. The two have
// to agree: a brace suppressed in front of a parenthesis that then went
// literal would be a third answer neither column gives.
//
// Spans are walked rather than indexed because a cursor one past the end of
// one span is the front of the next, and because a span may carry no text at
// all: an empty run standing between the `}` and the `(` — which is what
// `g='(a)'; e=; f {2,3}$e$g` writes — still suppresses the brace there,
// so it must not hide the parenthesis.
func liveParenAt(spans []syntax.Span, c cursor) bool {
	for i := c.span; i < len(spans); i++ {
		off := 0
		if i == c.span {
			off = c.off
		}
		if off >= len(spans[i].Value) {
			continue
		}
		return (braceable(spans[i]) || producedRun(spans[i])) &&
			spans[i].Value[off] == '('
	}
	return false
}
