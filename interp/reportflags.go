// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"

	"github.com/blairham/sh/syntax"
)

// The `(B)` `(E)` `(N)` and `(R)` expansion flags: the four that report
// *about* a match rather than choosing one, and the family `(M)` already
// belonged to.
//
// One grammar in the panel has them. The vendor manual names them beside
// `(M)` and `(S)` in one block:
//
//	B   the index of the beginning of the match
//	E   the index one character past its end
//	N   the length of the match
//	R   the unmatched portion (the Rest)
//	M   the matched portion, which #2143's span already answered
//
// #2143 made trimSpan answer with a span — where the match begins and where
// it ends — which is exactly what each of these reports one projection of.
// So this adds no matcher, no second walk and no parallel path: it reads the
// span that was already found.
//
// # They accumulate, and the order is not the order they were written
//
// Measured on zsh 5.9.2, 2026-09-12, with `str=aXbXc`. The single flags
// first, where the match is the `X` at the second character:
//
//	${(SB)str#X*}    2        one-based, and in *characters*
//	${(SE)str#X*}    3        one past the end, so B + N
//	${(SN)str#X*}    1
//	${(SR)str#X*}    abXc     which is the plain trim's own answer
//	${(SM)str#X*}    X
//
// and then together, which is the part an implementation reading them as
// alternative spellings of `(M)` would get wrong:
//
//	${(SBE)str#X*}    2 3
//	${(SBM)str#X*}    X 2
//	${(SBEM)str#X*}   X 2 3
//	${(SBMR)str#X*}   X abXc 2
//	${(BER)str#X*}    aXbXc 1 1
//	${(RB)str#X*}     aXbXc 1
//	${(SBENMR)str#X*} X abXc 2 3 1
//
// The last row fixes the order outright: **M, R, B, E, N**, whatever order
// the letters were written in.
//
// # It is one word and not a list
//
// The issue that asked for this expected a list, and the measurement says
// otherwise: `printf '[%s]' ${(SBE)str#X*}` is the single field `[2 3]`
// where a two-element array would print `[2][3]`, and this grammar does not
// split an unquoted expansion. The separator is a hard space rather than
// `$IFS[1]` — `IFS=-` leaves it `2 3` — and a join flag does not reach it
// either: `${(SBEj:_:)str#X*}` is `2 3` as well.
//
// # A pattern that matches nothing still reports
//
//	${(B)str#zzz}   1        so the indices are where a match would have begun
//	${(E)str#zzz}   1
//	${(N)str#zzz}   0
//	${(R)str#zzz}   aXbXc    the whole value is the Rest
//	${(M)str#zzz}   (empty)
//
// which is the same reading with an empty span at the front, and is why the
// four need no separate no-match branch.
//
// # Where they do not reach
//
// Measured one operator at a time, the way `(S)`'s boundary was, all with
// `str=aXbXc` and every one of them what it would have been with no flag:
//
//	${(B)str}         aXbXc    no operator
//	${(B)str:1}       XbXc     a substring
//	${(B)str:-alt}    aXbXc    a default
//	${(B)str/X/Y}     aYbXc    a replacement, where `(S)` *does* reach
//	${(B)#str}        5        the length
//
// The element-selecting operators are the exception and they are **refused
// by name** rather than answered. They do something there — `${(B)str:#a*}`
// is `1` and `${(N)str:#a*}` is `5`, so the whole-value test reports a span
// like a trim does — but the family does not answer alike: with `a=(x y)`
// and `b=(y z)`, `${(B)a:|b}` and `${(B)a:/x/Q}` both come back as the
// untouched array where `${(B)a:*b}` is empty, and a reading that produced
// any of those from the trim's span would be a plausible value at status 0.
// That is a measurement this change did not make, so it is a refusal.
//
// `(I:expr:)` is the fifth flag in the same block of the manual and is not
// here: it selects the *n*th match rather than reporting about the first,
// so it is a counter on the span search rather than a projection of what
// the search found, and "matches overlapping previous replacements are
// ignored" is a rule nothing in this tree has yet.

// matchReportOrder is the order the projections are emitted in, which is
// fixed and is not the order the letters were written. See the measurements
// above.
const matchReportOrder = "MRBEN"

// matchReportFlags is the subset of that order this node carries.
//
// Read off the node rather than carried on the Runner, for the reason
// searchingFlag is: the question is asked where the node is already held, and
// a field would be a second copy a nested expansion could leave stale.
func matchReportFlags(e *syntax.ParamExpr) string {
	if e == nil {
		return ""
	}
	var out []byte
	for i := 0; i < len(matchReportOrder); i++ {
		if strings.ContainsRune(e.Flags, rune(matchReportOrder[i])) {
			out = append(out, matchReportOrder[i])
		}
	}
	return string(out)
}

// reportsAboutTheMatch is whether any of the four new letters is written,
// which is the question the refusal below asks. `(M)` is not in it: it was
// already carried everywhere it reaches, the element exclusion included.
func reportsAboutTheMatch(e *syntax.ParamExpr) bool {
	return e != nil && strings.ContainsAny(e.Flags, "BENR")
}

// reportsAnElementOperator is where the four are refused: the operators that
// reshape a list, which report *something* in this shell and do not report it
// alike. See the note above.
func reportsAnElementOperator(op syntax.ParamOp) bool {
	return reshapesElements(op) || zipsElements(op)
}

// trimReport is the trim's span, projected the way the flags asked.
//
// It replaces trimWith and matchedWith for a node carrying any of the five,
// rather than sitting beside them, because the projections have to come from
// **one** span: `(M)` and `(B)` written together must report the same match,
// and two calls could not promise that under a searching trim.
func (r *Runner) trimReport(value, pattern string, e *syntax.ParamExpr, want string) string {
	lo, hi, m, ok := trimSpan(value, pattern, e.Op, r.patternOpts(pattern, value),
		r.armOrder(), searchingFlag(e))
	if !ok {
		// No match is an empty span at the front, which is the reading the
		// measurements above give: `1`, `1`, `0`, the whole value and
		// nothing. So there is no second branch.
		lo, hi = 0, 0
	}
	r.publishMatch(m)
	parts := make([]string, 0, len(want))
	for _, c := range want {
		switch c {
		case 'M':
			parts = append(parts, value[lo:hi])
		case 'R':
			parts = append(parts, value[:lo]+value[hi:])
		case 'B':
			parts = append(parts, strconv.Itoa(r.stringLength(value[:lo])+1))
		case 'E':
			parts = append(parts, strconv.Itoa(r.stringLength(value[:hi])+1))
		case 'N':
			parts = append(parts, strconv.Itoa(r.stringLength(value[lo:hi])))
		}
	}
	return strings.Join(parts, " ")
}

// firstOf is the first letter of set that appears in flags, for a refusal
// that names the flag the script wrote rather than the family it belongs to.
func firstOf(flags, set string) rune {
	for _, c := range flags {
		if strings.ContainsRune(set, c) {
			return c
		}
	}
	return ' '
}
