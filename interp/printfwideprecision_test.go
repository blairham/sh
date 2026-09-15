// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"
	"testing"
)

// The precision is found where the precision is, and not wherever a run of
// digits turns up.
//
// A spec carries two numbers and only the second one is this scanner's: a
// width past the ceiling is printfWideField's business and a `0` in either of
// them is not the zero flag. Reading the wrong run is how `%10000010.2f` would
// come out capped at a precision it never had.
//
// In-package because it names an unexported split; the behavior it protects is
// pinned from outside by TestAPrecisionWiderThanFmtRendersIsWrittenOutHere.
func TestPrintfWidePrecisionReadsTheSecondNumber(t *testing.T) {
	const ceiling = printfFmtWidthCeiling
	capped := strconv.Itoa(ceiling)
	for _, tc := range []struct {
		name, spec, wantCapped string
		wantPrec               int
	}{
		{"a bare wide precision", "%.10000010", "%." + capped, ceiling + 1},
		{"flags in front of it", "%-0.10000010", "%-0." + capped, ceiling + 1},
		{"a width in front of it", "%20.10000010", "%20." + capped, ceiling + 1},
		{"a length modifier behind it", "%.10000010l", "%." + capped + "l", ceiling + 1},
		{"far past the ceiling", "%.2147483646", "%." + capped, 2147483646},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, prec, wide := printfWidePrecision(tc.spec)
			if !wide {
				t.Fatalf("printfWidePrecision(%q) did not report a wide precision", tc.spec)
			}
			if got != tc.wantCapped || prec != tc.wantPrec {
				t.Errorf("capped=%q prec=%d, want %q and %d",
					got, prec, tc.wantCapped, tc.wantPrec)
			}
		})
	}
	// Everything `fmt` will render is left entirely alone, which is what
	// makes this cost nothing on an ordinary conversion.
	for _, spec := range []string{
		"%.20",       // an ordinary precision
		"%10000010",  // a wide *width*, which is not this one's business
		"%10000010d", // and the same with its conversion attached
		"%.10000009", // the ceiling itself is not past the ceiling
		"%.",         // a precision of zero written with no digits
		"%-20",       // no precision at all
		"%",
	} {
		if capped, prec, wide := printfWidePrecision(spec); wide {
			t.Errorf("printfWidePrecision(%q) = %q, %d, wide; want it left alone",
				spec, capped, prec)
		}
	}
}

// The zeros a wide precision owes go on the end the *verb* puts them on, and
// the two ends are opposites.
//
// An integer's precision is a minimum digit count and pads on the left; a
// float's is digits after the point and runs out on the right. Splicing at the
// wrong end is not a rounding error but a different number — and it is
// invisible on the `1` every measurement of this starts from, whose fraction
// is zeros whichever end you pick. So every row here has a value whose digits
// are not.
func TestPrintfWideDigitsSplicesAtTheEndTheVerbFills(t *testing.T) {
	const ceiling = printfFmtWidthCeiling
	fraction := func(digits string) string {
		return "0." + digits + strings.Repeat("0", ceiling-len(digits))
	}
	for _, tc := range []struct {
		name string
		verb byte
		in   string
		want string
	}{
		{
			"a fraction grows on the right",
			'f', fraction("125"), fraction("125") + "00",
		},
		{
			"an exponent stays behind the digits it belongs to",
			'e', "1." + strings.Repeat("0", ceiling) + "e-01",
			"1." + strings.Repeat("0", ceiling+2) + "e-01",
		},
		{
			"a capital exponent too",
			'E', "1." + strings.Repeat("0", ceiling) + "E-01",
			"1." + strings.Repeat("0", ceiling+2) + "E-01",
		},
		{
			"an integer grows on the left, behind its sign",
			'd', "-" + strings.Repeat("0", ceiling-1) + "7",
			"-" + strings.Repeat("0", ceiling+1) + "7",
		},
		{
			"and behind a `#`'s prefix",
			'x', "0x" + strings.Repeat("0", ceiling-2) + "ff",
			"0x" + strings.Repeat("0", ceiling) + "ff",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := printfWideDigits(tc.in, tc.verb, ceiling+2); got != tc.want {
				// Ten megabytes is not a diff anybody reads, so report the
				// shape rather than the string.
				t.Errorf("len %d, head %q, want len %d, head %q",
					len(got), head(got), len(tc.want), head(tc.want))
			}
		})
	}
}

func head(s string) string {
	if len(s) > 24 {
		return s[:24] + "…"
	}
	return s
}

// A precision only truncates for the string conversions, so the cut is made on
// the text rather than by capping the precision back to the ceiling.
//
// The difference shows only on an operand longer than the ceiling, which is
// the one case a cap would get wrong — and getting it right costs a `len`.
func TestPrintfStringCutsTheTextRatherThanCappingThePrecision(t *testing.T) {
	const ceiling = printfFmtWidthCeiling
	long := strings.Repeat("x", ceiling+5)
	if got := printfString("%."+strconv.Itoa(ceiling+2), long); len(got) != ceiling+2 {
		t.Errorf("a text past the ceiling was cut to %d, want %d", len(got), ceiling+2)
	}
	// A text the precision cannot reach keeps every byte.
	if got := printfString("%.10000010", "xyz"); got != "xyz" {
		t.Errorf("got %q, want %q", head(got), "xyz")
	}
	// And the width is still the field's, laid on what the cut left.
	if got := printfString("%6.10000010", "xyz"); got != "   xyz" {
		t.Errorf("got %q, want %q", head(got), "   xyz")
	}
}
