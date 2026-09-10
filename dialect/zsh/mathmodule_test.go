// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The `zsh/mathfunc` module: the C math library under names arithmetic can
// call.
//
// Every expectation here was measured on zsh 5.9.2 (Homebrew, aarch64) with a
// scratch HOME and no startup files, one function at a time. Two divergences
// are named in mathmodule.go and neither is asserted here as agreement.
//
// The assertions are on **what a value prints as** and not on a number within
// a tolerance, which is the point: `sqrt(4)` is `2.` and `sqrt(2)` is
// `1.4142135623730951`, so a shell that computed the right float and wrote it
// with the wrong precision, or dropped the trailing point, fails.

// mathValueOf evaluates one expression and returns what it printed.
func mathValueOf(t *testing.T, expr string) string {
	t.Helper()
	out, _, errs := runZshSplit(t, t.TempDir(), "print -r -- $(( "+expr+" ))")
	return out + errs
}

// TestEveryMathFunctionPrintsTheValueTheShellPrints is the module row by row.
//
// The float spellings are the trap this table exists for. Seventeen
// significant digits and a trailing point on a whole one are this dialect's
// float format, so `floor(3.7)` is `3.` where `int(3.7)` is `3` — a table of
// float64 results compared numerically would call those two the same answer.
func TestEveryMathFunctionPrintsTheValueTheShellPrints(t *testing.T) {
	for _, c := range []struct{ expr, want string }{
		// The line #1618 opens with.
		{"sqrt(4)", "2."},
		{"sqrt(2)", "1.4142135623730951"},
		{"acos(1)", "0."},
		{"acosh(1)", "0."},
		{"asin(1)", "1.5707963267948966"},
		{"asinh(1)", "0.88137358701954305"},
		{"atanh(1)", "Inf"},
		{"cbrt(27)", "3."},
		{"cbrt(-8)", "-2."},
		{"ceil(3.2)", "4."},
		{"ceil(-3.8)", "-3."},
		{"cos(1)", "0.54030230586813977"},
		{"cosh(1)", "1.5430806348152437"},
		{"exp(0)", "1."},
		{"exp(1)", "2.7182818284590451"},
		{"fabs(-2)", "2."},
		{"float(3)", "3."},
		{"floor(3.7)", "3."},
		{"floor(-3.2)", "-4."},
		{"gamma(5)", "24."},
		{"j0(1)", "0.76519768655796661"},
		{"j1(1)", "0.4400505857449335"},
		{"log(1)", "0."},
		{"log10(1)", "0."},
		{"log1p(1)", "0.69314718055994529"},
		{"log2(8)", "3."},
		{"logb(1)", "0."},
		{"sin(1)", "0.8414709848078965"},
		{"sinh(1)", "1.1752011936438014"},
		{"tan(1)", "1.5574077246549021"},
		{"tanh(1)", "0.76159415595576485"},
		{"y0(1)", "0.08825696421567697"},
		{"y1(1)", "-0.78121282130028868"},
		// The eight of two operands, including the three whose second — or
		// first — operand is an integer rather than a number.
		{"copysign(3,-1)", "-3."},
		{"copysign(-3,1)", "3."},
		{"fmod(7,3)", "1."},
		{"hypot(3,4)", "5."},
		{"jn(0,1)", "0.76519768655796661"},
		{"jn(1,2)", "0.57672480775687329"},
		{"ldexp(1,10)", "1024."},
		{"nextafter(1,2)", "1.0000000000000002"},
		{"scalb(1,3)", "8."},
		{"scalb(1,-3)", "0.125"},
		{"yn(0,1)", "0.08825696421567697"},
		// The two-operand form of the one function that has both.
		{"atan(1)", "0.78539816339744828"},
		{"atan(1,2)", "0.46364760900080609"},
		// And the four that give an integer back.
		{"abs(-3)", "3"},
		{"abs(-3.5)", "3.5"},
		{"abs(0.0)", "0."},
		{"int(3.7)", "3"},
		{"int(-0.5)", "0"},
		{"int(3)", "3"},
		{"ilogb(8)", "3"},
		{"ilogb(-8)", "3"},
		{"signgam", "0"},
		{"signgam()", "0"},
	} {
		t.Run(c.expr, func(t *testing.T) {
			if got, want := mathValueOf(t, c.expr), c.want+"\n"; got != want {
				t.Errorf("$(( %s )) = %q, want %q", c.expr, got, want)
			}
		})
	}
}

