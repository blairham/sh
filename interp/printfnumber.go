// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/blairham/sh/syntax"
)

// How `printf` reads a numeric operand.
//
// Three questions, and they are separate because the panel moves them
// separately — which is the shape that has been wrong three times in this
// builtin already (#2646, #2648, the `*` status axis):
//
//   - **What the value is.** The whole-operand reading is unanimous and is
//     answered here with no dialect consulted. Everything else is
//     Semantics.PrintfNumberOperand, which is three readings and not two.
//   - **Whether anything is said about it.** Semantics.PrintfReportsBadNumber,
//     which already existed, plus the arithmetic reading's own complaint —
//     which comes out of the evaluator rather than out of this file.
//   - **What it costs the status.** Diagnostics.PrintfBadNumberStatus, and
//     bash 3.2's `warning:` at 0 is an age rather than a language.
//
// A range error is where those three came apart most recently (#2727).
// `strconv.ParseFloat` hands back the infinity it overflowed to *and*
// `ErrRange`, and taking the error while discarding the value wrote
// `0.000000` where six of the seven columns write `inf`. The value and the
// diagnostic are not the same observation: zsh and ksh93 write the infinity
// and say nothing, bash and dash write it and report, and BusyBox ash writes
// it at `%f` and refuses the identical overflow at `%d`.
//
// Underflow is the other direction of the same errno and is measured rather
// than assumed: `printf '%f' 1e-320` is `Result too large` in bash and dash
// too, word for word, with a denormal for a value. One sentence for both
// directions is what says these columns report `ERANGE` and not a reading of
// the operand — so there is one question here and not two. Go does not
// report it: `ParseFloat("1e-400")` is `0, nil`, so the underflow is found
// from the value.

// printfNumber reads an integer operand, complaining where the dialect does.
//
// The zero is printed either way: the shells that report this still write the
// number the conversion would have produced, so the complaint is beside the
// output rather than instead of it.
func (r *Runner) printfNumber(arg string, present bool) (int64, int, bool) {
	if arg == "" && !r.printfEmptyNumberIsAnError(present) {
		return 0, 0, false
	}
	if n, ok := r.charConstant(arg); ok {
		return n, 0, false
	}
	text := afterLeadingBlanks(arg)
	_, radixRefused := r.radixRefusesThePoint(text)
	if v, ok := cAgreedInteger(r.radixParsed(text, false)); ok && !radixRefused {
		// The operand is an integer every reading in the panel agrees about,
		// so no dialect is consulted: `printf '%d' 42` and `printf '%d' 0x10`
		// are the same in all seven columns.
		if rounded := floatToInt64(float64(v)); rounded != v {
			// Except one, and only for the operands where the two readings
			// actually differ — see
			// Semantics.PrintfIntegerOperandGoesThroughTheFloatingType. The
			// test is the disagreement itself rather than a digit count, so
			// `printf '%d' 42` still asks nothing and neither does the
			// int64 maximum, which rounds to 2^63 and saturates back to
			// itself.
			if r.ask(r.sem().PrintfIntegerOperandGoesThroughTheFloatingType,
				"`printf` reading an integer operand too large for a double through the floating type") {
				return rounded, 0, false
			}
			if r.unspecified {
				return 0, r.status, true
			}
		}
		return v, 0, false
	}
	f, code, stop := r.printfPartialNumber(arg, text, false)
	if code == 0 && !stop {
		code = r.printfIntegerOverflow(arg, f, false)
	}
	return floatToInt64(f), code, stop
}

// printfIntegerOverflow reports a value an *integer* conversion cannot hold,
// where the dialect says anything about it.
//
// One column does, and only at the integer conversions: `printf '%d'
// 99999999999999999999` is the clamped value, `printf: warning:
// 99999999999999999999: overflow exception` and 1 in ksh93u+, while
// `printf '%f'` of the same operand is `100000000000000000000.000000` in
// silence at 0 (#2765). So the range is the *conversion's* and not the
// operand's, which is why this is asked here and not beside the ERANGE
// reading printfOutOfRange reports.
//
// Only a finite value is asked about. An operand that overflowed a double
// answers `0` in that column rather than an infinity — its evaluator reads
// `1e400` as zero, which is #2766 and not this — and it says nothing at all
// about it, so a complaint here would be one the reference does not make.
func (r *Runner) printfIntegerOverflow(arg string, f float64, evaluated bool) int {
	w := r.diag().PrintfIntegerOverflow
	if w == "" || !inRangeForOverflowReport(f, evaluated) {
		return 0
	}
	r.diagf("%s\n", Wording(w, "printf: warning: %[1]s: overflow exception", arg))
	return orDefault(r.diag().PrintfBadNumberStatus, 1)
}

