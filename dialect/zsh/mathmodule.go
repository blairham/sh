// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"errors"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
)

// The `zsh/mathfunc` module: the C math library under names arithmetic can
// call.
//
// Measured 2026-09-09 against zsh 5.9.2 (Homebrew, aarch64) with a scratch
// HOME and no startup files. `zmodload -lF zsh/mathfunc` names forty-seven
// features and every one of them is an `f:` — a math function — so this file
// is the module in full and there is nothing of it anywhere else.
//
// # What made it worth doing, and it is not `sqrt`
//
// #1618 records the module as one of four a real startup loads, and the
// plugin that loads it does so twice: `zmodload zsh/mathfunc zsh/zselect ||
// return` guards a whole prompt segment, and a second `zmodload zsh/mathfunc`
// sits at the file's top level. Both were `failed to load module` before this,
// so the failure was never about an expression that would not evaluate — it
// was a `return` taken at load time, with the segment never defined.
//
// # The results are formatted, not merely computed
//
// `$(( sqrt(4) ))` is `2.` and not `2`, and that is the trap in this module
// rather than the arithmetic. A whole float keeps its point in this shell and
// seventeen significant digits is its precision, which is
// [interp.Semantics]'s ArithFloatKeepsPoint and ArithFloatDigits and was
// already right; what a function has to get right is **which of the two kinds
// of number it returns**. Four of the forty-seven return integers —
//
//	abs      the type of its argument: abs(-2) is 2 and abs(-2.0) is 2.
//	int      always, truncating toward zero: int(-3.7) is -3
//	ilogb    always: ilogb(8) is 3 where logb(8) is 3.
//	signgam  always
//
// — and the other forty-three return floats however whole they are, which is
// what makes `floor(3.7)` print `3.` where `int(3.7)` prints `3`. A shared
// float64 signature would have collapsed all four of those rows.
//
// # The edges are the platform's and were measured, not assumed
//
//	sqrt(-1), acos(2), fmod(7,0)   NaN
//	log(0), logb(0)                -Inf
//	exp(1000), ldexp(1,2000)       Inf
//	rint(2.5), rint(3.5)           2. and 4. — half goes to even
//	ilogb(0)                       -2147483648
//	int(1e30), int(-1e30)          the largest and smallest integers
//	int(NaN)                       0
//
// None of them is an error: the expression evaluates and the value is named.
// That is worth stating because this shell's arithmetic *does* refuse a
// division by zero, so "an operation with no finite answer fails" was a
// plausible reading and is the wrong one for every row here.
//
// `int` saturating rather than wrapping is the one that needed care in Go,
// where converting an out-of-range float to an integer is not defined to do
// anything in particular. See mathInt.
//
// # `signgam` is zero and it stays zero
//
// The name is C's global that `lgamma` sets to the sign of the gamma function.
// Measured, this shell never sets it: `$(( lgamma(-0.5) ))` and
// `$(( lgamma(2.5) ))` both leave `$(( signgam ))` at 0, and a fresh shell
// reads 0 as well. So it is a constant here, which is what was observed rather
// than what the name suggests — and it is registered because the module names
// it and a script asking for it gets the same 0 it would get from zsh.
//
// # `rand48` is why a math function is handed its operands unevaluated
//
// Forty-six of these take numbers. `rand48` takes the *name* of a variable
// holding forty-eight bits of generator state, and the state is twelve
// hexadecimal digits — not a number this shell's arithmetic reads, so
// evaluating the operand would fail before the function ever ran. See
// [interp.MathCall], which exists for this and hands over the operands
// unevaluated so that each function asks for what it wants.
//
// The state's spelling was measured rather than guessed, and it is exactly the
// three sixteen-bit words a C `erand48` takes, **lowest first**:
//
//	seed=000000000001; (( rand48(seed) ))    0.90010070800785158, seed=000b0000e66d
//
// which pins the whole thing at once — that `000000000001` means the state
// 0x000100000000 rather than 1, that the multiplier and increment are
// drand48's, and that the value is the new state over 2^48. A reading in which
// the digits are one big-endian number gives 8.96e-05 for the same line.
//
// A variable that is unset, or that holds anything but twelve hexadecimal
// digits, is *reseeded* rather than refused — measured with `seed=zzz`, which
// yields a value and leaves twelve fresh digits behind. So does the no-operand
// form, which keeps its state where no script can reach it.
//
// # Three divergences, all named
//
//   - `rand48(1,2)` is `not an identifier: 1,2` there and
//     `wrong number of arguments: rand48(1,2)` here. The arity is 0 or 1 in
//     both, and this shell checks it before looking at the operand where zsh
//     reads the operand first; the call is refused either way.
//   - `gamma` is whatever the platform's C library calls `gamma`, and on this
//     one it is the true gamma function — `gamma(5)` is `24.`. Some C
//     libraries make that name a synonym for `lgamma` instead. This
//     implementation is the true gamma, which is what the panel says here, and
//     no corpus row pins it for exactly that reason.
//   - **`erf`, `erfc`, `expm1` and `lgamma` differ in their last binary
//     digit.** Go's math package computes them here and the platform's C
//     library computes them there, and the two disagree by one unit in the
//     last place — in *both* directions, so neither is simply the rounder one.
//     Closing it would mean calling the platform's library, which would make
//     this shell's arithmetic differ from itself between two machines. The
//     four are pinned to this shell's own values in mathmodule_test.go, where
//     the numbers are, and no corpus row pins any of them.

