// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"math"
	"strconv"
)

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
	if text, named := r.floatIsNamedRatherThanWritten(v); named {
		// An infinity and a NaN are *named* wherever they are written, in a
		// float name exactly as in an expression, so neither letter and
		// neither precision reaches them. Measured 2026-09-26: `float g;
		// (( g = 1.0/0.0 ))` leaves `$g` at the three characters `Inf` in
		// zsh 5.9.2 and `inf` in ksh93u+ — which `${#g}` counts and which an
		// exported child is told — where we wrote a formatted zero (#4662).
		//
		// Ahead of the `E` letter's axis, and that is asked at the
		// disagreement rather than skipped: the two readings of that letter
		// are two number formats, and neither of them writes a number here,
		// so a dialect that has not answered it is not being asked
		// something the answer would change.
		return text, true
	}
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

// floatIsNamedRatherThanWritten is what a value that is not a number is
// *called* in this dialect, and false for one that is.
//
// One function for the expression and for the store, because they are one
// question: `$(( 1.0/0.0 ))` and a float name holding that value write the
// same three characters, and a second copy of the table beside this one is
// how `Inf` and `inf` come to be right in one place and wrong in the other.
// Measured 2026-09-26 on zsh 5.9.2 — `Inf`, `-Inf`, `NaN` — and ksh93u+
// 2012-08-01 — `inf`, `-inf`, `nan`.
func (r *Runner) floatIsNamedRatherThanWritten(v float64) (string, bool) {
	switch {
	case math.IsInf(v, 1):
		return Wording(r.diag().ArithInfinity, "+Inf"), true
	case math.IsInf(v, -1):
		return "-" + Wording(r.diag().ArithInfinity, "Inf"), true
	case math.IsNaN(v):
		return Wording(r.diag().ArithNotANumber, "NaN"), true
	}
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

// NumericTypeLetterPrecedencePolicy is how a declaration carrying more than
// one of `E`, `F` and `i` decides which of them the name ends up with.
//
// Two readings, and neither is a relaxation of the other: one is about where
// the letters were *written* and the other about what they *are*, so the same
// line comes out a different kind under each.
type NumericTypeLetterPrecedencePolicy int

const (
	// NumericLetterPrecedenceUnspecified is no answer, which the three
	// shells with no numeric letter beyond `i` hold and never reach: a
	// declaration cannot write two of them there.
	NumericLetterPrecedenceUnspecified NumericTypeLetterPrecedencePolicy = iota
	// NumericLetterFirstWrittenWins is zsh's, and it is the reading the
	// parse falls into on its own: the letter read first sets the attribute
	// and the ones behind it find it already set. Measured 2026-09-14 on zsh
	// 5.9.2 — `typeset -iF 3 a=1.5` lists `typeset -i3 a=1`, `-Fi 3` lists
	// `typeset -F a=1.500`, `-EF 3` lists `typeset -E a=1.50e+00` and
	// `-FE 3` lists `typeset -F a=1.500`. Four lines, four answers, and the
	// order is the whole of what decides.
	NumericLetterFirstWrittenWins
	// NumericLetterFloatOutranksTheInteger is ksh93's: the letters have a
	// fixed rank — `E` over `F` over `i` — and the order they were written
	// in decides nothing. Measured in the same run, each read back with
	// `typeset -p`:
	//
	//	typeset -iF 3 a=1.5     typeset -F 3 a=1.500
	//	typeset -Fi 3 a=1.5     typeset -F 3 a=1.500
	//	typeset -i -F 3 a=1.5   typeset -F 3 a=1.500
	//	typeset -EF 3 a=1.5     typeset -E 3 a=1.5
	//	typeset -FE 3 a=1.5     typeset -E 3 a=1.5
	//
	// The third row is what says the rank holds across *words* as well as
	// inside one, which the order reading cannot do: there both letters are
	// really read, and the integer one still loses.
	//
	// `-iE` is not among them because that pair is refused in this column
	// before the rank is reached — see NumericTypeLettersAreExclusive, which
	// is the same shell's answer to the one combination it will not take.
	NumericLetterFloatOutranksTheInteger
)

func (p NumericTypeLetterPrecedencePolicy) String() string {
	switch p {
	case NumericLetterFirstWrittenWins:
		return "the first letter written"
	case NumericLetterFloatOutranksTheInteger:
		return "E over F over i, whatever the order"
	}
	return "unspecified"
}

// storedFloat is the number a float name is **holding**, beside the text that
// number renders as.
//
// `-E` and `-F` decide how a float is *written* and not what it is. Measured
// 2026-09-25 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f`, through `-c`:
//
//	typeset -E3 f=3.14159265358979   print $f        3.14e+00
//	                                 print $(( f ))  3.14159265358979
//	                                 print ${#f}     8
//	typeset -F1 f=3.14159265358979   (( f = f * 2 ))
//	                                 print $f        6.3
//	                                 print $(( f ))  6.28318530717958
//	typeset -F3 f=3.14159265358979   typeset -p f    typeset -F f=3.142
//	                                 typeset -E5 f   typeset -E f=3.1416e+00
//	                                 typeset -F f    typeset -F f=3.14159
//
// The last block is what this is for: three renderings of one number, and the
// third could not be written from the second's characters. A `-E3` name that
// holds `3.14` is a number a later `-F` can only write as `3.140`, where zsh
// writes `3.142`, so the digits past the format are not the format's to throw
// away.
//
// Everything that is not arithmetic still reads the **text**: `${#f}` is the
// eight characters of `3.14e+00`, a child is told `3.14e+00`, `typeset -p`
// lists the rendering, and `f[2]=9` replaces the whole of it. So the rendering
// stays what is stored, exactly as it was, and the number is remembered beside
// it rather than in place of it — which is also why this is a record of a
// rendering and not a second value anyone may write.
type storedFloat struct {
	// text is the rendering the number was stored under, and is what makes
	// this a record rather than a guess. A store by any route that does not
	// come through attributeFolded leaves the two disagreeing, and a
	// disagreement reads the characters — which is what every float read did
	// before this existed, so the worst a stale record can do is nothing.
	text string
	// value is the number those characters were written from.
	value float64
	// exponent is which float letter made the rendering — `E` rather than
	// `F`. It is here and not read off the name's table at the far end
	// because the question it answers is about the *past*: a declaration
	// that changes the letter has already written the new one down by the
	// time the standing value is re-read, so the only copy of the old one
	// left is the one that traveled with the rendering. See
	// Semantics.FloatLetterChangeRereadsTheRendering.
	exponent bool
}

// floatWrittenInFull is a number written so that reading the characters back
// gives the same number — the shortest spelling that round-trips, which is
// what a value on its way *to* a rendering has to be written as so that the
// rendering is made from the number and not from a shorter copy of it.
//
// Beside the renderings rather than at its one call site, because it is the
// same question they answer with a precision: how a float becomes characters.
func floatWrittenInFull(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// rememberStoredFloat records the number a rendering was made from, and the
// letter it was made with.
func (r *Runner) rememberStoredFloat(name, text string, v float64) {
	if r.floatExact == nil {
		r.floatExact = map[string]storedFloat{}
	}
	r.floatExact[name] = storedFloat{
		text: text, value: v, exponent: r.floatIsExponent(name),
	}
}

// storedFloatValue is the number a float name is holding, and false where the
// characters it is holding are all there is.
//
// Guarded on the attribute as well as on the text, because a name that has
// lost the attribute is holding characters and not a number: measured in the
// same run, `typeset -F1 f=3.14159265358979; typeset +F f; typeset -F1 f` then
// reads `3.1000000000000001` out of `$(( f ))`, so the plus form is where the
// number is really destroyed. forgetStoredFloat is where that is said, and it
// is said once — on the way *in*, where a name that did not already carry the
// attribute is one whose number can only come from its characters.
func (r *Runner) storedFloatValue(name, text string) (float64, bool) {
	c, ok := r.storedFloatRecord(name, text)
	return c.value, ok
}

// storedFloatRecord is the whole record behind a float name's characters,
// which the letter question needs as well as the number: see
// Semantics.FloatLetterChangeRereadsTheRendering.
func (r *Runner) storedFloatRecord(name, text string) (storedFloat, bool) {
	if _, ok := r.floatPrecision[name]; !ok {
		return storedFloat{}, false
	}
	c, ok := r.floatExact[name]
	if !ok || c.text != text {
		return storedFloat{}, false
	}
	return c, true
}

// forgetStoredFloat drops the number a name was holding.
//
// One caller, deliberately: the float attribute *arriving* at a name that did
// not have it. Every route that takes the attribute off — `+F`, `-i` over it,
// a case letter, `unset`, a shadow — has to end with the number gone, and
// writing that at each of them is five chances to add a sixth route and
// forget. Arriving is the one event they all have to pass through before the
// number can be read again, so it is the one place that has to say it.
func (r *Runner) forgetStoredFloat(name string) { delete(r.floatExact, name) }

// arrivingFloat is a number on its way into a float name through the ordinary
// store, beside the characters it is being handed as.
//
// It exists because the store is a **text** door: an arithmetic assignment
// renders its value and setVar reads the characters back through the name's
// attribute, which is right for every number that has a numeral and wrong for
// the two that do not. `Inf` is a literal this arithmetic reads in one
// dialect and nothing at all in the other, so the round trip is where an
// infinity was lost and a formatted zero came back (#4662).
//
// Carried for one call rather than recorded against the name, because what it
// says is about the characters being *given* and not the ones being *held* —
// and those two are the same string often enough for a record to be read by
// the wrong assignment. `typeset -F1 f=3.14159265358979; f=3.1` is the case:
// the name is holding the three characters `3.1` rendered from every digit of
// pi, and the plain assignment of those same characters means three tenths.
type arrivingFloat struct {
	name  string
	text  string
	value float64
}

// floatArrivesAsANumber says that the next store of text into name is a
// rendering of v, and returns the undo.
//
// The caller runs the store and then the undo, in that order and always: a
// slot left standing would be read by whatever assigned those characters
// next, which is the confusion the type comment is about.
func (r *Runner) floatArrivesAsANumber(name, text string, v float64) func() {
	r.arrivingFloat = &arrivingFloat{name: name, text: text, value: v}
	return func() { r.arrivingFloat = nil }
}

// arrivingFloatValue is the number behind the characters a float name is
// being handed, where the store was told one.
func (r *Runner) arrivingFloatValue(name, text string) (float64, bool) {
	a := r.arrivingFloat
	if a == nil || a.name != name || a.text != text {
		return 0, false
	}
	return a.value, true
}
