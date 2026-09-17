// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// The hand renderers a precision past `fmt`'s ceiling reaches write what
// `fmt` writes.
//
// This is the whole guard on printfWideInteger and printfWideFloat. They exist
// because `fmt` gives up past printfFmtFieldCeiling (#3017), and a renderer
// that only ever runs where nothing can compare it is a renderer nobody is
// grading — so it is graded here at precisions `fmt` does render, over every
// flag and width and value the wide band can carry. A field ten million wide
// is the same layout as one five wide with more zeros in the middle.
//
// The rows where `fmt` and C disagree are skipped and pinned separately
// below, because there the answer to agree with is not `fmt`'s.
func TestTheWidePrecisionRenderersAgreeWithFmt(t *testing.T) {
	flagsets := []string{"", "-", "0", "-0", "+", " ", "#", "#0", "+-", "# "}
	widths := []string{"", "1", "8", "20"}
	precs := []string{"0", "1", "3", "5", "12"}

	t.Run("integers", func(t *testing.T) {
		for _, verb := range []byte{'d', 'o', 'x', 'X'} {
			signed := verb == 'd'
			for _, flags := range flagsets {
				for _, width := range widths {
					for _, prec := range precs {
						spec := "%" + flags + width + "." + prec
						if !signed {
							// The unsigned conversions reach the renderer
							// with their sign flags already taken out.
							spec = printfWithoutSignFlags(spec)
						}
						for _, n := range []int64{0, 1, -1, 42, -42, 255, 1 << 62} {
							if n == 0 && strings.ContainsAny(flags, "+ #") {
								continue // see TestAValueOfNoughtPartsFmtFromC
							}
							got := printfWideInteger(spec, verb, n, signed)
							want := fmt.Sprintf(spec+string(verb), uint64(n))
							if signed {
								want = fmt.Sprintf(spec+"d", n)
							}
							if got != want {
								t.Errorf("%s%c of %d: got %q, want %q", spec, verb, n, got, want)
							}
						}
					}
				}
			}
		}
	})

	t.Run("floats", func(t *testing.T) {
		for _, verb := range []byte{'f', 'e', 'E', 'g', 'G'} {
			for _, flags := range flagsets {
				for _, width := range widths {
					for _, prec := range precs {
						spec := "%" + flags + width + "." + prec
						// A negative zero, which keeps its sign through the
						// conversion. Built rather than written: Go reads the
						// literal -0.0 as a constant zero.
						for _, f := range []float64{0, 1, -1, 1.5, -1.5, 0.1, 1e20, math.Copysign(0, -1)} {
							got := printfWideFloat(spec, verb, f)
							want := fmt.Sprintf(spec+string(verb), f)
							if got != want {
								t.Errorf("%s%c of %v: got %q, want %q", spec, verb, f, got, want)
							}
						}
					}
				}
			}
		}
	})

	t.Run("strings", func(t *testing.T) {
		for _, flags := range []string{"", "-"} {
			for _, width := range widths {
				for _, prec := range precs {
					spec := "%" + flags + width + "." + prec
					for _, s := range []string{"", "a", "abcdef"} {
						got := printfByteField(spec, s)
						want := fmt.Sprintf(spec+"s", s)
						if got != want {
							t.Errorf("%ss of %q: got %q, want %q", spec, s, got, want)
						}
					}
				}
			}
		}
	})
}

// The rows the test above skips, and why they are skipped: Go's `fmt` and C
// disagree about a value of nought, and it is `fmt` that is wrong.
//
// Measured 2026-09-15 against bash 5.3.20 and zsh 5.9, which agree on every
// row. None of them can be reached from the wide band — a precision of ten
// million is not a precision of nought, and a `0x` is not a digit a precision
// counts — so the hand renderer being right about them changes nothing today.
// The *narrow* path writes `fmt`'s answer and is wrong about all three
// (#3024); this table is here so that fixing it has a statement of what the
// answer is, and so the skip above is a measurement rather than a shrug.
func TestAValueOfNoughtPartsFmtFromC(t *testing.T) {
	for _, tc := range []struct {
		spec string
		verb byte
		want string
	}{
		// A sign flag is written even where the precision left no digits.
		{"%+.0", 'd', "+"},
		{"% .0", 'd', " "},
		// `#` on an octal raises the precision until there is a leading
		// zero, and a precision of nought is not exempt.
		{"%#.0", 'o', "0"},
		// `#` on a hexadecimal writes `0x` only for a *nonzero* value.
		{"%#.1", 'x', "0"},
		{"%#.3", 'X', "000"},
		{"%#", 'x', "0"},
	} {
		t.Run(tc.spec+string(tc.verb), func(t *testing.T) {
			signed := tc.verb == 'd'
			spec := tc.spec
			if !signed {
				spec = printfWithoutSignFlags(spec)
			}
			if got := printfWideInteger(spec, tc.verb, 0, signed); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// printfSpecParts tells a precision the spec does not write from one it
// writes as nought, which is C's own distinction and the thing a `-1` is for.
func TestPrintfSpecPartsReadsAnAbsentPrecisionApart(t *testing.T) {
	for _, tc := range []struct {
		spec, flags string
		width, prec int
	}{
		{"%", "", 0, -1},
		{"%8", "", 8, -1},
		{"%-08", "-0", 8, -1},
		{"%.", "", 0, 0},
		{"%.0", "", 0, 0},
		{"%8.3", "", 8, 3},
		{"%+ #8.10000010", "+ #", 8, 10000010},
	} {
		t.Run(tc.spec, func(t *testing.T) {
			flags, width, prec := printfSpecParts(tc.spec)
			if flags != tc.flags || width != tc.width || prec != tc.prec {
				t.Errorf("got %q %d %d, want %q %d %d",
					flags, width, prec, tc.flags, tc.width, tc.prec)
			}
			if wide := printfWidePrecision(tc.spec); wide != (tc.prec > printfFmtFieldCeiling) {
				t.Errorf("printfWidePrecision = %v", wide)
			}
		})
	}
}