// inRangeForOverflowReport reports whether f is a value the complaint above is
// made about, which is a finite number outside what an int64 holds — and,
// where the value came out of the *evaluator*, an infinity as well.
//
// The two are one question and not two, which is what folds this into the
// caller above rather than a second helper beside it: `printf '%d' 1.0/0` is
// `overflow exception` and the clamped maximum at 1 in ksh93u+, and so are
// `1e2/0` and `1e308*10`, while `printf '%d' 1e400` — an infinity a *numeral*
// would have produced — is a silent zero there, that shell's reader answering
// zero for a numeral it cannot hold (#2766). A not-a-number is neither, in
// either direction.
func inRangeForOverflowReport(f float64, evaluated bool) bool {
	if math.IsNaN(f) {
		return false
	}
	if math.IsInf(f, 0) {
		return evaluated
	}
	return f >= math.MaxInt64 || f <= math.MinInt64
}

// printfFloat reads a float operand the same way, with the conversion's own
// reader in place of the integer one.
func (r *Runner) printfFloat(arg string, present bool) (float64, int, bool) {
	if arg == "" && !r.printfEmptyNumberIsAnError(present) {
		return 0, 0, false
	}
	if n, ok := r.charConstant(arg); ok {
		// The same operand a `%d` would read, widened: `printf '%f' "'A"` is
		// `65.000000` in every column.
		return float64(n), 0, false
	}
	text := afterLeadingBlanks(arg)
	_, radixRefused := r.radixRefusesThePoint(text)
	if f, ranged, whole := cWholeNumber(r.radixParsed(text, true), true); whole && !radixRefused {
		// Every reading agrees about a float C can read whole, the reading
		// that evaluates included: it reads a numeral as a numeral before it
		// reads anything as an expression. Only the range error needs a
		// dialect, and the one it needs is PrintfReportsBadNumber, which was
		// already here.
		if ranged {
			return f, r.printfOutOfRange(arg), false
		}
		return f, 0, false
	}
	if f, ok := cNonFinite(text); ok {
		// `inf`, `nan` and the spellings around them, read before any
		// dialect and by every dialect. See printfNonFinite and #2729: this
		// shell does not reproduce ksh93's evaluator here, which answers
		// `-0` for the word its own arithmetic produces.
		return f, 0, false
	}
	return r.printfPartialNumber(arg, text, true)
}

