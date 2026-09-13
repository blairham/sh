// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `${(B)x}`, `${(E)x}`, `${(N)x}` and `${(R)x}` through the shell that has
// them, against zsh 5.9.2 on 2026-09-12 — #2153.
//
// The four report *about* a match where `(M)` reports the match itself, and
// they were refused by name. Every assertion is written as a row of the same
// value under the same operator, because a single index is a number and only
// the set of them says what the number is of.

// TestTheReportingFlagsProjectTheSpan is the single-flag table: `str=aXbXc`,
// where `(S)` puts the match on the `X` at the second character.
func TestTheReportingFlagsProjectTheSpan(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `str=aXbXc
print -r -- "[${(SB)str#X*}][${(SE)str#X*}][${(SN)str#X*}][${(SR)str#X*}][${(SM)str#X*}]"
print -r -- "[${(B)str#X*}][${(E)str#X*}][${(N)str#X*}][${(R)str#X*}][${(M)str#X*}]"
print -r -- "[${(B)str%c}][${(E)str%c}][${(N)str%c}][${(R)str%c}]"
print -r -- "[${(SB)str%X*}][${(B)str##a*X}][${(E)str##a*X}]"`)
	want := "[2][3][1][abXc][X]\n" +
		// No match: the indices are where a match would have begun, the
		// length is 0, the Rest is the whole value and the match is nothing.
		// `X*` cannot match at the front of `aXbXc`, so the unsearched trim
		// finds nothing — which is the same reading as a pattern that
		// matches nowhere at all.
		"[1][1][0][aXbXc][]\n" +
		// A suffix trim, where the span is pinned to the end instead.
		"[5][6][1][aXbX]\n" +
		// A searching suffix trim finds the *last* position that matches,
		// and a longest prefix trim takes as much as it can from the first.
		"[4][1][5]\n"
	if out != want || st != 0 {
		t.Errorf("the reporting flags = %q (status %d), want %q", out, st, want)
	}
}

// TestTheReportingFlagsAccumulateInAFixedOrder is the part an implementation
// reading them as alternative spellings of `(M)` would get wrong: written
// together they report one match from several sides, in an order that is not
// the order the letters were written.
//
// The last row fixes it outright — **M, R, B, E, N**.
func TestTheReportingFlagsAccumulateInAFixedOrder(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `str=aXbXc
print -r -- "[${(SBE)str#X*}][${(SBM)str#X*}][${(SBEM)str#X*}][${(SBMR)str#X*}]"
print -r -- "[${(BER)str#X*}][${(RB)str#X*}][${(EB)str#X*}][${(BM)str#X*}]"
print -r -- "[${(SBENMR)str#X*}]"`)
	want := "[2 3][X 2][X 2 3][X abXc 2]\n" +
		"[aXbXc 1 1][aXbXc 1][1 1][ 1]\n" +
		"[X abXc 2 3 1]\n"
	if out != want || st != 0 {
		t.Errorf("the reporting flags together = %q (status %d), want %q", out, st, want)
	}
}

// TestTheReportedIndicesAreCharacters — one-based, and in characters rather
// than bytes.
//
// The unit is `${#x}`'s and is asserted beside it, which is the point: it is
// the locale and MultibyteEncodingIsHonored together that decide, so a row
// naming characters without saying under what would be a fact about the
// machine the test ran on. The locale is set in the snippet because the
// harness's own environment has none, where both readings answer in bytes
// and the row could not tell them apart.
func TestTheReportedIndicesAreCharacters(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `export LC_ALL=en_US.UTF-8
s="a日b日c"
print -r -- "[${#s}][${(SB)s#日}][${(SE)s#日}][${(SN)s#日}]"`)
	want := "[5][2][3][1]\n"
	if out != want || st != 0 {
		t.Errorf("multibyte indices = %q (status %d), want %q", out, st, want)
	}
}

// TestTheAccumulationIsOneWordAndNotAList — the issue that asked for this
// expected a list and the measurement says otherwise. A two-element array
// would print `[2][3]` from a `printf` with one conversion; this prints
// `[2 3]`, and this grammar does not split an unquoted expansion. The
// separator is a hard space rather than `$IFS[1]`, and a join flag does not
// reach it either.
func TestTheAccumulationIsOneWordAndNotAList(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `str=aXbXc
printf '[%s]' ${(SBE)str#X*}; print
IFS=-; printf '[%s]' "${(SBE)str#X*}"; print
printf '[%s]' "${(SBEj:_:)str#X*}"; print
a=(${(SBEM)str#X*}); print -r -- "${#a}"`)
	want := "[2 3]\n[2 3]\n[2 3]\n1\n"
	if out != want || st != 0 {
		t.Errorf("the accumulation = %q (status %d), want %q", out, st, want)
	}
}

