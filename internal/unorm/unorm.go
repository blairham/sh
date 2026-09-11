// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package unorm answers whether two strings are the same text, which is not
// the same question as whether they are the same bytes.
//
// It exists because a sandbox rule matches a name and a filesystem holds one
// file under a class of names. On macOS's default volumes a directory stored
// with a precomposed `é` is reached by the spelling that writes `e` and a
// combining acute, and a deny covering one of those covered the file only by
// luck — #2045, which leaked a credential and then overwrote it.
//
// # Why this is here and not imported
//
// `golang.org/x/text/unicode/norm` does this and does it well, and was used
// for one release. It is not here now for the reason internal/eastasian does
// not import `x/text/width`: this module ships no dependency it can generate
// instead, and the tables are small enough to own — 2081 decompositions and
// 934 combining classes, against a package that also carries composition,
// the compatibility mappings, and an iterator API nothing here calls.
//
// What is given up by owning it is somebody else's correctness, and that is
// bought back rather than assumed: unorm_test.go runs Unicode's own
// NormalizationTest.txt, every line of it, in all three canonically
// equivalent spellings. The oracle is the standard, not a library.
//
// # Only NFD, and only canonical
//
// Two strings are canonically equivalent exactly when their NFD forms are
// identical, so decomposing is the whole job and composing again would be
// work whose result is discarded. Compatibility equivalence is deliberately
// not here: NFKD folds `ﬁ` onto `fi`, no filesystem treats those as one
// name, and a gate that did would refuse files nobody denied.
package unorm

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// Hangul is decomposed by arithmetic rather than by table — 11172 syllables
// would be most of the table and all of them follow one rule.
const (
	hangulSBase = 0xAC00
	hangulLBase = 0x1100
	hangulVBase = 0x1161
	hangulTBase = 0x11A7
	hangulTCnt  = 28
	hangulNCnt  = 588
	hangulSCnt  = 11172
)

// NFD returns the canonical decomposition of s, with combining marks in
// canonical order.
//
// The ASCII fast path is not decoration. Every path a shell opens goes
// through here under a policy, and almost none of them has a character above
// U+007F in it; ASCII cannot decompose and cannot carry a combining class,
// so the answer is the input and the work is one scan.
func NFD(s string) string {
	if isASCII(s) {
		return s
	}
	out := make([]rune, 0, len(s)+8)
	for _, r := range s {
		out = appendDecomposed(out, r)
	}
	reorder(out)
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range out {
		b.WriteRune(r)
	}
	return b.String()
}

func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func appendDecomposed(dst []rune, r rune) []rune {
	if r >= hangulSBase && r < hangulSBase+hangulSCnt {
		i := r - hangulSBase
		dst = append(dst,
			hangulLBase+i/hangulNCnt,
			hangulVBase+(i%hangulNCnt)/hangulTCnt)
		if t := i % hangulTCnt; t != 0 {
			dst = append(dst, hangulTBase+t)
		}
		return dst
	}
	if d, ok := decomposition(r); ok {
		return append(dst, d...)
	}
	return append(dst, r)
}

func decomposition(r rune) ([]rune, bool) {
	i := sort.Search(len(decompKeys), func(i int) bool { return decompKeys[i] >= r })
	if i == len(decompKeys) || decompKeys[i] != r {
		return nil, false
	}
	return decompRunes[decompOffsets[i]:decompOffsets[i+1]], true
}

// combiningClass is 0 for all but the 934 characters that have one, so the
// search is skipped for anything below the first of them — which includes
// every ASCII character, and so every separator in a path.
func combiningClass(r rune) uint8 {
	if r < cccKeys[0] {
		return 0
	}
	i := sort.Search(len(cccKeys), func(i int) bool { return cccKeys[i] >= r })
	if i == len(cccKeys) || cccKeys[i] != r {
		return 0
	}
	return cccVals[i]
}

// reorder puts each run of combining marks into canonical order, in place.
//
// An insertion sort rather than sort.Slice, because the rule is not "sort the
// string": only adjacent marks with non-zero classes may be exchanged, and a
// character of class zero is a wall that nothing crosses. Sorting across one
// would move a mark onto a different base letter and change the text.
//
// Stability is the rest of the rule. Two marks of the *same* class keep the
// order they were written in, because their order is meaningful — they apply
// at the same position and the text says which came first — so the
// comparison is strictly greater-than and never greater-or-equal.
func reorder(rs []rune) {
	for i := 1; i < len(rs); i++ {
		c := combiningClass(rs[i])
		if c == 0 {
			continue
		}
		r := rs[i]
		j := i
		for j > 0 {
			p := combiningClass(rs[j-1])
			if p == 0 || p <= c {
				break
			}
			rs[j] = rs[j-1]
			j--
		}
		rs[j] = r
	}
}