// printfPartialNumber is the operand no conversion in the panel could read on
// its own, and the one place Semantics.PrintfNumberOperand is consulted.
//
// It is reached only after the whole-operand reading above has failed or has
// found a numeral the three readings disagree about, which is what keeps the
// axis off the common path: `printf '%d' 42` and `printf '%f' 1.5` never ask
// a dialect anything.
//
// The value is a float whichever conversion asked for it, because the reading
// that evaluates produces one — `printf '%d' 1e3` is `1000` in the two
// columns that evaluate — and truncating late is what lets `1.5` be `1` and
// `1e3abc` be `1000` under the same rule.
func (r *Runner) printfPartialNumber(arg, text string, float bool) (float64, int, bool) {
	// The operand as the readers below see it, and whether a point in it is a
	// radix at all under the locale in force. `head` is `text` wherever the
	// question does not arise, so every reading below reads exactly as it did
	// before the axis existed. See radixParsed and radixRefusesThePoint.
	parsed := r.radixParsed(text, float)
	head, radixRefused := r.radixRefusesThePoint(text)
	switch r.numberReading() {
	case PrintfNumberWholeOperand:
		v, ranged, whole := cWholeNumber(parsed, float)
		whole = whole && !radixRefused
		switch {
		case !whole:
			return 0, r.printfIncomplete(arg, text), false
		case ranged && !float:
			// The one place the two conversions part company in this column,
			// and it is measured: `printf '%f' 1e400` is `inf` at a complaint
			// in BusyBox ash and `printf '%d' 99999999999999999999` is `0` at
			// the same complaint. C's `strtod` hands back the infinity it
			// overflowed to and sets `errno`; the integer reader hands back
			// nothing.
			return 0, r.printfIncomplete(arg, text), false
		case ranged:
			return v, r.printfOutOfRange(arg), false
		}
		return v, 0, false
	case PrintfNumberLeadingNumber:
		v, ranged, whole := cWholeNumber(parsed, float)
		whole = whole && !radixRefused
		switch {
		case whole && ranged:
			return v, r.printfOutOfRange(arg), false
		case whole:
			return v, 0, false
		}
		return r.leadingNumber(r.radixParsed(head, float), float), r.printfIncomplete(arg, text), false
	case PrintfNumberArithmetic:
		if v, _, whole := cWholeNumber(parsed, true); whole && !radixRefused {
			// A numeral is read as a numeral, which is why `printf '%d' 010`
			// is 10 in ksh93 where `echo $((010))` there is 8. The reading is
			// C's `strtod` and not the conversion's, so `1e3` is 1000 at `%d`
			// as well.
			return v, 0, false
		}
		n, err, reading := arithNum{}, error(nil), false
		if radixRefused {
			// A point where this locale's radix is something else, which this
			// shell's arithmetic cannot read as a number at all. The complaint
			// is the one it already writes for an operand that is a number and
			// then something else, so the error is built rather than a second
			// sentence written — see radixRefusesThePoint.
			se := &syntax.Error{Kind: syntax.ErrArithOperator, Expr: text, Token: cRadixChar}
			err, reading = arithError{msg: r.expressionFailure(text, se), complete: true}, true
		} else {
			// The **expression** reading gets the operand translated only
			// where the locale's radix displaces the comma operator, which is
			// measured and is the row an operand holding both characters
			// separates. Under `LC_NUMERIC=de_DE.UTF-8`, `printf '%.2f' 1,5.5`
			// is `5,50` in zsh — the comma is still the operator there, and the
			// numeral reading above has already had its chance at the whole
			// word — and `arithmetic syntax error` with `1,50` in ksh93, where
			// the comma has stopped being an operator at all. The same
			// distinction the two policies already carry, so no third value is
			// needed for it.
			expr := parsed
			if r.sem().NumberRadix.readsThePointAsWell() {
				expr = text
			}
			n, err, reading = r.printfArithValue(expr, text)
		}
		if err == nil {
			f := n.asFloat()
			if !float && math.IsInf(f, 0) {
				// An infinity the *evaluator* produced, which is a value the
				// integer conversion cannot hold and which one column
				// reports: `printf '%d' 1.0/0` is `overflow exception` and
				// the clamped maximum at 1 in ksh93u+, and so are `1e2/0`
				// and `1e308*10`. An infinity a *numeral* produced is not
				// the same row and not reported — `1e400` is a silent 0
				// there, which is #2766 — so the question is asked here,
				// where the value is known to have been computed, rather
				// than of every value the reading returns.
				return f, r.printfIntegerOverflow(arg, f, true), false
			}
			return f, 0, false
		}
		code := r.printfArithFailure(err, reading, float)
		if ae, ok := err.(arithError); ok && ae.keptTheValue {
			// The evaluation ran to the end past a division by zero and the
			// number it came to is the operand's — see
			// Semantics.ArithDivisionByZeroYieldsAValue. The complaint has
			// already gone out; only the value is decided here, and it is
			// not the leading-number reading below.
			return n.asFloat(), code, false
		}
		if r.unspecified {
			// The count is an axis of its own and it is asked before the
			// sentence goes out, so a dialect that has not answered it stops
			// here rather than writing a line it cannot know the number of.
			return 0, r.status, true
		}
		if !r.ask(r.sem().PrintfRefusedOperandKeepsItsLeadingNumber,
			"`printf` keeping the number at the front of an operand its arithmetic would not evaluate") {
			if r.unspecified {
				return 0, r.status, true
			}
			return 0, code, false
		}
		// Always the float read, whichever conversion asked: the column that
		// keeps it answers `1000` for `printf '%d' 1e3abc`, which `strtoimax`
		// could not have produced.
		return r.leadingNumber(r.radixParsed(head, true), true), code, false
	}
	return 0, r.status, true
}