// registerMathFuncModule installs all forty-seven.
//
// At startup rather than when `zmodload` runs, which is the shape every module
// in this dialect already has — see parameter.go, datetime.go and terminfo.go.
// `zmodload` reports what this shell has; it does not bring it.
func registerMathFuncModule(r *interp.Runner) {
	for _, f := range mathFuncModule {
		r.RegisterMathFunction(f.name, f.min, f.max, f.fn)
	}
}

// mathFunction is one of the forty-seven: the name arithmetic calls, the operand
// count it accepts, and what it computes.
type mathFunction struct {
	name     string
	min, max int
	fn       interp.MathFunction
}

// mathFuncModule is the module's feature list turned into implementations, in
// the order `zmodload -lF zsh/mathfunc` writes it — which is alphabetical, so
// a name can be found here against that listing.
//
// The arity of each was measured one function at a time, by calling it with
// nought, one, two and three operands and recording which counts were `wrong
// number of arguments`. Two are not simply "one" or "two": `atan` takes one or
// two — the second turning it into the two-argument arctangent — and `rand48`
// takes none or one.
var mathFuncModule = buildMathFuncModule()

func buildMathFuncModule() []mathFunction {
	out := make([]mathFunction, 0, 47)
	for _, u := range mathUnaryFloat {
		out = append(out, mathFunction{name: u.name, min: 1, max: 1, fn: unaryMathFunc(u.fn)})
	}
	for _, b := range mathBinaryFloat {
		out = append(out, mathFunction{name: b.name, min: 2, max: 2, fn: binaryMathFunc(b.fn)})
	}
	return append(out,
		mathFunction{name: "abs", min: 1, max: 1, fn: mathAbs},
		mathFunction{name: "atan", min: 1, max: 2, fn: mathAtan},
		mathFunction{name: "ilogb", min: 1, max: 1, fn: mathIlogb},
		mathFunction{name: "int", min: 1, max: 1, fn: mathInt},
		mathFunction{name: "rand48", min: 0, max: 1, fn: mathRand48},
		mathFunction{name: "signgam", min: 0, max: 0, fn: mathSigngam},
	)
}

// mathUnaryFloat are the thirty-three that take one number and give a float back
// however whole it is.
//
// `float` is in this list and is the identity, which is the whole of what it
// does: it is the conversion, and the conversion is visible because `float(3)`
// prints `3.` where `3` prints `3`.
var mathUnaryFloat = []struct {
	name string
	fn   func(float64) float64
}{
	{"acos", math.Acos},
	{"acosh", math.Acosh},
	{"asin", math.Asin},
	{"asinh", math.Asinh},
	{"atanh", math.Atanh},
	{"cbrt", math.Cbrt},
	{"ceil", math.Ceil},
	{"cos", math.Cos},
	{"cosh", math.Cosh},
	{"erf", math.Erf},
	{"erfc", math.Erfc},
	{"exp", math.Exp},
	{"expm1", math.Expm1},
	{"fabs", math.Abs},
	{"float", func(f float64) float64 { return f }},
	{"floor", math.Floor},
	{"gamma", math.Gamma},
	{"j0", math.J0},
	{"j1", math.J1},
	{"lgamma", mathLgamma},
	{"log", math.Log},
	{"log10", math.Log10},
	{"log1p", math.Log1p},
	{"log2", math.Log2},
	{"logb", math.Logb},
	{"rint", math.RoundToEven},
	{"sin", math.Sin},
	{"sinh", math.Sinh},
	{"sqrt", math.Sqrt},
	{"tan", math.Tan},
	{"tanh", math.Tanh},
	{"y0", math.Y0},
	{"y1", math.Y1},
}

