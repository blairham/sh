// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package charset

import "testing"

// TestEncodeIsTheBytesTheReferenceShellsWrote, because the whole point of the
// package is a byte a real shell was measured writing. Every row here was
// read off `printf '%s' $'\uHHHH' | od -An -tx1` under bash 5.3.15 and zsh
// 5.9.2 on 2026-09-15, with the locale named in the first column.
func TestEncodeIsTheBytesTheReferenceShellsWrote(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		codeset string
		r       rune
		want    byte
		ok      bool
	}{
		{"ISO8859-1", 0x00e9, 0xe9, true},
		{"ISO8859-1", 0x00a4, 0xa4, true},
		{"ISO8859-1", 0x20ac, 0, false},
		{"ISO8859-15", 0x00e9, 0xe9, true},
		{"ISO8859-15", 0x20ac, 0xa4, true},
		{"ISO8859-15", 0x00a4, 0, false},
		{"ISO8859-5", 0x0411, 0xb1, true},
		{"CP1251", 0x0411, 0xc1, true},
		{"CP1251", 0x20ac, 0x88, true},
		{"KOI8-R", 0x0411, 0xe2, true},
		// The spellings a locale name is written with, all one charset.
		{"iso-8859-1", 0x00e9, 0xe9, true},
		{"iso88591", 0x00e9, 0xe9, true},
		{"windows-1251", 0x0411, 0xc1, true},
		// ASCII is the same byte in every one of them, and in one this
		// package holds no table for.
		{"ISO8859-1", 'A', 'A', true},
		{"SJIS", 'A', 'A', true},
		// A charset with no table, a locale that named none, and a value
		// past Unicode: three ways to be unrepresentable, one answer.
		{"SJIS", 0xff9f, 0, false},
		{"", 0x00e9, 0, false},
		{"ISO8859-1", 0x110000, 0, false},
	} {
		got, ok := Encode(tc.codeset, tc.r)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("Encode(%q, %#x) = %#02x, %v; want %#02x, %v",
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
	if Known("SJIS") {
		t.Error("SJIS has no table here and Known says it has one")
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
