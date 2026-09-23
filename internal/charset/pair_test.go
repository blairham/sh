// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package charset

import "testing"

// Where a character *ends*, which is the one question from the decode side
// this package answers.
//
// It exists because a trail byte can be `0x5C`: in Big5 the character U+03B1
// is `a3 5c`, so a reader walking bytes finds a backslash in the middle of a
// letter (#4235). Every row below is taken from this package's own tables
// rather than from a range written out by hand — the encode direction already
// holds every code the charset writes, and [Width] searches those.
func TestWidthIsTwoForAPairTheCharsetWrites(t *testing.T) {
	// The pair from the issue, read back out of the table so the row cannot
	// drift from the data it is about.
	alpha, ok := Encode("Big5", 0x03B1)
	if !ok || len(alpha) != 2 || alpha[1] != 0x5C {
		t.Fatalf("Big5 writes U+03B1 as % x, want a pair ending in 5c", alpha)
	}
	kana, ok := Encode("SJIS", 0x30A2)
	if !ok || len(kana) != 2 {
		t.Fatalf("Shift-JIS writes U+30A2 as % x, want a pair", kana)
	}
	for _, c := range []struct {
		name    string
		codeset string
		b, next byte
		want    int
	}{
		{"the character from the issue, whose trail byte is a backslash", "Big5", alpha[0], alpha[1], 2},
		{"spelled the way a locale spells the name", "zh_TW.Big5's charset is big5", alpha[0], alpha[1], 1},
		{"a lead byte with nothing after it is a byte", "Big5", alpha[0], 0, 1},
		{"and so is one a newline follows", "Big5", alpha[0], '\n', 1},
		{"its own trail byte leads nothing", "Big5", 0x5C, alpha[1], 1},
		{"ASCII is one byte whatever follows it", "Big5", 'a', alpha[1], 1},
		{"a Shift-JIS character", "SJIS", kana[0], kana[1], 2},
		{"a pair in neither charset's table", "Big5", 0xFF, 0xFF, 1},
		{"a single-byte charset has no pairs", "ISO8859-1", 0xE9, 0xA9, 1},
		{"and UTF-8 is answered 1 rather than decoded", "UTF-8", 0xCE, 0xB1, 1},
		{"a charset with no table here", "GB18030", 0xA3, 0x5C, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := Width(c.codeset, c.b, c.next); got != c.want {
				t.Errorf("Width(%q, %#02x, %#02x) = %d, want %d",
					c.codeset, c.b, c.next, got, c.want)
			}
		})
	}
}

// Every pair of bytes the tables write answers 2 and every other pair answers
// 1 — asserted over all 65536 of them rather than over a handful, because the
// set is derived from the encode direction and a derivation is only as good as
// its coverage.
func TestWidthAgreesWithTheTableOverEveryPair(t *testing.T) {
	for _, codeset := range []string{"Big5", "SJIS"} {
		table := lookupMultibyte(normalize(codeset))
		if table == nil {
			t.Fatalf("no table for %q", codeset)
		}
		want := map[uint16]bool{}
		for _, code := range table.vals {
			if code >= 0x100 {
				want[code] = true
			}
		}
		for b := 0; b < 256; b++ {
			for next := 0; next < 256; next++ {
				expect := 1
				if want[uint16(b)<<8|uint16(next)] {
					expect = 2
				}
				if got := Width(codeset, byte(b), byte(next)); got != expect {
					t.Fatalf("Width(%q, %#02x, %#02x) = %d, want %d",
						codeset, b, next, got, expect)
				}
			}
		}
		// And no character begins with an ASCII byte, which is what makes a
		// shell's own metacharacters safe to compare as bytes.
		for code := range want {
			if code>>8 < 0x80 {
				t.Errorf("%q writes %#04x, whose lead byte is ASCII", codeset, code)
			}
		}
	}
}