// mathBinaryFloat are the eight that take two numbers.
//
// Three of them take an *integer* as one of their two, and which one it is
// differs: `ldexp` and `scalb` scale by a power of two given second, and `jn`
// and `yn` take the Bessel function's order first. Measured — `jn(0,1)` is
// `j0(1)` and `jn(1,2)` is `j1(2)`, so the order leads.
var mathBinaryFloat = []struct {
	name string
	fn   func(a, b float64) float64
}{
	{"copysign", math.Copysign},
	{"fmod", math.Mod},
	{"hypot", math.Hypot},
	{"jn", func(n, x float64) float64 { return math.Jn(mathExponent(n), x) }},
	{"ldexp", func(x, n float64) float64 { return math.Ldexp(x, mathExponent(n)) }},
	{"nextafter", math.Nextafter},
	{"scalb", func(x, n float64) float64 { return math.Ldexp(x, mathExponent(n)) }},
	{"yn", func(n, x float64) float64 { return math.Yn(mathExponent(n), x) }},
}

// mathLgamma is the log-gamma, without the sign C returns beside it. See the
// note on `signgam` at the top: this shell never publishes that sign, so
// dropping it here is what makes the two agree.
func mathLgamma(f float64) float64 {
	v, _ := math.Lgamma(f)
	return v
}

// mathExponent is an operand that is an integer rather than a number: a power
// of two to scale by, or a Bessel function's order. Truncated the way `int`
// truncates, and clamped to what a Go `int` holds so that a nonsensical
// operand is a saturated one rather than an undefined conversion.
func mathExponent(f float64) int { return mathTruncate(f) }

// unaryMathFunc and binaryMathFunc are the two shapes above turned into
// registrations. One place where an operand is evaluated for each shape, so a
// function that gains an operand cannot gain a second way of reading one.
func unaryMathFunc(fn func(float64) float64) interp.MathFunction {
	return func(_ *interp.Runner, call interp.MathCall) (interp.MathValue, error) {
		x, err := call.Value(0)
		if err != nil {
			return interp.MathValue{}, err
		}
		return interp.MathFloat(fn(x.Float())), nil
	}
}

func binaryMathFunc(fn func(a, b float64) float64) interp.MathFunction {
	return func(_ *interp.Runner, call interp.MathCall) (interp.MathValue, error) {
		a, err := call.Value(0)
		if err != nil {
			return interp.MathValue{}, err
		}
		b, err := call.Value(1)
		if err != nil {
			return interp.MathValue{}, err
		}
		return interp.MathFloat(fn(a.Float(), b.Float())), nil
	}
}

// mathAbs is the absolute value, and the one function whose *kind* of result
// is its argument's: `abs(-2)` is `2` and `abs(-2.0)` is `2.`.
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
		// The smallest integer has no positive counterpart and stays where
		// it is, which is what the panel does: `abs(-9223372036854775808)`
		// is itself there.
		if n == math.MinInt {
			return interp.MathInt(n), nil
		}
		n = -n
	}
	return interp.MathInt(n), nil
}

// mathAtan is the arctangent, and the only one whose second operand changes
// what it computes: with one it is the arctangent of a ratio, with two it is
// the angle to the point, which is C's `atan2` and takes the numerator first.
func mathAtan(_ *interp.Runner, call interp.MathCall) (interp.MathValue, error) {
	y, err := call.Value(0)
	if err != nil {
		return interp.MathValue{}, err
	}
	if call.Len() == 1 {
		return interp.MathFloat(math.Atan(y.Float())), nil
	}
	x, err := call.Value(1)
	if err != nil {
		return interp.MathValue{}, err
	}
	return interp.MathFloat(math.Atan2(y.Float(), x.Float())), nil
}

// mathIlogb is the binary exponent as an *integer*, where `logb` is the same
// number as a float. Measured: `ilogb(0)` is -2147483648 — the smallest
// thirty-two-bit integer, which is what C names for a value with no exponent
// — and `ilogb(-8)` is 3, so the sign is dropped rather than refused.
func mathIlogb(_ *interp.Runner, call interp.MathCall) (interp.MathValue, error) {
	x, err := call.Value(0)
	if err != nil {
		return interp.MathValue{}, err
	}
	return interp.MathInt(math.Ilogb(x.Float())), nil
}

// mathInt truncates toward zero and gives an integer back.
func mathInt(_ *interp.Runner, call interp.MathCall) (interp.MathValue, error) {
	x, err := call.Value(0)
	if err != nil {
		return interp.MathValue{}, err
	}
	if !x.IsFloat() {
		return interp.MathInt(x.Int()), nil
	}
	return interp.MathInt(mathTruncate(x.Float())), nil
}

