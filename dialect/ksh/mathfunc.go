// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import (
	"math"
	"runtime"

	"github.com/blairham/sh/interp"
)

// The math functions arithmetic can call in this shell: the C math library
// under the names C gives it, built in and needing nothing registered.
//
// Measured 2026-09-13 against ksh93u+ 2012-08-01 (`/bin/ksh`, macOS 26), with
// `env -i PATH=/usr/bin:/bin` and a scratch HOME. Every name below was called
// with nought, one, two, three and four operands and its arity read off which
// counts answered; the whole of C99's `math.h` was swept for names beyond
// them, and what is not here came back `unknown function`.
//
// # This is the second table, and the first one is not a model for it
//
// dialect/zsh has forty-seven of these and they arrive through `zmodload
// zsh/mathfunc`. These sixty-nine are simply present, which is the first
// difference and the one a script sees: `$(( sqrt(4) ))` needs no preamble
// here. The sets also differ — this shell has `exp2`, `fma`, `round`,
// `trunc`, `tgamma`, the six ordered comparisons and the five predicates,
// and has no `float`, `gamma`, `rand48` or `signgam` — so neither table is
// the other's subset and neither is derived from the other.
//
// The third difference is the one that would have been easy to carry across
// wrongly. **Every function here gives a float back**, including the four
// that look like integers, and it is measurable: `$(( 7/int(2) ))` is 3.5
// where an integer result would make it 3, and `$(( ilogb(8)+0.5 ))` is 3.5.
// The shell that needs the distinction spells a whole float with its point
// and this one does not, so `$(( sqrt(4) ))` is `2` here and `2.` there with
// the same number underneath.
//
// `abs` is the exception, and it is the same exception the other table has
// for a different reason: `$(( abs(-9223372036854775807) ))` is that number
// exactly, which a float64 cannot hold, so the argument's own kind is kept.
//
// # Four measured divergences, all named
//
//   - **`int` is `floor` here and truncation there.** `int(-3.7)` is -4 in
//     this shell and -3 in the other, and `int(-0.5)` is -1 against 0. So the
//     name is shared and the function is not.
//   - **`scalbn` answers `inf` for every operand pair** — `scalbn(3,0)`,
//     `scalbn(3,1)` and `scalbn(1,2)` alike — where `scalb` and `ldexp` on
//     the same operands give 3, 6 and 4. That reads as an operand reaching
//     the C function under the wrong type rather than as a rule, and this
//     implementation computes the scaling `scalbn` names. Nothing in the
//     corpus pins it either way.
//   - **A literal outside the double range does not survive being read.**
//     `$(( 1e400 ))` is `-0` there and `+inf` here, which is the number
//     reader and not these functions; it shows up through them because
//     `fpclassify(1e400)` is then 3, the code for a zero. Beyond the two
//     shells' agreement to disagree about a literal, nothing here depends on
//     it.
//   - **The very large integers come back rounded.** This shell's arithmetic
//     is a long double, whose sixty-four-bit significand holds every signed
//     integer exactly; a float64's fifty-three bits do not, so
//     `$(( int(9223372036854775807) ))` is that number there and
//     9.22337203685478e+18 here. That is the arithmetic engine's width rather
//     than this table's, and it is recorded here because this table is where
//     it becomes visible.
//
// # `fpclassify` answers with the platform's own constants
//
// They are a C library's numbering and the two libraries this shell is built
// against do not agree: measured on macOS, a NaN is 1, an infinity 2, a zero
// 3 and a normal number 4, where glibc numbers the same five from 0 with the
// subnormal before the normal. So the answer is chosen by where this program
// is running, which is the only reading under which we and a locally built
// ksh93 say the same thing on both. No corpus row pins it, for that reason.

// registerMathFuncs installs all sixty-nine.
//
// At startup, because that is where they are: this shell has no `zmodload`
// and nothing to load. See Apply.
func registerMathFuncs(r *interp.Runner) {
	for _, f := range mathFuncTable {
		r.RegisterMathFunction(f.name, f.min, f.max, f.fn)
	}
}

// mathFunction is one of the sixty-nine: the name arithmetic calls, the
// operand count it takes, and what it computes.
type mathFunction struct {
	name     string
	min, max int
	fn       interp.MathFunction
}

// mathFuncTable is the whole set, built from the three shapes it has.
var mathFuncTable = buildMathFuncTable()

func buildMathFuncTable() []mathFunction {
	out := make([]mathFunction, 0, 69)
	for _, u := range mathUnary {
		out = append(out, mathFunction{name: u.name, min: 1, max: 1, fn: interp.UnaryMathFunction(u.fn)})
	}
	for _, b := range mathBinary {
		out = append(out, mathFunction{name: b.name, min: 2, max: 2, fn: interp.BinaryMathFunction(b.fn)})
	}
	return append(out,
		mathFunction{name: "abs", min: 1, max: 1, fn: mathAbs},
		mathFunction{name: "fma", min: 3, max: 3, fn: mathFma},
	)
}

