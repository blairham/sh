// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package charset

import (
	"slices"
	"testing"
)

// TestEncodeIsTheBytesTheReferenceShellsWrote, because the whole point of the
// package is a byte a real shell was measured writing. Every row here was
// read off `printf '%s' $'\uHHHH' | od -An -tx1` under bash 5.3.15 and zsh
// 5.9.2 on 2026-09-15, with the locale named in the first column.
func TestEncodeIsTheBytesTheReferenceShellsWrote(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		codeset string
		r       rune
		want    []byte
		ok      bool
	}{
		{"ISO8859-1", 0x00e9, []byte{0xe9}, true},
		{"ISO8859-1", 0x00a4, []byte{0xa4}, true},
		{"ISO8859-1", 0x20ac, nil, false},
		{"ISO8859-15", 0x00e9, []byte{0xe9}, true},
		{"ISO8859-15", 0x20ac, []byte{0xa4}, true},
		{"ISO8859-15", 0x00a4, nil, false},
		{"ISO8859-5", 0x0411, []byte{0xb1}, true},
		{"CP1251", 0x0411, []byte{0xc1}, true},
		{"CP1251", 0x20ac, []byte{0x88}, true},
		{"KOI8-R", 0x0411, []byte{0xe2}, true},
		// The spellings a locale name is written with, all one charset.
		{"iso-8859-1", 0x00e9, []byte{0xe9}, true},
		{"iso88591", 0x00e9, []byte{0xe9}, true},
		{"windows-1251", 0x0411, []byte{0xc1}, true},
		// ASCII is the same byte in every one of them, and in one this
		// package holds no table for.
		{"ISO8859-1", 'A', []byte{'A'}, true},
		{"eucJP", 'A', []byte{'A'}, true},

		// The multibyte charsets, and the point of them: width is the code
		// point's and not the charset's. Shift-JIS writes the halfwidth
		// katakana in one byte, the kanji in two, and Big5 writes the same
		// kanji as two different bytes.
		{"SJIS", 0xff9f, []byte{0xdf}, true},
		{"SJIS", 0x4e00, []byte{0x88, 0xea}, true},
		{"Big5", 0x4e00, []byte{0xa4, 0x40}, true},
		{"SJIS", 0x3042, []byte{0x82, 0xa0}, true},
		// `SJIS` is how a locale spells it and `SHIFTJIS` is how Unicode
		// files it; both, and the separator spellings, reach one table.
		{"SHIFTJIS", 0x4e00, []byte{0x88, 0xea}, true},
		{"Shift_JIS", 0x4e00, []byte{0x88, 0xea}, true},
		{"big5", 0x4e00, []byte{0xa4, 0x40}, true},
		// Shift-JIS is the charset whose low half is not ASCII, and both
		// answers were measured. It stands YEN SIGN in 0x5C, and it writes
		// U+005C there as well.
		{"SJIS", 0x00a5, []byte{0x5c}, true},
		{"SJIS", 0x005c, []byte{0x5c}, true},
		{"SJIS", 0x203e, []byte{0x7e}, true},
		{"SJIS", 0x007e, []byte{0x7e}, true},
		// Big5 holds the hiragana too, in a different byte pair — measured,
		// because "Big5 is Chinese so it has no kana" is the plausible guess
		// and it is wrong.
		{"Big5", 0x3042, []byte{0xc6, 0xa6}, true},
		// A charset that has a table but not this character. Neither of them
		// predates the euro, and both shells write the escape back for it.
		{"SJIS", 0x20ac, nil, false},
		{"Big5", 0x20ac, nil, false},
		// A charset with no table, a locale that named none, and a value
		// past Unicode: three ways to be unrepresentable, one answer.
		{"eucJP", 0xff9f, nil, false},
		{"", 0x00e9, nil, false},
		{"ISO8859-1", 0x110000, nil, false},
		{"SJIS", 0x110000, nil, false},
	} {
		got, ok := Encode(tc.codeset, tc.r)
		if ok != tc.ok || (ok && !slices.Equal(got, tc.want)) {
			t.Errorf("Encode(%q, %#x) = % #02x, %v; want % #02x, %v",
				tc.codeset, tc.r, got, ok, tc.want, tc.ok)
		}
	}
}

