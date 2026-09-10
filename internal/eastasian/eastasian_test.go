// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package eastasian

import "testing"

// TestTheWideTableIsSortedAndDisjoint, because the lookup is a binary search
// and a table out of order would answer wrongly for everything past the
// mistake — quietly, and only for some characters.
func TestTheWideTableIsSortedAndDisjoint(t *testing.T) {
	if len(wideRanges) == 0 {
		t.Fatal("no ranges, so nothing is measured as wide")
	}
	for i, r := range wideRanges {
		if r[0] > r[1] {
			t.Errorf("range %d is backwards: %#x..%#x", i, r[0], r[1])
		}
		if i > 0 && r[0] <= wideRanges[i-1][1]+1 {
			t.Errorf("range %d starts at %#x, which touches or overlaps the one before it ending at %#x",
				i, r[0], wideRanges[i-1][1])
		}
	}
	if Version == "" {
		t.Error("no Unicode version recorded, so nothing says which one this measured")
	}
}

// TestWideIsTheEastAsianClasses, because the binary search above it is only
// as good as the answers it gives at the edges of a range. Latin is one
// cell, the CJK block and the emoji that were measured as two columns in a
// real prompt are two, and a code point just outside a range is not.
func TestWideIsTheEastAsianClasses(t *testing.T) {
	for _, tc := range []struct {
		r    rune
		want bool
	}{
		{'a', false},
		{'é', false},
		{0x10ff, false},
		{0x1100, true},
		{0x115f, true},
		{0x1160, false},
		{'日', true},
		{'本', true},
		{0x1f44d, true},
	} {
		if got := Wide(tc.r); got != tc.want {
			t.Errorf("Wide(%#x) = %v, want %v", tc.r, got, tc.want)
		}
	}
}