// printfArithValue evaluates an operand as an expression, which is the second
// half of the arithmetic reading — the first is the numeral above.
//
// Not arithValueAsExpression, which is the same two steps for a value a
// *variable* was holding and differs in both of the ways that matter here: it
// evaluates one frame down, so an unset name in it meets
// Semantics.ArithRecursedNameMustBeSet, and it asks
// Semantics.ArithNameValueRecurses for permission to re-read at all. An
// operand written on the command line is neither. `printf '%d' abc` is 0 and
// silent in zsh and ksh93 alike — the same answer `echo $((abc))` gives
// there — where the stored-value path would call it a parameter that is not
// set.
//
// The third result says whether what failed was the *reading of the operand
// as a number* rather than anything about the expression around it, which is
// the one column that parts them: see Diagnostics.PrintfArithArgumentType.
// Two texts rather than one: `parsed` is what the evaluator reads and `written`
// is what a complaint names. They differ only under a locale whose radix
// character is not the point — see radixParsed — and a first attempt at #4230
// passed the rewritten text to both, which put `printf: 1\x015: arithmetic
// syntax error` on stderr, a sentence naming a word no script wrote.
func (r *Runner) printfArithValue(parsed, written string) (arithNum, error, bool) {
	text := parsed
	if text == "" {
		// No expression at all, which every evaluating column reads as zero
		// and says nothing about: `printf '%d' ' '` is `[0]` at 0 in both.
		return intNum(0), nil, false
	}
	p := syntax.NewParser("", r.dialect())
	tree := p.ParseArithFor(text, syntax.Pos{})
	err := p.Err()
	if err == nil && tree == nil {
		// No tree and no complaint is text holding an expansion the parser
		// leaves for its caller. An operand has already been expanded once
		// and does not go through it again, so a `$` left here is an operand
		// failure — the same reading arithValueAsExpression takes of it.
		err = &syntax.Error{Kind: syntax.ErrArithOperand, Expr: written, Token: written}
	}
	if err != nil {
		// Text left over after a complete expression is a reading failure:
		// the operand was a number and then something else. Wanting an
		// operand, or a parenthesis that never closed, is not.
		var se *syntax.Error
		left := errors.As(err, &se) && se.Kind == syntax.ErrArithOperator
		return intNum(0), arithError{msg: r.expressionFailure(written, err), complete: true}, left
	}
	outer, held := r.arithValueSurvivesTheDivision, r.arithDivisionFailure
	r.arithValueSurvivesTheDivision, r.arithDivisionFailure = true, nil
	n, err := r.evalNum(tree)
	kept := r.arithDivisionFailure
	r.arithValueSurvivesTheDivision, r.arithDivisionFailure = outer, held
	if err == nil && kept != nil {
		// The evaluation carried on past a division by zero and reached the
		// end. The complaint is the one that was held, and the value beside
		// it is the whole expression's.
		ae, _ := kept.(arithError)
		return n, arithError{
			msg: r.arithFailure(text, kept), complete: true, keptTheValue: true,
			badNumeral: ae.badNumeral,
		}, ae.badNumeral
	}
	if err != nil {
		ae, _ := err.(arithError)
		return intNum(0), arithError{msg: r.arithFailure(text, err), complete: true}, ae.badNumeral
	}
	return n, nil, false
}

// leadingNumber is the number at the front of an operand, or zero where there
// is none. The reader is the conversion's own, which is the whole of why
// `printf '%d' 1e3` is `1` and `printf '%f' 1e3abc` is `1000`.
func (r *Runner) leadingNumber(text string, float bool) float64 {
	if float {
		if n := cFloatRun(text); n > 0 {
			f, _ := cFloat(text[:n])
			return f
		}
		return 0
	}
	if n := cIntegerRun(text); n > 0 {
		v, _ := cInteger(text[:n])
		return float64(v)
	}
	return 0
}