// TestKnownIsAboutTheCharsetAndNotTheCharacter, because the two questions are
// easy to collapse into one and a caller that collapsed them would treat a
// character the charset simply lacks as a charset nobody has a table for.
func TestKnownIsAboutTheCharsetAndNotTheCharacter(t *testing.T) {
	t.Parallel()
	if !Known("ISO8859-1") {
		t.Error("ISO8859-1 has a table and Known says it does not")
	}
	if _, ok := Encode("ISO8859-1", 0x20ac); ok {
		t.Error("ISO 8859-1 has no euro sign and Encode found one")
	}
	if !Known("SJIS") {
		t.Error("SJIS has a table and Known says it does not")
	}
	if _, ok := Encode("SJIS", 0x20ac); ok {
		t.Error("Shift-JIS has no euro sign and Encode found one")
	}
	if Known("eucJP") {
		t.Error("eucJP has no table here and Known says it has one")
	}
	if Known("") {
		t.Error("a locale naming no charset at all is not a charset")
	}
}

// TestEveryTableIsWholeAndDistinct, because the tables are generated and a
// generator that mis-parsed one file would leave it plausible-looking: an
// empty table answers false for everything, and a duplicated code point makes
// Encode's answer depend on which byte it reaches first.
func TestEveryTableIsWholeAndDistinct(t *testing.T) {
	t.Parallel()
	if len(tables) == 0 {
		t.Fatal("no charsets, so nothing is encodable")
	}
	for name, table := range tables {
		defined, seen := 0, map[rune]int{}
		for i, r := range table {
			if r < 0 {
				continue
			}
			defined++
			if r < 0x80 {
				t.Errorf("%s: byte %#02x stands for ASCII %#x, which Encode answers without a table",
					name, 0x80+i, r)
			}
			if first, dup := seen[r]; dup {
				t.Errorf("%s: %#04x is at both %#02x and %#02x", name, r, 0x80+first, 0x80+i)
			}
			seen[r] = i
		}
		// A high half with almost nothing in it is a parse that found the
		// file and not its rows. The emptiest charset here is ISO 8859-6,
		// which defines a little over half of its high half.
		if defined < 64 {
			t.Errorf("%s: only %d of 128 high bytes are defined", name, defined)
		}
	}
}

// TestEveryMultibyteTableIsSortedAndDistinct, because the lookup is a binary
// search: a table out of order answers wrongly for everything past the
// mistake, quietly and only for some characters, and two rows for one code
// point would make the answer depend on where the search landed.
func TestEveryMultibyteTableIsSortedAndDistinct(t *testing.T) {
	t.Parallel()
	if len(multibyteTables) == 0 {
		t.Fatal("no multibyte charsets, so the whole of #3029 is undone")
	}
	for name, table := range multibyteTables {
		if len(table.keys) != len(table.vals) {
			t.Fatalf("%s: %d keys against %d values, so the two have drifted apart",
				name, len(table.keys), len(table.vals))
		}
		// A table with a handful of rows is a parse that found the file and
		// not its rows. The smaller of the two here has a little under 7,000.
		if len(table.keys) < 4096 {
			t.Errorf("%s: only %d code points, which is too few to be the charset",
				name, len(table.keys))
		}
		for i, r := range table.keys {
			if r < 0x80 {
				t.Errorf("%s: %#04x is ASCII, which Encode answers without a table",
					name, r)
			}
			if r == 0xfffd {
				t.Errorf("%s: U+FFFD is a mapping file's word for a hole, not a character",
					name)
			}
			if i > 0 && table.keys[i-1] >= r {
				t.Errorf("%s: %#04x follows %#04x, so the binary search is broken from here on",
					name, r, table.keys[i-1])
			}
		}
	}
}

// TestNoCharsetIsBothSingleByteAndMultibyte, because [Encode] asks the
// multibyte tables first and a name in both would make the single-byte table
// unreachable without anything saying so.
func TestNoCharsetIsBothSingleByteAndMultibyte(t *testing.T) {
	t.Parallel()
	for name := range multibyteTables {
		if _, both := tables[name]; both {
			t.Errorf("%s has a table of each shape and only one of them is ever read", name)
		}
	}
	for name, target := range aliases {
		_, single := tables[target]
		_, multi := multibyteTables[target]
		if !single && !multi {
			t.Errorf("the alias %s points at %s, which is no charset at all", name, target)
		}
	}
}

// Multibyte is answered from the table rather than from a list of names, which
// is what keeps it in step with [Width]: the two charsets that have a width
// above one are the two that say yes, an alias of one of them says yes, and a
// single-byte charset says no even though [Known] says yes about it.
func TestMultibyte(t *testing.T) {
	for _, c := range []struct {
		codeset string
		want    bool
	}{
		{"Big5", true},
		{"big5", true},
		{"SJIS", true},
		{"ISO8859-1", false},
		// Known about, and one byte per character — the case a caller reaching
		// for Known instead of this would get wrong.
		{"UTF-8", false},
		{"C", false},
		{"", false},
		{"eucJP", false},
	} {
		if got := Multibyte(c.codeset); got != c.want {
			t.Errorf("Multibyte(%q) = %v, want %v", c.codeset, got, c.want)
		}
	}
}