// mathUnary are the forty-six that take one number.
//
// The five predicates and `fpclassify` are in here rather than in a table of
// their own because they answer with a number like everything else does —
// `isnan(sqrt(-1))` is 1 and `isnan(0)` is 0, and `$(( isnan(0)+0.5 ))` is
// 0.5, so the 1 and the 0 are floats as much as `sqrt(4)`'s 2 is.
var mathUnary = []struct {
	name string
	fn   func(float64) float64
}{
	{"acos", math.Acos},
	{"acosh", math.Acosh},
	{"asin", math.Asin},
	{"asinh", math.Asinh},
	{"atan", math.Atan},
	{"atanh", math.Atanh},
	{"cbrt", math.Cbrt},
	{"ceil", math.Ceil},
	{"cos", math.Cos},
	{"cosh", math.Cosh},
	{"erf", math.Erf},
	{"erfc", math.Erfc},
	{"exp", math.Exp},
	{"exp2", math.Exp2},
	{"expm1", math.Expm1},
	{"fabs", math.Abs},
	{"floor", math.Floor},
	{"fpclassify", mathFpclassify},
	{"ilogb", func(f float64) float64 { return float64(math.Ilogb(f)) }},
	// `int` is this shell's floor and not a truncation: `int(-3.7)` is -4 and
	// `int(-0.5)` is -1, where `trunc` on the same operands is -3 and -0.
	{"int", math.Floor},
	{"isfinite", mathPredicate(func(f float64) bool { return !math.IsInf(f, 0) && !math.IsNaN(f) })},
	{"isinf", mathPredicate(func(f float64) bool { return math.IsInf(f, 0) })},
	{"isnan", mathPredicate(math.IsNaN)},
	{"isnormal", mathPredicate(mathIsNormal)},
	{"j0", math.J0},
	{"j1", math.J1},
	{"lgamma", mathLgamma},
	{"log", math.Log},
	{"log10", math.Log10},
	{"log1p", math.Log1p},
	{"log2", math.Log2},
	{"logb", math.Logb},
	{"nearbyint", math.RoundToEven},
	{"rint", math.RoundToEven},
	{"round", math.Round},
	{"signbit", mathPredicate(math.Signbit)},
	{"sin", math.Sin},
	{"sinh", math.Sinh},
	{"sqrt", math.Sqrt},
	{"tan", math.Tan},
	{"tanh", math.Tanh},
	{"tgamma", math.Gamma},
	{"trunc", math.Trunc},
	{"y0", math.Y0},
	{"y1", math.Y1},
}

// mathBinary are the twenty-two that take two numbers.
//
// Three of them read one operand as an *integer*, and which one it is
// differs: `ldexp` and `scalb` scale by a power of two given second, while
// `jn` and `yn` take the Bessel function's order first. Measured —
// `jn(2,1)` is 0.1149034849319, which is J₂(1) and not J₁(2).
var mathBinary = []struct {
	name string
	fn   func(a, b float64) float64
}{
	{"atan2", math.Atan2},
	{"copysign", math.Copysign},
	{"fdim", math.Dim},
	{"fmax", mathFmax},
	{"fmin", mathFmin},
	{"fmod", math.Mod},
	{"hypot", math.Hypot},
	{"isgreater", mathComparison(func(a, b float64) bool { return a > b })},
	{"isgreaterequal", mathComparison(func(a, b float64) bool { return a >= b })},
	{"isless", mathComparison(func(a, b float64) bool { return a < b })},
	{"islessequal", mathComparison(func(a, b float64) bool { return a <= b })},
	{"islessgreater", mathComparison(func(a, b float64) bool { return a < b || a > b })},
	{"isunordered", mathComparison(func(a, b float64) bool { return math.IsNaN(a) || math.IsNaN(b) })},
	{"jn", func(n, x float64) float64 { return math.Jn(interp.MathTruncate(n), x) }},
	{"ldexp", func(x, n float64) float64 { return math.Ldexp(x, interp.MathTruncate(n)) }},
	{"nextafter", math.Nextafter},
	{"nexttoward", math.Nextafter},
	{"pow", math.Pow},
	{"remainder", math.Remainder},
	{"scalb", func(x, n float64) float64 { return math.Ldexp(x, interp.MathTruncate(n)) }},
	// The scaling `scalbn` names rather than the `inf` the measured build
	// answers with — see the divergence at the top of this file.
	{"scalbn", func(x, n float64) float64 { return math.Ldexp(x, interp.MathTruncate(n)) }},
	{"yn", func(n, x float64) float64 { return math.Yn(interp.MathTruncate(n), x) }},
}

// mathPredicate is a question about one number, answered as 1 or 0 — which is
// what every one of these answers with and the reason they are ordinary
// entries in the unary table.
func mathPredicate(fn func(float64) bool) func(float64) float64 {
	return func(f float64) float64 {
		if fn(f) {
			return 1
		}
		return 0
	}
}

// mathComparison is the same for the six that compare two numbers. They are
// the quiet forms C names: a NaN on either side makes every one of them false
// except `isunordered`, which is the one that asks.
func mathComparison(fn func(a, b float64) bool) func(a, b float64) float64 {
	return func(a, b float64) float64 {
		if fn(a, b) {
			return 1
		}
		return 0
	}
}

