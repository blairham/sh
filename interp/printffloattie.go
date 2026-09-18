// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// printfFloatTie answers the value a floating conversion is laid out from,
// which is the operand itself in every column but one.
//
// The disagreement is about a **half and nothing else** — a value the
// conversion's precision leaves exactly between two renderings — so that is
// the only shape the axis is asked about. Everything else rounds the same way
// in all six columns and never reaches the question; see
// PrintfFloatHalfPolicy for the measurements.
//
// The answer is given as one step of the operand rather than as a rendering
// of its own, and that is what keeps this out of the layout: a half nudged by
// a single representable step is no longer a half, so the flags, the width
// and the precision are still laid out by the one renderer every other
// operand goes through. A step is many orders of magnitude below the place
// being rounded — a value that is a half at that place is representable
// there, which bounds the step well under it — so the nudge can only decide
// the tie and can never carry past it.
func (r *Runner) printfFloatTie(spec string, verb byte, f float64) float64 {
	place, kept, tenth, ok := printfRoundingPlace(spec, verb, f)
	if !ok || !printfIsExactHalf(f, place) {
		return f
	}
	toward := math.Inf(-1)
	if math.Signbit(f) {
		toward = math.Inf(1)
	}
	switch r.floatHalf() {
	case PrintfFloatHalfAwayFromZeroAboveATenth:
		if kept <= 0 || !tenth {
			// The other arm of the same rule, and the one a bool could not
			// have held: below a tenth, and where the precision reaches no
			// digit of the value at all, the same column takes the smaller
			// magnitude instead.
			return math.Nextafter(f, toward)
		}
		return math.Nextafter(f, -toward)
	}
	return f
}

// printfRoundingPlace reports the decimal place a conversion rounds at, how
// many digits it keeps in front of it, and whether the value is a tenth or
// more. The last two are what the away-from-zero reading is conditioned on.
//
// The place is counted after the decimal point, so 1 is the tenths and -1 the
// tens. The count is what says whether the conversion reaches the value at
// all: `%.0f` of 0.5 keeps nothing.
func printfRoundingPlace(spec string, verb byte, f float64) (place, kept int, tenth, ok bool) {
	if f == 0 || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, 0, false, false
	}
	_, _, prec := printfSpecParts(spec)
	if prec < 0 {
		// The default precision of every floating conversion C has.
		prec = 6
	}
	e := printfDecimalExponent(f)
	// A tenth is where the leading digit is the first one after the point.
	tenth = e >= -1
	switch verb {
	case 'f', 'F':
		// A precision is digits after the point, so the place is the
		// precision and the digits kept run from the leading one.
		return prec, e + 1 + prec, tenth, true
	case 'e', 'E':
		// One digit in front of the point and the precision after it, the
		// point itself following the value's own exponent.
		return prec - e, prec + 1, tenth, true
	case 'g', 'G':
		// A precision is significant digits here, and C reads a nought as
		// one. Which style is chosen is settled after the rounding and does
		// not move the place the rounding happens at.
		sig := prec
		if sig == 0 {
			sig = 1
		}
		return sig - 1 - e, sig, tenth, true
	}
	return 0, 0, false, false
}

// printfDecimalExponent reports the power of ten of a value's leading digit,
// read off the shortest rendering that names it rather than computed: a
// logarithm at the boundary answers the neighboring power for values that
// are exactly a power of ten, which is precisely where a half lands.
func printfDecimalExponent(f float64) int {
	s := strconv.FormatFloat(math.Abs(f), 'e', -1, 64)
	i := strings.IndexByte(s, 'e')
	if i < 0 {
		return 0
	}
	n, err := strconv.Atoi(s[i+1:])
	if err != nil {
		return 0
	}
	return n
}

// printfIsExactHalf reports whether the value is exactly halfway between the
// two renderings a rounding at this place chooses from.
//
// Exactly, on the value the operand became and not on a decimal that names
// it: `printf '%.2f' 1.005` is `1.00` in every column because 1.005 is under
// the half once it is a double, and a test that read the digits `1.005` would
// call it a half and part columns that agree.
//
// A rational is what makes that exact. Scaling the value by a power of ten
// and asking whether what is left is a half-integer is one comparison on a
// normalized denominator, and a float64 goes into a rational without loss.
func printfIsExactHalf(f float64, place int) bool {
	if place < -400 || place > 1200 {
		// Past a double's own range in both directions, where no value can
		// be a half at that place and the power of ten is the only thing
		// that would be expensive.
		return false
	}
	v := new(big.Rat).SetFloat64(math.Abs(f))
	if v == nil {
		return false
	}
	n := place
	if n < 0 {
		n = -n
	}
	scale := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil))
	if place >= 0 {
		v.Mul(v, scale)
	} else {
		v.Quo(v, scale)
	}
	return v.Denom().Cmp(big.NewInt(2)) == 0
}