// TestFourFunctionsDifferFromThisPlatformInTheirLastBit records a divergence
// rather than asserting agreement, and it is the only one in the module.
//
// `erf`, `erfc`, `expm1` and `lgamma` are computed by Go's math package here
// and by the platform's C library there, and the two disagree in the last
// binary digit — in *both* directions, so neither is simply the rounder one:
//
//	           this shell           zsh 5.9.2 on macOS 26   correctly rounded
//	erf(1)     0.84270079294971489  0.84270079294971478     …489
//	erfc(1)    0.15729920705028513  0.15729920705028516     …513
//	expm1(1)   1.7182818284590451   1.7182818284590453      …452
//	lgamma(5)  3.1780538303479458   3.1780538303479453      …456
//
// Nothing can be done about it short of calling the platform's library, which
// would make this shell's arithmetic differ from itself between two machines.
// The rows are pinned so that a *change* is still caught: what is not
// asserted is not measured, and four functions quietly excluded from the table
// above would be four functions nothing checks at all.
//
// The oracle corpus has no row for any of the four, for the same reason.
func TestFourFunctionsDifferFromThisPlatformInTheirLastBit(t *testing.T) {
	for _, c := range []struct{ expr, want string }{
		{"erf(1)", "0.84270079294971489"},
		{"erfc(1)", "0.15729920705028513"},
		{"expm1(1)", "1.7182818284590451"},
		{"lgamma(5)", "3.1780538303479458"},
	} {
		t.Run(c.expr, func(t *testing.T) {
			if got, want := mathValueOf(t, c.expr), c.want+"\n"; got != want {
				t.Errorf("$(( %s )) = %q, want %q", c.expr, got, want)
			}
		})
	}
}

// TestAnEdgeIsNamedRatherThanRefused: an operation with no finite answer
// evaluates and the value is named.
//
// Worth its own case because this shell's arithmetic *does* refuse a division
// by zero, so "no finite answer means the expression fails" was a plausible
// reading of the module and is the wrong one for every row here.
func TestAnEdgeIsNamedRatherThanRefused(t *testing.T) {
	for _, c := range []struct{ expr, want string }{
		{"sqrt(-1)", "NaN"},
		{"acos(2)", "NaN"},
		{"atanh(2)", "NaN"},
		{"fmod(7,0)", "NaN"},
		{"log(0)", "-Inf"},
		{"logb(0)", "-Inf"},
		{"exp(1000)", "Inf"},
		{"ldexp(1,2000)", "Inf"},
		// Half goes to even, which is the rounding a C `rint` does and is not
		// the same as rounding half away from zero: 3.5 and 2.5 land on the
		// same number.
		{"rint(2.5)", "2."},
		{"rint(3.5)", "4."},
		{"rint(-2.5)", "-2."},
		// The smallest thirty-two-bit integer, which is what C names for a
		// value with no exponent at all.
		{"ilogb(0)", "-2147483648"},
		// Saturating rather than wrapping, and a NaN truncating to nought.
		{"int(1e30)", "9223372036854775807"},
		{"int(-1e30)", "-9223372036854775808"},
		{"int(sqrt(-1))", "0"},
	} {
		t.Run(c.expr, func(t *testing.T) {
			if got, want := mathValueOf(t, c.expr), c.want+"\n"; got != want {
				t.Errorf("$(( %s )) = %q, want %q", c.expr, got, want)
			}
		})
	}
}

// TestTheOperandCountIsCheckedAndTheCallQuotedBack: each function's arity was
// measured by calling it with nought, one, two and three operands, and the
// refusal quotes the call as it was written.
func TestTheOperandCountIsCheckedAndTheCallQuotedBack(t *testing.T) {
	for _, expr := range []string{
		"sqrt()", "sqrt(1,2)", "atan()", "atan(1,2,3)",
		"copysign(1)", "fmod(1)", "signgam(1)", "abs()",
	} {
		t.Run(expr, func(t *testing.T) {
			got := mathValueOf(t, expr)
			want := "zsh:1: wrong number of arguments: " + expr + "\n"
			if got != want {
				t.Errorf("$(( %s )) = %q, want %q", expr, got, want)
			}
		})
	}
}

