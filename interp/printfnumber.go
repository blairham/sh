// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
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
	text := strings.TrimSpace(arg)
	if v, ok := cAgreedInteger(text); ok {
		// The operand is an integer every reading in the panel agrees about,
		// so no dialect is consulted: `printf '%d' 42` and `printf '%d' 0x10`
		// are the same in all seven columns.
		return v, 0, false
	}
	f, code, stop := r.printfPartialNumber(arg, text, false)
	return floatToInt64(f), code, stop
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
	text := strings.TrimSpace(arg)
	if f, ranged, whole := cWholeNumber(text, true); whole {
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
	switch r.numberReading() {
	case PrintfNumberWholeOperand:
		v, ranged, whole := cWholeNumber(text, float)
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
		v, ranged, whole := cWholeNumber(text, float)
		switch {
		case whole && ranged:
			return v, r.printfOutOfRange(arg), false
		case whole:
			return v, 0, false
		}
		return r.leadingNumber(text, float), r.printfIncomplete(arg, text), false
	case PrintfNumberArithmetic:
		if v, _, whole := cWholeNumber(text, true); whole {
			// A numeral is read as a numeral, which is why `printf '%d' 010`
			// is 10 in ksh93 where `echo $((010))` there is 8. The reading is
			// C's `strtod` and not the conversion's, so `1e3` is 1000 at `%d`
			// as well.
			return v, 0, false
		}
		n, err := r.printfArithValue(text)
		if err == nil {
			return n.asFloat(), 0, false
		}
		code := r.printfArithFailure(err)
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
		return r.leadingNumber(text, true), code, false
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
func (r *Runner) printfArithValue(text string) (arithNum, error) {
	if text == "" {
		// No expression at all, which every evaluating column reads as zero
		// and says nothing about: `printf '%d' ' '` is `[0]` at 0 in both.
		return intNum(0), nil
	}
	p := syntax.NewParser("", r.dialect())
	tree := p.ParseArithFor(text, syntax.Pos{})
	err := p.Err()
	if err == nil && tree == nil {
		// No tree and no complaint is text holding an expansion the parser
		// leaves for its caller. An operand has already been expanded once
		// and does not go through it again, so a `$` left here is an operand
		// failure — the same reading arithValueAsExpression takes of it.
		err = &syntax.Error{Kind: syntax.ErrArithOperand, Expr: text, Token: text}
	}
	if err != nil {
		return intNum(0), arithError{msg: r.subscriptFailure(text, err), complete: true}
	}
	n, err := r.evalNum(tree)
	if err != nil {
		return intNum(0), arithError{msg: r.arithFailure(text, err), complete: true}
	}
	return n, nil
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

// printfIncomplete reports an operand that was not a number, or not all of
// one, where the dialect reports it at all.
func (r *Runner) printfIncomplete(arg, text string) int {
	if !r.ask(r.sem().PrintfReportsBadNumber, "`printf` complaining about an operand that is not a number") {
		return 0
	}
	d := r.diag()
	if w := printfBadNumberBase(d, arg); w != "" {
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
func (r *Runner) printfArithFailure(err error) int {
	msg := err.Error()
	if w := r.diag().PrintfArithOperandFailure; w != "" {
		msg = Wording(w, "%[1]s", msg)
	}
	r.diagf("%s\n", msg)
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
