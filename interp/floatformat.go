// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strconv"

// `typeset -E n`, where the letter is a **format** and not a width.
//
// Its sibling `-F n` is a number of decimal places and means the same in both
// shells that spell it. `-E n` does not: the two read the number differently
// *and* render differently, which is why it did not travel with the width
// letters in #2538 and why this is an axis rather than a number.
//
// Measured 2026-09-15, zsh 5.9.2 and ksh93u+ 2012-08-01, `env -i` with a
// scratch HOME and no startup files, from a script file:
//
//	typeset -E 3 a=3.14159      zsh  3.14e+00        ksh  3.14
//	typeset -E 3 b=0.000123456  zsh  1.23e-04        ksh  0.000123
//	typeset -E 3 f=100          zsh  1.00e+02        ksh  100
//	typeset -E 1 g=3.14159      zsh  3e+00           ksh  3
//	typeset -E c=1.5            zsh  1.500000000e+00 ksh  1.5
//	typeset -E 3 h=abc          zsh  0.00e+00        ksh  0
//
// which is `%.*e` with **n−1** places in zsh and `%.*g` with **n** in ksh93,
// the default n being 10 in both. Neither rendering is a special case of the
// other — `100` at n=3 is `1.00e+02` there and `100` here — so a shell that
// picked one would be writing numbers the other never writes, at status 0,
// out of a script that told it the format.
//
// The listing splits the same way, and is answered by the fields that already
// exist rather than here: zsh writes no width for `-E`, exactly as it writes
// none for `-F`, and ksh93 writes it as a word of its own — `typeset -E 3
// a=3.14`.

// FloatFormatPolicy is what the `E` letter of a declaration renders a float
// with, in the dialect that spells it.
//
// The unanswered value is refused by name rather than given one shell's
// reading, the way every unanswered axis is: the two renderings agree on no
// value a script is likely to write.
type FloatFormatPolicy int

const (
	// FloatFormatUnspecified is no answer, which bash and dash hold and
	// never reach: neither spells the letter.
	FloatFormatUnspecified FloatFormatPolicy = iota
	// FloatFormatExponentWithPlaces is zsh's: `%.*e` with n−1 places after
	// the point, so the number is the *total* significant figures and the
	// exponent is always written.
	FloatFormatExponentWithPlaces
	// FloatFormatSignificantDigits is ksh93's: `%.*g` with n significant
	// digits, so a number that fits without an exponent is written without
	// one.
	FloatFormatSignificantDigits
)

func (p FloatFormatPolicy) String() string {
	switch p {
	case FloatFormatExponentWithPlaces:
		return "an exponent with n-1 places"
	case FloatFormatSignificantDigits:
		return "n significant digits"
	}
	return "unspecified"
}

// floatFormatted is the text a float name's value is stored and read as.
//
// One function for both letters, because the choice between them is a
// property of the *name* — which letter declared it — and every caller that
// renders a float has to make the same choice. Three call sites had a bare
// strconv.FormatFloat with 'f' in it before this existed, which is three
// places that would each have had to learn about the second letter.
func (r *Runner) floatFormatted(name string, v float64) (string, bool) {
	prec := r.floatPrecision[name]
	if !r.floatIsExponent(name) {
		return strconv.FormatFloat(v, 'f', floatPlaces(prec), 64), true
	}
	n := floatPlaces(prec)
	switch r.sem().FloatFormatLetterE {
	case FloatFormatSignificantDigits:
		return strconv.FormatFloat(v, 'g', n, 64), true
	case FloatFormatExponentWithPlaces:
		// n−1, and the subtraction is the whole of the difference between
		// the two readings: `-E 1` is `3e+00` and not `3.0e+00`, so the
		// number is significant figures and `%.*e` counts places after the
		// point.
		return strconv.FormatFloat(v, 'e', n-1, 64), true
	}
	// A dialect that spells the letter and has not said which rendering it
	// means. Reached only where the letter was really written, so a preset
	// that never meets it is never asked.
	r.errf("%s\n", r.diag().Report(r.name(), r.line,
		r.unanswered("the `E` letter of a declaration")))
	r.status, r.unspecified = 2, true
	return "", false
}

// floatIsExponent reports whether the float attribute on a name came from the
// `E` letter rather than from `F`.
//
// A table of its own beside floatPrecision rather than a second value in it,
// for the reason fieldWidth keeps its letter: the precision is a number a
// script may write as zero, and which letter wrote it is a separate fact that
// a shadowed binding has to carry and put back.
func (r *Runner) floatIsExponent(name string) bool { return r.floatExponent[name] }

// markFloatExponent records the letter, and clearFloatExponent takes it off.
func (r *Runner) markFloatExponent(name string, on bool) {
	if !on {
		delete(r.floatExponent, name)
		return
	}
	if r.floatExponent == nil {
		r.floatExponent = map[string]bool{}
	}
	r.floatExponent[name] = true
}
