// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"
	"sort"
	"strings"

	"github.com/blairham/sh/syntax"
)

// The expansion flags that decide which words survive and in what order:
// `u` keeps the first of each repeat, and `o`, `O`, `n` and `i` sort.
//
// They are one function because they are one step of the pipeline. Measured
// on zsh 5.9.2 under the C locale, which is what the corpus runs in:
//
//	a=(c a b);      ${(@o)a}   a b c        ascending
//	a=(c a b);      ${(@O)a}   c b a        descending
//	a=(10 9 1);     ${(@o)a}   1 10 9       lexical, so 10 before 9
//	a=(10 9 1);     ${(@n)a}   1 9 10       numeric
//	a=(x1 x10 x9);  ${(@n)a}   x1 x9 x10    and the digits need not be alone
//	a=(B a C b);    ${(@o)a}   B C a b      byte order, so case separates
//	a=(B a C b);    ${(@oi)a}  a B b C      case folded, and stable in a tie
//	a=(b a b c a);  ${(@u)a}   b a c        the first of each, order untouched
//	a=(c a b);      ${(@a)a}   c a b        the index, so the order it was written in
//	a=(c a b);      ${(@aO)a}  b a c        and that reversed
//
// Where the step sits is measured too, and it is not where the manual's rule
// numbers would put it by name. The sort runs *after* the operator —
// `a=(zb ya); ${(@o)a#z}` is `b ya`, which is trim-then-sort and not the
// other way — and *after* case conversion: `a=(B a); ${(@oU)a}` is `A B`,
// where sorting first would give `B A`. It runs after splitting, so
// `${(@s.,.o)v}` on `c,a,b` sorts the three fields; and after joining, so
// `${(oj.-.)a}` is `c-a-b` unsorted, one word being already in order.

// orderFlags are the letters this step answers.
const orderFlags = "uoOnia"

// orderWords applies them, in the one order that reproduces every
// composition measured: the repeats go first and the sort follows.
//
// `${(@u)a}` alone does not sort and `${(@ou)a}`, `${(@uo)a}`, `${(@uO)a}`
// and `${(@Ou)a}` all agree, so which of the two runs first is not
// observable when both are written — but it is when only `u` is, and that is
// what fixes the order here.
func orderWords(e *syntax.ParamExpr, words []string) []string {
	if strings.ContainsRune(e.Flags, 'u') {
		seen := make(map[string]bool, len(words))
		out := words[:0:0]
		for _, w := range words {
			if seen[w] {
				continue
			}
			seen[w] = true
			out = append(out, w)
		}
		words = out
	}
	if !strings.ContainsAny(e.Flags, "oOnia") {
		return words
	}
	descending := strings.ContainsRune(e.Flags, 'O')
	// `a` is the sort *key* rather than a sort: the element's own position,
	// so ascending is the order it was written in and descending is that
	// reversed. Which is why it does not fit beside the comparisons below —
	// it beats every one of them, in either spelling: `${(@oa)a}`,
	// `${(@na)a}` and `${(@ia)a}` on `(c a b)` are all `c a b`, and
	// `${(@aO)a}`, `${(@Oa)a}` and `${(@aOn)a}` are all `b a c`.
	if strings.ContainsRune(e.Flags, 'a') {
		if descending {
			slices.Reverse(words)
		}
		return words
	}
	// `i` sorts on its own: `a=(c a b); ${(@i)a}` is `a b c`, so it is not
	// only a modifier of `o`.
	fold := strings.ContainsRune(e.Flags, 'i')
	numeric := strings.ContainsRune(e.Flags, 'n')
	// Stable, because a fold makes ties reachable and they keep the order
	// they were written in: `${(@oi)a}` on `(B a C b)` is `a B b C`, with the
	// `B` still ahead of the `b`.
	sort.SliceStable(words, func(i, j int) bool {
		c := compareWords(words[i], words[j], fold, numeric)
		if descending {
			return c > 0
		}
		return c < 0
	})
	return words
}

// compareWords orders two words: bytewise, or by the numbers inside them
// when `n` was written, and either with case folded away or not.
func compareWords(a, b string, fold, numeric bool) int {
	if !numeric {
		if fold {
			return strings.Compare(strings.ToLower(a), strings.ToLower(b))
		}
		return strings.Compare(a, b)
	}
	if c := compareNatural(a, b, fold); c != 0 {
		return c
	}
	// Numerically equal and not the same text: `(001 1 01)` comes back
	// `001 01 1`, which is byte order and not the order they were written
	// in, so the tie is broken rather than left to the stable sort.
	return strings.Compare(a, b)
}

// compareNatural compares two words with each run of digits read as a number.
//
// `x1 x9 x10` is the shape it exists for: the digits need not be the whole
// word, so a word is a sequence of digit and non-digit runs and the two are
// compared differently.
func compareNatural(a, b string, fold bool) int {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if isDigit(a[i]) && isDigit(b[j]) {
			ai, bj := i, j
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			if c := compareDigitRuns(a[ai:i], b[bj:j]); c != 0 {
				return c
			}
			continue
		}
		x, y := a[i], b[j]
		if fold {
			x, y = lowerByte(x), lowerByte(y)
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
		i++
		j++
	}
	switch {
	case i < len(a):
		return 1
	case j < len(b):
		return -1
	}
	return 0
}

// compareDigitRuns compares two runs of digits as numbers, without converting
// them: a run may be longer than any integer and a shell has no business
// refusing to sort it.
func compareDigitRuns(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

func lowerByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c - 'A' + 'a'
	}
	return c
}

// orderApplies reports whether this expansion asked for any of it, so the
// step is skipped rather than copying a slice for nothing.
func orderApplies(e *syntax.ParamExpr) bool {
	return strings.ContainsAny(e.Flags, orderFlags)
}
