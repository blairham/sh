// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runOutputFormat runs src under a grammar that has the output-format
// specifier, with an alphabet to spell a base in.
//
// The alphabet is upper case here and the negative is a sign in front of the
// magnitude, which is one shell's pair of answers rather than the only one —
// the axes are IntegerBaseDigits and IntegerBaseNegativeIsTwosComplement, and
// the tests that move them are the integer attribute's.
func runOutputFormat(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArithOutputFormat = true
	}, func(r *interp.Runner) {
		r.Semantics.IntegerBaseDigits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		r.Semantics.IntegerBaseNegativeIsTwosComplement = interp.No
	})
}

// What the specifier writes the answer as: the base, whether the `base#` is in
// front of it, and where the `_` separators fall.
func TestTheOutputFormatWritesTheAnswer(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a base with its mark", `printf "%s" $(( [#16] 255 ))`, "16#FF"},
		{"a base without it", `printf "%s" $(( [##16] 255 ))`, "FF"},
		{"the smallest base", `printf "%s" $(( [#2] 5 ))`, "2#101"},
		{"the largest base", `printf "%s" $(( [#36] 1295 ))`, "36#ZZ"},
		{"base ten marks nothing", `printf "%s %s" $(( [#10] 255 )) $(( [##10] 255 ))`, "255 255"},
		{"zero under a base", `printf "%s %s" $(( [#16] 0 )) $(( [##16] 0 ))`, "16#0 0"},
		{"a negative keeps its sign outside", `printf "%s %s" $(( [#16] -255 )) $(( [##16] -255 ))`, "-16#FF -FF"},
		{"the whole expression, not the operand beside it", `printf "%s" $(( [#16] 255 + 1 ))`, "16#100"},
		{"nothing to write is zero", `printf "%s" $(( [#16] ))`, "16#0"},
		{"grouping under a base", `printf "%s" $(( [#16_4] 1048575 ))`, "16#F_FFFF"},
		{"grouping and no mark", `printf "%s" $(( [##16_4] 1048575 ))`, "F_FFFF"},
		{"grouping alone is decimal in threes", `printf "%s" $(( [#_] 1234567 ))`, "1_234_567"},
		{"a group size of its own", `printf "%s" $(( [#_5] 1234567 ))`, "12_34567"},
		{"a bare underscore after a base", `printf "%s" $(( [#16_] 1048575 ))`, "16#FF_FFF"},
		{"a group of zero is no grouping", `printf "%s %s" $(( [#16_0] 1048575 )) $(( [#_0] 1234567 ))`, "16#FFFFF 1234567"},
		{"a grouped negative", `printf "%s" $(( [#_] -1234567 ))`, "-1_234_567"},
		{"an explicit base reads in, the format writes out", `printf "%s" $(( [#16] 0x1f ))`, "16#1F"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runOutputFormat(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The specifier decides how the answer is written and never what it is, which
// is what makes it a side-channel rather than arithmetic. The control is the
// same expression read back: `16#FF` is 255 wherever it stands.
func TestTheOutputFormatChangesNoValue(t *testing.T) {
	out, st := runOutputFormat(t, `printf "%s %s" $(( [#16] 255 )) $(( 16#FF ))`)
	if out != "16#FF 255" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "16#FF 255")
	}
}

// It reaches the text an assignment inside the expression stores, not only the
// text the expansion produces — which is what makes the value read back as the
// base it was written in.
func TestTheOutputFormatReachesAnAssignment(t *testing.T) {
	out, st := runOutputFormat(t, `x=5; (( x = [#16] 255 )); printf "[%s][%s]" "$x" "$(( x ))"`)
	if out != "[16#FF][255]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[16#FF][255]")
	}
}

// And it does not reach a subscript, which is the other text an assignment
// writes. The two are told apart by the same expression touching both.
func TestTheOutputFormatDoesNotReachASubscript(t *testing.T) {
	out, st := runOutputFormat(t, `a=(1 2 3); (( [#16] a[2] = 9 )); printf "[%s]" "${a[*]}"`)
	if out != "[1 2 16#9]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[1 2 16#9]")
	}
}

// Nothing survives into the next expression: the format belongs to the
// evaluation that carried it.
func TestTheOutputFormatDoesNotLeak(t *testing.T) {
	out, st := runOutputFormat(t, `printf "%s %s %s" $(( [#16] 255 )) $(( 255 )) $(( [#2] 5 ))`)
	if out != "16#FF 255 2#101" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "16#FF 255 2#101")
	}
}

// The boundary with the integer attribute, which is the other construct that
// writes a base the same way.
//
// A name that already has the attribute does not take its base from an
// expression's format: the format renders an answer, it does not write a
// literal for IntegerBaseComesFromTheValueAssigned to read. The same six
// characters arriving as the text of an ordinary assignment do teach one,
// which is the half that makes this a boundary rather than a rule — without
// it, "the format never teaches a base" would be indistinguishable from the
// axis being off.
func TestTheOutputFormatTeachesAnIntegerNameNothing(t *testing.T) {
	out, st := runGrammar(t,
		`typeset -i i; (( i = [#16] 255 )); echo "[$i]"; typeset -i j; j=$(( [#16] 255 )); echo "[$j]"`,
		func(d *syntax.Dialect) { d.ArithOutputFormat = true },
		func(r *interp.Runner) {
			r.Semantics.IntegerBaseDigits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
			r.Semantics.IntegerBaseNegativeIsTwosComplement = interp.No
			r.Semantics.IntegerAttributeTakesABase = interp.Yes
			r.Semantics.IntegerBaseComesFromTheValueAssigned = interp.Yes
			r.Semantics.IntegerBaseTenIsNoBase = interp.No
		})
	if out != "[255]\n[16#FF]\n" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[255]\n[16#FF]\n")
	}
}

// A base the alphabet cannot spell is refused, with the base quoted back and
// the expansion producing nothing. The range is the alphabet's length rather
// than a constant here, which is why this is an interpreter question and the
// parser takes any number it is given.
func TestABaseOutsideTheAlphabetIsRefused(t *testing.T) {
	for _, src := range []string{
		`printf "[%s]" $(( [#37] 5 ))`,
		`printf "[%s]" $(( [#1] 5 ))`,
		`printf "[%s]" $(( [#0] 5 ))`,
	} {
		out, st := runOutputFormat(t, src)
		if st == 0 || strings.Contains(out, "[5]") {
			t.Errorf("%s = %q (status %d), want the base refused", src, out, st)
		}
		if !strings.Contains(out, "invalid base") {
			t.Errorf("%s = %q, want the base named", src, out)
		}
	}
}

// The base is checked before the expression is evaluated, so an expression
// that would also have failed is still reported against the base. The control
// beside it is the same expression under a base that is fine, which reports
// the division — without it, a check that never ran would pass this too.
func TestTheBaseIsCheckedBeforeTheExpression(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`printf "[%s]" $(( [#37] 1/0 ))`, "invalid base"},
		{`printf "[%s]" $(( [#2] 1/0 ))`, "division by zero"},
	} {
		out, st := runOutputFormat(t, tc.src)
		if st == 0 || !strings.Contains(out, tc.want) {
			t.Errorf("%s = %q (status %d), want %q named", tc.src, out, st, tc.want)
		}
	}
}