// mathTruncate is the conversion Go leaves undefined, given the answers this
// shell was measured to give: `int(1e30)` is the largest integer, `int(-1e30)`
// the smallest, and `int` of a NaN is 0.
//
// Written out rather than left to a plain conversion because an out-of-range
// float converted to an integer in Go is not defined to do anything in
// particular — it is one number on one architecture and another elsewhere,
// which is exactly the sort of thing that passes here and fails on the release
// runner.
func mathTruncate(f float64) int {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= -float64(math.MinInt):
		return math.MaxInt
	case f <= float64(math.MinInt):
		return math.MinInt
	}
	return int(math.Trunc(f))
}

// mathSigngam is the sign C's `lgamma` records. Always 0 — see the top of the
// file, where the measurement is.
func mathSigngam(*interp.Runner, interp.MathCall) (interp.MathValue, error) {
	return interp.MathInt(0), nil
}

// The constants of the `drand48` generator: a value is the next state over
// 2^48, and the next state is this multiplier and increment applied to the
// last one, modulo 2^48.
const (
	rand48Multiplier = 0x5deece66d
	rand48Increment  = 0xb
	rand48Modulus    = 1 << 48
	// rand48Digits is how many hexadecimal digits a state is written in: three
	// sixteen-bit words, four digits each.
	rand48Digits = 12
)

// rand48State is where the no-operand form keeps its state, under a name no
// script can reach — the same device zmodload, zstyle and emulate use, and it
// gives a subshell its own copy for the same reason.
const rand48State = ".zsh.rand48"

// mathRand48 is a number in [0,1) from a forty-eight-bit generator, with the
// state either in a variable the caller names or in the shell's own.
//
// The operand is a *name* and is never evaluated: see the note at the top of
// this file, and [interp.MathCall.Name], which is what makes that possible. An
// operand that is not a bare name at all — `rand48(1+1)` — is refused rather
// than treated as a number, because there is nowhere to put the state back.
func mathRand48(r *interp.Runner, call interp.MathCall) (interp.MathValue, error) {
	name := rand48State
	if call.Len() == 1 {
		var ok bool
		if name, ok = call.Name(0); !ok {
			return interp.MathValue{}, errors.New("rand48: not an identifier")
		}
	}
	stored, _ := r.GetVar(name)
	state := parseRand48State(stored)
	// Unsigned throughout: the multiplier is thirty-five bits and the state
	// forty-eight, so the product overflows a signed sixty-four-bit integer
	// and its remainder comes back *negative*. Measured against zsh, that
	// mistake gives -0.09989929199214842 where the answer is
	// 0.90010070800785158 — one less, and plausible enough to pass a test
	// that only checked the range.
	state = (state*rand48Multiplier + rand48Increment) % rand48Modulus
	r.SetVar(name, writeRand48State(state))
	return interp.MathFloat(float64(state) / float64(rand48Modulus)), nil
}

// parseRand48State reads the twelve hexadecimal digits, and seeds afresh when
// the variable holds anything else — which is what an unset one holds and is
// measured behavior for a variable holding rubbish as well.
//
// The three words are lowest first, which is the order C's `erand48` takes its
// array in and which the measurement at the top of this file pins: the state
// `000000000001` is 0x000100000000 and yields 0.90010070800785158, where
// reading the digits as one number yields 8.96e-05.
func parseRand48State(s string) uint64 {
	if len(s) != rand48Digits {
		return seedRand48()
	}
	var state uint64
	for i := range 3 {
		word, err := strconv.ParseUint(s[i*4:i*4+4], 16, 16)
		if err != nil {
			return seedRand48()
		}
		state |= word << (16 * i)
	}
	return state
}

// writeRand48State is the state back as twelve lowercase hexadecimal digits,
// in the same lowest-word-first order parseRand48State reads.
func writeRand48State(state uint64) string {
	var b strings.Builder
	for i := range 3 {
		word := (state >> (16 * i)) & 0xffff
		// Or-ed with the bit above the four digits and then cut off again,
		// which is how a fixed width of four is got without a format string.
		b.WriteString(strconv.FormatUint(word|0x10000, 16)[1:])
	}
	return b.String()
}

// seedRand48 starts a generator nobody seeded.
//
// math/rand/v2 rather than crypto/rand, which is the reasoning
// internal/event/event.go writes down and applies here for the same reason:
// nothing about a shell's arithmetic is a secret, and the first crypto/rand
// read in a process permanently opens a descriptor a shell would rather not
// spend.
func seedRand48() uint64 { return rand.Uint64() % rand48Modulus }