// afterLeadingBlanks is the operand with the blanks C's readers skip taken
// off the front, and **nothing taken off the back**.
//
// The two ends are not the same question and trimming both got one of them
// wrong for every dialect at once. C's `strtol` and `strtod` skip leading
// whitespace as part of the grammar, so ` 7` is a whole number everywhere.
// What is left after the number is the caller's problem, and four of the
// seven columns say so: `printf '%d' "7 "` is `printf: 7 : invalid number`
// at 1 in bash 5.3, `not completely converted` at 1 in dash, `invalid number
// '7 '` at 1 in BusyBox ash — where the value is `0` rather than the 7 the
// other two write — and silent at 0 in zsh and ksh93, whose reading is an
// expression and takes a trailing blank the way any expression does.
// Measured 2026-09-15 on the `-c`, file and standard-input routes (#2905).
//
// The set is C's, not Go's: `strings.TrimSpace` also trims the Unicode
// spaces, and no `strtol` in the panel does.
func afterLeadingBlanks(s string) string {
	i := 0
	for i < len(s) && isCBlank(s[i]) {
		i++
	}
	return s[i:]
}

// isCBlank is C's `isspace` in the C locale, which is what a numeric reader
// skips in front of a number.
func isCBlank(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}

// printfIncomplete reports an operand that was not a number, or not all of
// one, where the dialect reports it at all.
func (r *Runner) printfIncomplete(arg, text string) int {
	if !r.ask(r.sem().PrintfReportsBadNumber, "`printf` complaining about an operand that is not a number") {
		return 0
	}
	d := r.diag()
	if d.PrintfBadNumberEchoesPastTheBlanks {
		// One column quotes the operand back from its first non-blank byte:
		// `printf '%d' "  7  "` is `invalid number '7  '` in BusyBox ash,
		// where bash and dash echo the leading blanks they were given.
		// Measured in the pinned image, BusyBox v1.37.0, 2026-09-15.
		arg = text
	}
	if w := printfBadNumberBase(*d, arg); w != "" {
		// One column names the base the operand was *spelled* in — `invalid
		// hex number` for `0x10zz`, `invalid octal number` for `08` — and
		// asks it of the operand rather than of the conversion or of the
		// reading that failed. See Diagnostics.PrintfBadHexNumber.
		r.diagf("%s\n", Wording(w, "printf: %[1]s: invalid number", arg))
		return orDefault(d.PrintfBadNumberStatus, 1)
	}
	if w := d.PrintfIncompleteNumber; w != "" && cIntegerRun(text) > 0 {
		// The one column with two sentences keeps them apart by whether
		// anything was read at all: dash says `not completely converted` for
		// `1.5` and `expected numeric value` for `abc`.
		r.diagf("%s\n", Wording(w, "printf: %[1]s: invalid number", arg))
		return orDefault(d.PrintfBadNumberStatus, 1)
	}
	return r.printfReport(printfBadNumber, arg)
}

// printfBadNumberBase is the bad-number sentence a dialect keeps for an
// operand written with a radix prefix, or empty where it has none.
//
// The question is the operand's spelling and not the value: `0x` in lower
// case at the very front is hexadecimal, a `0` followed by a decimal digit is
// octal, and everything else — a sign, a leading blank, `0X`, `0b11`, `0z`,
// `0.5` — is neither. All of that is measured; see
// Diagnostics.PrintfBadHexNumber for the rows.
//
// It is asked only where the *general* sentence would have gone out. An
// operand C read as a number and could not hold is a range error and keeps
// its own sentence: `printf '%d' 0xFFFFFFFFFFFFFFFFFF` is `Result too large`
// in bash and not `invalid hex number`.
func printfBadNumberBase(d Diagnostics, arg string) string {
	switch {
	case strings.HasPrefix(arg, "0x"):
		return d.PrintfBadHexNumber
	case len(arg) > 1 && arg[0] == '0' && arg[1] >= '0' && arg[1] <= '9':
		return d.PrintfBadOctalNumber
	}
	return ""
}