// TestRand48IsDeterministicFromItsSeedVariable is the whole of what makes
// `rand48` different from the other forty-six, and it is deterministic
// precisely because the state is the caller's.
//
// Two things are pinned by one line. The value — `0.90010070800785158` from a
// state written `000000000001` — says the three sixteen-bit words are read
// **lowest first**, since reading the digits as one number gives 8.96e-05
// instead. And the state left behind says the generator's step is drand48's
// and that it is written back in the same order.
//
// It also catches a signed overflow that no range check would: computed in
// signed arithmetic the product wraps and the remainder comes back negative,
// giving -0.09989929199214842 — exactly one less, in range, and wrong.
func TestRand48IsDeterministicFromItsSeedVariable(t *testing.T) {
	out, _, errs := runZshSplit(t, t.TempDir(),
		"seed=000000000001\n"+
			"print -r -- $(( rand48(seed) ))\n"+
			"print -r -- $seed\n"+
			"print -r -- $(( rand48(seed) ))\n"+
			"print -r -- $seed\n")
	want := "0.90010070800785158\n000b0000e66d\n0.041650067526212808\ne6ba942d0aa9\n"
	if got := out + errs; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestRand48ReseedsRatherThanRefusingAVariableItCannotRead: measured, a
// variable holding rubbish yields a value and twelve fresh digits.
//
// Asserted on the shape rather than on the digits, which are the point of a
// reseed. The discriminating half is the *second* line: a shell that left the
// variable alone, or that refused, would not write twelve hexadecimal digits
// over `zzz`.
func TestRand48ReseedsRatherThanRefusingAVariableItCannotRead(t *testing.T) {
	out, _, errs := runZshSplit(t, t.TempDir(),
		"seed=zzz\n"+
			"v=$(( rand48(seed) ))\n"+
			`[[ $v == 0.* ]] && print -r -- in-range`+"\n"+
			"setopt extendedglob\n"+
			`[[ $seed == [0-9a-f](#c12) ]] && print -r -- reseeded`+"\n")
	if got, want := out+errs, "in-range\nreseeded\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestRand48WithoutAnOperandKeepsItsOwnState: the no-operand form advances
// something, so two calls in one shell differ.
func TestRand48WithoutAnOperandKeepsItsOwnState(t *testing.T) {
	out, _, errs := runZshSplit(t, t.TempDir(),
		"a=$(( rand48() ))\nb=$(( rand48() ))\n"+
			`[[ $a != $b ]] && print -r -- moved`+"\n")
	if got, want := out+errs, "moved\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestAMathFunctionOfThisModuleIsNotInTheFunctionsMListing: measured against
// the shell that has both kinds, loading this module leaves `functions -M`
// printing nothing at all.
//
// Which is what keeps the two registries apart: a script that registers its
// own and then lists them gets its own back and not forty-seven lines of C
// library.
func TestAMathFunctionOfThisModuleIsNotInTheFunctionsMListing(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "zmodload zsh/mathfunc\nfunctions -M\n")
	if out != "" || st != 0 {
		t.Errorf("output = %q status %d, want nothing and 0", out, st)
	}
	out, st = runZsh(t, t.TempDir(), "g(){ }\nfunctions -M mine 1 1 g\nfunctions -M\n")
	if want := "functions -M mine 1 1 g\n"; out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q and 0", out, st, want)
	}
}

// TestTheMathFuncModuleLoadsAndListsAllFortySeven: `zmodload zsh/mathfunc` is
// the second line #1618 quotes.
func TestTheMathFuncModuleLoadsAndListsAllFortySeven(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"zmodload zsh/mathfunc && zmodload -lF zsh/mathfunc\n")
	if st != 0 {
		t.Fatalf("status = %d, want 0; output %q", st, out)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 47 {
		t.Errorf("listing has %d lines, want 47: %q", len(lines), out)
	}
	if lines[0] != "+f:abs" || lines[len(lines)-1] != "+f:yn" {
		t.Errorf("listing runs %q … %q, want +f:abs … +f:yn", lines[0], lines[len(lines)-1])
	}
}

// TestAMathFunctionTakenAwayHoldsTheModuleShut is the gate working in the
// direction nothing else exercises.
//
// A math function has a registry now and `zmodload` asks it, so the module's
// answer moves when the shell's does — `functions +M sqrt` takes one away and
// the module refuses by that name. A gate that only ever said yes would pass
// every other case in this file.
func TestAMathFunctionTakenAwayHoldsTheModuleShut(t *testing.T) {
	out, _, errs := runZshSplit(t, t.TempDir(), "functions +M sqrt\nzmodload zsh/mathfunc\n")
	want := "zsh:2: failed to load module `zsh/mathfunc': sqrt is not implemented yet\n"
	if got := out + errs; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
