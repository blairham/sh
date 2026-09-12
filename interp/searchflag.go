// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The `(S)` expansion flag: the *other* end of the length order a trim and a
// substitution already choose between.
//
// One shell in the panel has the flag, and it does two things that read as
// one. Against a substitution it is non-greedy — the shortest match is
// replaced where the operator otherwise replaces the longest. Against a trim
// it is a **substring search**: the pattern no longer has to sit at the end
// of the value it is trimmed from, and what is taken out may come from the
// middle.
//
// Both are the same axis turned over: *which* of the matches available is the
// one the operator acts on. So this flag adds no matcher, no second span walk
// and no parallel path — it is a parameter on the walk that was already there
// (see spanByLength and replace in interp/expand.go), which is the only way
// the two readings cannot drift apart.
//
// # Measured on zsh 5.9.2, 2026-09-12
//
// The substitution half, with `s=abab`:
//
//	${s/*b/_}     _          the longest match, from the start
//	${(S)s/*b/_}  _ab        the shortest
//	${s//*b/_}    _          one match took the whole value
//	${(S)s//*b/_} __         two shortest matches
//
// and with `v=abcabc`, where the anchors say the flag reaches them too:
//
//	${v/#a*b/X}     Xc       the longest prefix
//	${(S)v/#a*b/X}  Xcabc    the shortest
//	${v/%b*c/X}     aX       the longest suffix
//	${(S)v/%b*c/X}  abcaX    the shortest
//
// The trim half, with `str=aXbXc`, is the one a reader would not guess from
// the name. The vendor manual states it as a search: with `#` and `##` for
// the match that **starts closest to the start** of the value, and with `%`
// and `%%` for the one that starts closest to the *end* — explicitly not the
// one that ends closest to it. The operator still chooses how much of a match
// to take at whichever position the search settled on:
//
//	${(S)str#X*}    abXc     shortest at the first position that matches
//	${(S)str##X*}   a        longest at that same position
//	${(S)str%X*}    aXbc     shortest at the last position that matches
//	${(S)str%%X*}   aXb      longest there — and the match stops short of
//	                         the end, which a suffix trim cannot do
//
// So a trim under this flag is a **span removal** rather than a prefix or a
// suffix removal, which is why trimSpan answers with a range rather than with
// a split point. The unflagged operators are that same range with one end
// pinned: a prefix trim's span begins at 0 and a suffix trim's ends at the
// length, and nothing else about them changes.
//
// # Where the flag does not reach
//
// Measured one operator at a time rather than reasoned from the name, the
// same way `(M)`'s boundary was (see param/the-matching-flag-elsewhere-does-
// nothing in the corpus). With `str=aXbXc`, every one of these is what it
// would have been with no flag at all:
//
//	${(S)str}        aXbXc      no operator
//	${(S)str:1}      XbXc       a substring
//	${(S)str:-alt}   aXbXc      a default
//	${(S)str:#a*}    (empty)    the element exclusion
//	${(S)#str}       5          the length
//	${(S)str[(i)X]}  2          a subscript search
//
// The exclusion is the one worth naming, because it is a pattern operator and
// it is a *whole-value* test: there is no longest or shortest match to choose
// between, so a flag that chooses between them has nothing to say.
//
// # And it composes with the flag beside it
//
//	${(SM)str#X*}    X        the match rather than the remainder
//	${(SM)str##X*}   XbXc
//	${(SM)str%%X*}   Xc
//	${(SM)str#zz}    (empty)  a pattern that matches nothing
//
// `(M)` reads the other side of the same split, so it needs nothing from this
// flag beyond the span both of them are now written against — which is the
// whole reason the span is what trimSpan returns.

// searchingFlag reports whether an expansion carries `(S)`: the flag that
// turns a trim into a substring search and a substitution non-greedy.
//
// Read off the node rather than carried on the Runner, because the question
// is asked in three places that all already hold the node, and a field would
// be a second copy of the answer that a nested expansion could leave stale.
func searchingFlag(e *syntax.ParamExpr) bool {
	return e != nil && strings.ContainsRune(e.Flags, 'S')
}