// printfOutOfRange reports an operand that is a number C cannot hold, in
// either direction. The value has already been taken: what this decides is
// only whether anything is said about it and what that costs.
func (r *Runner) printfOutOfRange(arg string) int {
	if !r.ask(r.sem().PrintfReportsBadNumber, "`printf` complaining about an operand that is not a number") {
		return 0
	}
	d := r.diag()
	w := d.PrintfNumberOutOfRange
	if w == "" {
		return r.printfReport(printfBadNumber, arg)
	}
	r.diagf("%s\n", Wording(w, "printf: %[1]s: Result too large", arg))
	return orDefault(d.PrintfBadNumberStatus, 1)
}

// printfArithFailure writes the sentence the same expression would earn in
// `$(( ))`, which is what the two evaluating columns write here.
//
// reading says the failure was the operand's own reading rather than the
// expression's, which is what one of the two columns parts on: see
// Diagnostics.PrintfArithArgumentType.
//
// float says the conversion that asked for the number was a floating one,
// which is what decides how many times the sentence goes out: see
// Semantics.PrintfFloatOperandIsEvaluatedTwice.
func (r *Runner) printfArithFailure(err error, reading, float bool) int {
	msg := err.Error()
	if w := r.diag().PrintfArithOperandFailure; w != "" {
		msg = Wording(w, "%[1]s", msg)
	}
	lines := 1
	if float && r.ask(r.sem().PrintfFloatOperandIsEvaluatedTwice,
		"a floating `printf` conversion evaluating an operand its arithmetic would not take") {
		lines = 2
	}
	if r.unspecified {
		return r.status
	}
	for range lines {
		// arithDiagf rather than diagf: this is the evaluator's sentence and
		// not the builtin's, and one column says so by leaving the builtin
		// out of the location it otherwise always writes. See #2906.
		r.arithDiagf("%s\n", msg)
	}
	if w := r.diag().PrintfArithArgumentType; w != "" {
		if !reading {
			// The complaint still goes out; the second line and the status
			// do not. `printf '%d' 1/0` is `divide by zero` at 0 in ksh93,
			// against `42abc`'s two lines at 1.
			return 0
		}
		r.diagf("%s\n", Wording(w, "printf: warning: invalid argument of type %[1]s", r.printfConversionName))
	}
	return orDefault(r.diag().PrintfBadNumberStatus, 1)
}

// floatToInt64 is the integer a float conversion produces, clamped rather than
// left to Go — converting an out-of-range float is undefined there, and an
// infinity is exactly the value that reaches this after a range error.
func floatToInt64(f float64) int64 {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= math.MaxInt64:
		return math.MaxInt64
	case f <= math.MinInt64:
		return math.MinInt64
	}
	return int64(f)
}

// cWholeNumber reads the whole operand as C's reader for the conversion, and
// reports the value, whether C would have set ERANGE for it, and whether the
// reader consumed all of it.
func cWholeNumber(text string, float bool) (value float64, ranged, whole bool) {
	if float {
		n := cFloatRun(text)
		if n == 0 || n != len(text) {
			return 0, false, false
		}
		f, over := cFloat(text)
		return f, over, true
	}
	n := cIntegerRun(text)
	if n == 0 || n != len(text) {
		return 0, false, false
	}
	v, over := cInteger(text)
	return float64(v), over, true
}

// cAgreedInteger is the integer operand no reading in the panel disagrees
// about, which is what lets the common case skip the axis entirely.
//
// Two shapes are held back. A numeral C reads as octal and arithmetic reads as
// decimal is the first — `010` is 8 in bash, dash and BusyBox ash and 10 in
// zsh and ksh93 — and it is decided by comparing the two readings rather than
// by the leading zero, because `007` and `00` are the same number under both
// and must not be questioned. A numeral too large for the type is the second:
// the columns split on whether the saturated value survives.
func cAgreedInteger(text string) (int64, bool) {
	n := cIntegerRun(text)
	if n == 0 || n != len(text) {
		return 0, false
	}
	v, over := cInteger(text)
	if over {
		return 0, false
	}
	if f, ranged := cFloat(text); !ranged && f == float64(v) {
		return v, true
	}
	return 0, false
}