// mathIsNormal is a finite number that is neither zero nor subnormal.
func mathIsNormal(f float64) bool {
	if math.IsNaN(f) || math.IsInf(f, 0) || f == 0 {
		return false
	}
	return math.Abs(f) >= math.SmallestNonzeroFloat64*(1<<52)
}

// mathLgamma is the log-gamma without the sign C returns beside it, which
// this shell does not publish: there is no `signgam` here to read it from.
func mathLgamma(f float64) float64 {
	v, _ := math.Lgamma(f)
	return v
}

// mathFmax and mathFmin are C's, which differ from a plain comparison in one
// place: a NaN on one side is *ignored* rather than propagated. Measured,
// `fmax(1,sqrt(-1))`, `fmax(sqrt(-1),1)` and `fmin(1,sqrt(-1))` are all 1,
// where Go's math.Max and math.Min would answer NaN for each.
func mathFmax(a, b float64) float64 {
	switch {
	case math.IsNaN(a):
		return b
	case math.IsNaN(b):
		return a
	}
	return math.Max(a, b)
}

func mathFmin(a, b float64) float64 {
	switch {
	case math.IsNaN(a):
		return b
	case math.IsNaN(b):
		return a
	}
	return math.Min(a, b)
}

// mathFpclassify is C's classification of a number, answered with the
// platform's own constants — see the note at the top of this file, and
// mathFpclassifyCodes for where the two numberings come from.
func mathFpclassify(f float64) float64 {
	c := mathFpclassifyCodes()
	switch {
	case math.IsNaN(f):
		return float64(c.nan)
	case math.IsInf(f, 0):
		return float64(c.infinite)
	case f == 0:
		return float64(c.zero)
	case !mathIsNormal(f):
		return float64(c.subnormal)
	}
	return float64(c.normal)
}

// fpclassifyCodes is one C library's numbering of the five classes.
type fpclassifyCodes struct{ nan, infinite, zero, subnormal, normal int }

// mathFpclassifyCodes is the numbering of the platform this program is
// running on.
//
// Measured 2026-09-13 on macOS 26: `fpclassify(sqrt(-1))` is 1,
// `fpclassify(1/0.0)` is 2, `fpclassify(0)` is 3 and `fpclassify(-1)` is 4,
// which is the BSD family's order — NaN, infinite, zero, normal, subnormal,
// numbered from 1. glibc numbers the same five from 0 and puts the subnormal
// before the normal, so a normal number is 4 in both and nothing else is.
//
// A subnormal could not be reached from a script on the measured build: its
// number reader turns `1e-320` into a zero, the same way it turns `1e400`
// into one, so the fifth code is the one the numbering implies rather than
// one that was read back.
func mathFpclassifyCodes() fpclassifyCodes {
	switch runtime.GOOS {
	case "darwin", "freebsd", "netbsd", "openbsd", "dragonfly":
		return fpclassifyCodes{nan: 1, infinite: 2, zero: 3, subnormal: 5, normal: 4}
	}
	return fpclassifyCodes{nan: 0, infinite: 1, zero: 2, subnormal: 3, normal: 4}
}

// mathAbs is the absolute value, and the one function here whose result is
// not a float: the kind of its argument survives, which is what lets
// `abs(-9223372036854775807)` come back exactly rather than rounded to the
// nearest float64.
func mathAbs(_ *interp.Runner, call interp.MathCall) (interp.MathValue, error) {
	x, err := call.Value(0)
	if err != nil {
		return interp.MathValue{}, err
	}
	if x.IsFloat() {
		return interp.MathFloat(math.Abs(x.Float())), nil
	}
	n := x.Int()
	if n < 0 {
		// The smallest integer has no positive counterpart in the word, and
		// this shell does not work in the word: the value is a double, so
		// |-2^63| is 2^63, and 2^63 comes back through the saturating cast
		// as the largest integer. Measured 2026-09-16 against AT&T ksh93u+
		// 2012-08-01 — `$(( abs(-9223372036854775807) ))` and
		// `$(( abs(-9223372036854775808) ))` are both 9223372036854775807
		// there, and the first of those is already the most negative value
		// by the time abs sees it. See
		// Semantics.ArithValuesAreCarriedInAFloat.
		if n == math.MinInt {
			return interp.MathInt(math.MaxInt), nil
		}
		n = -n
	}
	return interp.MathInt(n), nil
}

// mathFma is the fused multiply-add, and the only function here with three
// operands: `fma(a,b,c)` is `a*b+c` with one rounding rather than two.
func mathFma(_ *interp.Runner, call interp.MathCall) (interp.MathValue, error) {
	a, err := call.Value(0)
	if err != nil {
		return interp.MathValue{}, err
	}
	b, err := call.Value(1)
	if err != nil {
		return interp.MathValue{}, err
	}
	c, err := call.Value(2)
	if err != nil {
		return interp.MathValue{}, err
	}
	return interp.MathFloat(math.FMA(a.Float(), b.Float(), c.Float())), nil
}
