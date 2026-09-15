// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"math"
	"testing"
)

// A field number is read the way C's `strtol` reads one: saturating at the top
// of the type rather than wrapping into it.
//
// The difference is visible from outside and is measured rather than chosen —
// `printf '[%99999999999999999999s]' x` is `[x]` in zsh, which is LONG_MAX
// truncated to -1 and a width of one. A value wrapped modulo 2^64 instead
// lands somewhere else and pads.
func TestAFieldNumberSaturatesRatherThanWrapping(t *testing.T) {
	for _, tc := range []struct {
		name, digits string
		want         int64
	}{
		{"an ordinary width", "10", 10},
		{"the edge itself", "2147483647", math.MaxInt32},
		{"ten times the edge", "21474836470", 21474836470},
		{"past a 64-bit integer", "99999999999999999999", math.MaxInt64},
		{"very far past it", "999999999999999999999999999999", math.MaxInt64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := printfFieldNumber(tc.digits, 0, len(tc.digits)); got != tc.want {
				t.Errorf("printfFieldNumber(%q) = %d, want %d", tc.digits, got, tc.want)
			}
		})
	}
}

// Storing a field number in a C int is what the wrapping columns do, and a
// width that comes out negative is a left-justified one.
//
// INT_MIN is the row worth having: negating it wraps back to itself, so a
// magnitude taken carelessly puts a two-billion-wide field where the panel
// writes nothing at all.
func TestAFieldNumberWrapsIntoAnInt(t *testing.T) {
	for _, tc := range []struct {
		name      string
		in        int64
		wantWidth int
		wantLeft  bool
	}{
		{"inside the int", 10, 10, false},
		{"one turn of the wheel", 4294967306, 10, false},
		{"ten times the edge is minus ten", 21474836470, 10, true},
		{"the saturated value is minus one", math.MaxInt64, 1, true},
		{"INT_MIN has no magnitude", 2147483648, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, left := printfFieldWrap(tc.in)
			if w != tc.wantWidth || left != tc.wantLeft {
				t.Errorf("printfFieldWrap(%d) = %d, %v, want %d, %v",
					tc.in, w, left, tc.wantWidth, tc.wantLeft)
			}
		})
	}
}

// The scanner answers at the widest boundary any reading uses — INT_MAX itself
// — and says which side of it the number landed on, because the two refusing
// columns turn at the edge and the empty ones turn one past it.
//
// The wrapped spec is the other half: it is what the wrapping columns format
// with, so a `-` flag arriving from a negative width has to be in it and a
// negative precision has to be gone from it, dot and all.
func TestAFieldBeyondAnIntIsFoundAndWrapped(t *testing.T) {
	for _, tc := range []struct {
		name, spec  string
		wantBeyond  bool
		wantAtEdge  bool
		wantWrapped string
	}{
		{"an ordinary width", "%6", false, false, "%6"},
		{"no field at all", "%", false, false, "%"},
		{"a width just inside", "%2147483646", false, false, "%2147483646"},
		{"the edge itself", "%2147483647", true, true, "%2147483647"},
		{"a width that wraps positive", "%4294967306", true, false, "%10"},
		{"a width that wraps negative", "%21474836470", true, false, "%-10"},
		{"a flag is kept in front", "%021474836470", true, false, "%0-10"},
		{"a minus already there is not doubled", "%-21474836470", true, false, "%-10"},
		{"a precision that wraps positive", "%.4294967306", true, false, "%.10"},
		{"a negative precision is omitted, dot and all", "%.21474836470", true, false, "%"},
		{"both at once", "%4294967306.4294967306", true, false, "%10.10"},
		{"a length modifier is kept", "%21474836470l", true, false, "%-10l"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			beyond, atEdge, wrapped := printfFieldBeyondAnInt(tc.spec)
			if beyond != tc.wantBeyond || atEdge != tc.wantAtEdge || wrapped != tc.wantWrapped {
				t.Errorf("printfFieldBeyondAnInt(%q) = %v, %v, %q, want %v, %v, %q",
					tc.spec, beyond, atEdge, wrapped,
					tc.wantBeyond, tc.wantAtEdge, tc.wantWrapped)
			}
		})
	}
}