// cNonFinite reads the infinities and the not-a-numbers C's `strtod` takes,
// which are the operands no *run* covers: no column reads a prefix of one.
func cNonFinite(text string) (float64, bool) {
	if f, err := strconv.ParseFloat(text, 64); err == nil && (math.IsInf(f, 0) || math.IsNaN(f)) {
		return f, true
	}
	return cNotANumber(text)
}

// cIntegerRun is how many bytes of s C's `strtoimax` with base 0 would take.
//
// Written out rather than handed to `strconv.ParseInt`, which is a different
// grammar in both directions: Go takes `0b11` and `1_0` where C stops at the
// `b` and the `_`, and Go has no notion of stopping part way at all.
func cIntegerRun(s string) int {
	i := 0
	for i < len(s) && (s[i] == '+' || s[i] == '-') {
		if i > 0 {
			return 0
		}
		i++
	}
	digits := i
	if i+1 < len(s) && s[i] == '0' && (s[i+1] == 'x' || s[i+1] == 'X') {
		j := i + 2
		for j < len(s) && isHexDigit(s[j]) {
			j++
		}
		if j > i+2 {
			return j
		}
		// `0x` with no hex digit after it is the `0` and nothing more, which
		// is what C does with it.
		return i + 1
	}
	if i < len(s) && s[i] == '0' {
		j := i + 1
		for j < len(s) && s[j] >= '0' && s[j] <= '7' {
			j++
		}
		return j
	}
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == digits {
		return 0
	}
	return i
}

// cInteger reads a run cIntegerRun measured, saturating the way `strtoimax`
// does rather than discarding the value with the error (#2727).
func cInteger(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 0, 64)
	if err == nil {
		return n, false
	}
	if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
		return n, true
	}
	return 0, false
}

// cFloatRun is how many bytes of s C's `strtod` would take, hex forms
// included. The non-finite words are not here: they are read whole by
// cNotANumber and by ParseFloat, and no column takes a *prefix* of one.
func cFloatRun(s string) int {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	if i+1 < len(s) && s[i] == '0' && (s[i+1] == 'x' || s[i+1] == 'X') {
		j := i + 2
		start := j
		for j < len(s) && isHexDigit(s[j]) {
			j++
		}
		if j < len(s) && s[j] == '.' {
			j++
			for j < len(s) && isHexDigit(s[j]) {
				j++
			}
		}
		if j == start || (j == start+1 && s[start] == '.') {
			return 0
		}
		return j + exponentRun(s[j:], 'p')
	}
	j := i
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	whole := j - i
	if j < len(s) && s[j] == '.' {
		j++
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
	}
	if j-i == 0 || (whole == 0 && j-i == 1) {
		return 0
	}
	return j + exponentRun(s[j:], 'e')
}

// exponentRun is the length of an exponent at the front of s, or zero. The
// letter is `e` for a decimal float and `p` for a hexadecimal one, and an
// exponent that is started and not finished is no exponent at all — C leaves
// the letter behind, so `1e` reads as `1`.
func exponentRun(s string, letter byte) int {
	if len(s) == 0 || (s[0]|0x20) != letter {
		return 0
	}
	i := 1
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	j := i
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if j == i {
		return 0
	}
	return j
}

// cFloat reads a run cFloatRun measured, and reports whether C would have set
// ERANGE for it.
//
// Overflow is Go's own `ErrRange`, which carries the infinity with it.
// Underflow is not: `ParseFloat("1e-400")` is `0, nil` and
// `ParseFloat("1e-320")` is a denormal with no error, where bash and dash
// report both. So it is found from the value — a result that is zero or
// denormal where the digits were not all zeros is the underflow C reports.
func cFloat(s string) (float64, bool) {
	text := s
	if len(text) > 1 && (text[0] == '+' || text[0] == '-') {
		text = text[1:]
	}
	if len(text) > 1 && text[0] == '0' && (text[1] == 'x' || text[1] == 'X') && exponentRun(hexTail(text), 'p') == 0 {
		// Go wants the binary exponent C makes optional.
		s += "p0"
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			return f, true
		}
		return 0, false
	}
	if math.Abs(f) < 2.2250738585072014e-308 && hasNonZeroDigit(s) {
		return f, true
	}
	return f, false
}

