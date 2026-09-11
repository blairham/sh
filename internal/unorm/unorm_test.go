// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package unorm_test

import (
	"bufio"
	"compress/gzip"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/unorm"
)

// TestUnicodesOwnConformanceFile is the argument for owning these tables
// rather than importing them.
//
// Every line of NormalizationTest.txt, reduced by internal/normgen to the two
// columns a canonical fold answers to, and asserted three times: the source,
// its NFC and its NFD are canonically equivalent by definition, so all three
// have to decompose to the same thing. The third of those is what catches a
// fold that only works on input already in the shape it expects.
//
// The oracle is the standard itself. That matters more than the count: a
// differential test against another implementation inherits that
// implementation's opinion, and this inherits Unicode's.
func TestUnicodesOwnConformanceFile(t *testing.T) {
	t.Parallel()
	f, err := os.Open("testdata/conformance.tsv.gz")
	if err != nil {
		t.Fatalf("fixture: %v — regenerate with internal/normgen", err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gz.Close() }()

	var cases, failures int
	sc := bufio.NewScanner(gz)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		src, want, ok := strings.Cut(sc.Text(), "\t")
		if !ok {
			continue
		}
		cases++
		if got := unorm.NFD(src); got != want {
			failures++
			// Capped, because a table regenerated from the wrong file would
			// otherwise print sixty thousand lines and bury the count that
			// actually tells you what happened.
			if failures <= 10 {
				t.Errorf("NFD(%q) = %q, want %q", src, got, want)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if cases < 50000 {
		t.Fatalf("only %d cases — the fixture is truncated and this test is not testing much", cases)
	}
	if failures > 0 {
		t.Errorf("%d of %d conformance cases disagree with Unicode %s", failures, cases, unorm.Version)
	}
	t.Logf("%d conformance cases against Unicode %s", cases, unorm.Version)
}

// TestTheFoldIsIdempotent, because a normal form that is not a fixed point is
// not a normal form, and the gate folds a pattern once and a path once and
// compares — so any drift between the two passes is a hole.
func TestTheFoldIsIdempotent(t *testing.T) {
	t.Parallel()
	for _, s := range []string{
		"café", "café", "/p/.env", "가힣", "Ḍ̇",
		"q̣̇", "Å", "", "/",
	} {
		once := unorm.NFD(s)
		if twice := unorm.NFD(once); twice != once {
			t.Errorf("NFD(%q) = %q, NFD again = %q", s, once, twice)
		}
	}
}

// TestASCIIIsUntouched pins the fast path against the slow one. If they ever
// disagree the fast path is a silent wrong answer, which is the worst shape a
// bug in this package could take: paths are overwhelmingly ASCII, so it would
// be wrong almost everywhere and tested almost nowhere.
func TestASCIIIsUntouched(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"/proj/.env", "/a/b/c", "", "/", "AZaz09._-"} {
		if got := unorm.NFD(s); got != s {
			t.Errorf("NFD(%q) = %q, want it unchanged", s, got)
		}
	}
}

// TestCompatibilityIsNotFolded. NFKD maps these onto their plain spellings
// and NFD must not: no filesystem treats `ﬁ` and `fi` as one name, so a gate
// that folded them would refuse files nobody denied, which is the half of
// this mistake that refuses too much.
func TestCompatibilityIsNotFolded(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ in, notWant string }{
		{"ﬁ", "fi"},  // ligature fi
		{"①", "1"},   // circled digit one
		{"Ａ", "A"},   // fullwidth A
		{"½", "1⁄1"}, // vulgar fraction half
	} {
		if got := unorm.NFD(c.in); got == c.notWant {
			t.Errorf("NFD(%q) = %q — that is a compatibility mapping, not a canonical one", c.in, got)
		}
	}
}

// TestHangulDecomposesByArithmetic covers the 11172 syllables that are
// deliberately absent from the table. A generator that did not know they were
// algorithmic would emit a table with a hole in it and fail to fold every
// Korean name, silently.
func TestHangulDecomposesByArithmetic(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ in, want string }{
		{"가", "가"},  // LV
		{"각", "각"}, // LVT
		{"힣", "힣"}, // the last syllable
	} {
		if got := unorm.NFD(c.in); got != c.want {
			t.Errorf("NFD(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestMarksOfEqualClassKeepTheirOrder is the stability rule, which a sort
// that used >= instead of > would break. The two marks here share class 230,
// so their written order is meaningful and must survive.
func TestMarksOfEqualClassKeepTheirOrder(t *testing.T) {
	t.Parallel()
	const same = "à́" // grave then acute, both class 230
	if got := unorm.NFD(same); got != same {
		t.Errorf("NFD(%q) = %q — marks of one class were reordered", same, got)
	}
}

// TestAClassZeroCharacterIsAWall. Reordering must not carry a mark across a
// starter onto a different base letter, which is the way an in-place sort of
// the whole string goes wrong.
func TestAClassZeroCharacterIsAWall(t *testing.T) {
	t.Parallel()
	const s = "a̧b́" // cedilla (202) on a, acute (230) on b
	if got := unorm.NFD(s); got != s {
		t.Errorf("NFD(%q) = %q — a mark crossed a starter", s, got)
	}
}