// TestTheReportingFlagsDoNotReachTheOtherOperators — measured one operator at
// a time, the way `(S)`'s boundary was, and every one of these is what it
// would have been with no flag at all.
func TestTheReportingFlagsDoNotReachTheOtherOperators(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `str=aXbXc
print -r -- "[${(B)str}][${(B)str:1}][${(B)str:-alt}][${(B)str/X/Y}][${(B)#str}]"
print -r -- "[${(N)str:1}][${(E)str}][${(R)str/X/Y}]"`)
	want := "[aXbXc][XbXc][aXbXc][aYbXc][5]\n[XbXc][aXbXc][aYbXc]\n"
	if out != want || st != 0 {
		t.Errorf("the flags off the trims = %q (status %d), want %q", out, st, want)
	}
}

// TestTheReportingFlagsReachEachElement — a whole-array trim reports per
// element, which is the same distribution the operator already had.
func TestTheReportingFlagsReachEachElement(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `arr=(aXb cXd)
printf '[%s]' "${arr[@]#*X}"; print
printf '[%s]' "${(B)arr[@]#*X}"; print
printf '[%s]' "${(M)arr[@]#*X}"; print
printf '[%s]' "${(BM)arr[@]#*X}"; print`)
	want := "[b][d]\n[1][1]\n[aX][cX]\n[aX 1][cX 1]\n"
	if out != want || st != 0 {
		t.Errorf("per element = %q (status %d), want %q", out, st, want)
	}
}

// TestTheReportingFlagsAreRefusedOverAnElementOperator — the boundary this
// change draws, and the reason it draws it there.
//
// They do something over the element-selecting operators — `${(B)str:#a*}` is
// `1` and `${(N)str:#a*}` is `5`, so the whole-value test reports a span the
// way a trim does — and the family does not do it alike: with `a=(x y)` and
// `b=(y z)`, `${(B)a:|b}` and `${(B)a:/x/Q}` both come back as the untouched
// array where `${(B)a:*b}` is empty. Projecting the trim's span onto any of
// those would be a plausible value at status 0, and that measurement has not
// been made, so it is refused by name.
//
// `(M)` is not refused there: it was already carried over the exclusion and
// is unchanged.
func TestTheReportingFlagsAreRefusedOverAnElementOperator(t *testing.T) {
	dir := t.TempDir()
	for _, src := range []string{
		`str=aXbXc; print -r -- "${(B)str:#a*}"`,
		`a=(x y); b=(y z); print -r -- "${(N)a:|b}"`,
		`a=(x y); b=(y z); print -r -- "${(E)a:*b}"`,
		`a=(x y); print -r -- "${(R)a:/x/Q}"`,
		`a=(x y); b=(y z); print -r -- "${(B)a:^b}"`,
	} {
		t.Run(src, func(t *testing.T) {
			out, st := runZsh(t, dir, src)
			if st == 0 || !strings.Contains(out, "is not implemented beside this operator") {
				t.Errorf("%s = %q (status %d), want a refusal naming the operator", src, out, st)
			}
		})
	}
	// And the flag that was already carried there still is: over a scalar
	// the exclusion is a whole-value test, and `(M)` reads the side of it
	// that keeps the value rather than dropping it.
	out, st := runZsh(t, dir, `str=aXbXc; print -r -- "[${(M)str:#a*}][${str:#a*}]"`)
	if want := "[aXbXc][]\n"; out != want || st != 0 {
		t.Errorf("(M) over the exclusion = %q (status %d), want %q", out, st, want)
	}
}

// TestTheSearchFlagIsStillTheFifthLetter — `(I:expr:)` sits beside these four
// in the same block of the vendor manual and is deliberately not implemented:
// it selects the *n*th match rather than reporting about the one that was
// found, so it is a counter on the span search rather than a projection of
// it. It must keep being refused by name rather than quietly reading as one
// of the four.
func TestTheSearchFlagIsStillRefused(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `str=aXbXc; print -r -- "${(SI:2:)str#X*}"`)
	if st == 0 || !strings.Contains(out, "not implemented") {
		t.Errorf("(I) = %q (status %d), want a refusal", out, st)
	}
}