// hexTail is what is left of a hexadecimal float after its digits, which is
// where its exponent would be.
func hexTail(s string) string {
	i := 2
	for i < len(s) && (isHexDigit(s[i]) || s[i] == '.') {
		i++
	}
	return s[i:]
}

// hasNonZeroDigit says whether the significand held anything but zeros, which
// is what parts an underflow from a written zero: `0.0` is not a range error
// anywhere and `1e-400` is one in three columns.
func hasNonZeroDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 'e' || c == 'E' || c == 'p' || c == 'P' {
			return false
		}
		if (c >= '1' && c <= '9') || (isHexDigit(c) && c != '0') {
			return true
		}
	}
	return false
}

// radixParsed is the operand as this shell's number *readers* see it under a
// locale whose radix is not the point: the locale's radix put back as a point,
// which is the only character every reader here knows.
//
// Safe in both directions, because the locale's radix is never a character a
// *numeral* could have held otherwise — `1,5` can only have meant `1.5`. So this
// is a translation and not a guess. What a *point* means under such a locale is
// the other half of the question and is radixRefusesThePoint's; where the
// operand is an expression rather than a numeral the comma may still be the
// comma operator, which is what the `float` argument is about.
//
// Applied at each reader and **never to the operand a complaint names**, which
// is the half a first attempt at #4230 got wrong: rewriting the text once at the
// top put `printf: 1\x015: arithmetic syntax error` on stderr, a sentence naming
// a word no script wrote.
func (r *Runner) radixParsed(text string, float bool) string {
	radix, moved := r.localeHasItsOwnRadix()
	if !moved {
		return text
	}
	if !float && r.sem().NumberRadix.readsThePointAsWell() {
		// The lenient policy reads the locale's radix in a **floating**
		// conversion's numeral and nowhere else. Measured on zsh 5.9.2 under
		// `LC_NUMERIC=de_DE.UTF-8`: `printf '%.2f' 1,5` is `1,50`, the comma
		// read as a radix, and `printf '%d' 1,5` is `5`, the same comma read as
		// the operator its arithmetic has always had. The stricter policy does
		// not split this way — there the radix displaces the operator for every
		// conversion — which is why the test is the policy and not the verb.
		return text
	}
	return strings.ReplaceAll(text, radix, cRadixChar)
}

// radixRefusesThePoint reports whether a point in this operand is not a radix at
// all, and answers the part of the operand in front of it.
//
// The stricter of the two locale-reading policies: bash and ksh93 take the
// locale's radix *instead of* the point, so `printf '%.2f' 1.5` under a comma
// locale is not a number with a fractional part — it is a number and then
// something else. Which is exactly the shape each of them already refuses, and
// is why this answers a head rather than a boolean: the value they write is the
// leading number. Measured on ksh93u+ under `LC_NUMERIC=de_DE.UTF-8`, four
// operands drawing one triple —
//
//	1.5    the syntax error twice, `invalid argument of type f`, then 1,00
//	.5     the same, then 0,00
//	1.     the same, then 1,00
//	1.5x   the same, then 1,00
//
// — the last of which reaches that path with no locale involved at all, which is
// what says the refusal is the dialect's own. bash's shape is its own and the
// same shape: `printf: 1.5: invalid number` and 1,00.
//
// Expressed as a refusal rather than by rewriting the point into something no
// reader takes, which was tried first and is where the sentence naming a word
// nobody wrote came from: a rewrite has to be undone for every complaint, and it
// cannot be undone at all under the lenient policy, where a point and the
// locale's radix both become a point.
func (r *Runner) radixRefusesThePoint(text string) (head string, refused bool) {
	if _, moved := r.localeHasItsOwnRadix(); !moved {
		return text, false
	}
	if r.sem().NumberRadix.readsThePointAsWell() {
		return text, false
	}
	before, _, found := strings.Cut(text, cRadixChar)
	if !found {
		return text, false
	}
	return before, true
}
