// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An infinity and a NaN stored in a float name. Tests name the wordings and
// never a shell.
//
// The store renders a number and reads the characters back through the name's
// own attribute, and these are the two values whose rendering is a **name**
// rather than a numeral — so the round trip is where they were lost and a
// formatted zero came back. The `1.0/3` rows are the control that says this
// is not "the float store is wrong": an ordinary float is unaffected.
func TestAFloatNameHoldsAnInfinityAndANaN(t *testing.T) {
	dg := Diagnostics{ArithInfinity: "Inf", ArithNotANumber: "NaN", ArithFloatDigits: 17}
	for _, tc := range []struct{ name, src, want string }{
		{"a NaN", `typeset -F 9 f; (( f = 0.0/0.0 )); echo "$f"`, "NaN"},
		{"an infinity", `typeset -F 9 g; (( g = 1.0/0.0 )); echo "$g"`, "Inf"},
		{"a negative infinity", `typeset -F 9 g; (( g = -1.0/0.0 )); echo "$g"`, "-Inf"},
		// The rendering is what is *stored*, so a length counts its
		// characters and neither letter nor precision reaches it.
		{"the rendering is what is stored", `typeset -F 9 g; (( g = 1.0/0.0 )); echo "${#g}"`, "3"},
		{"and the E letter writes the same name", `typeset -E 3 g; (( g = 1.0/0.0 )); echo "$g"`, "Inf"},
		// And the number is still there to be read back, which a formatted
		// zero could not do.
		{"arithmetic reads it back", `typeset -F 9 g; (( g = 1.0/0.0 )); echo "$(( g ))"`, "Inf"},
		{"and it carries into a larger expression", `typeset -F 9 g; (( g = 1.0/0.0 )); echo "$(( g + 1 ))"`, "Inf"},
		{"and into another name", `typeset -F 9 g; (( g = 1.0/0.0 )); (( h = g )); echo "$h"`, "Inf"},
		{"a declaration's own value", `typeset -F 9 g=1.0/0.0; echo "$g"`, "Inf"},
		// The controls: an ordinary float is untouched, at the same
		// precision and through the same route.
		{"an ordinary float", `typeset -F 9 h; (( h = 1.0/3 )); echo "$h"`, "0.333333333"},
		{"and its number is whole", `typeset -F 9 h; (( h = 1.0/3 )); echo "$(( h ))"`, "0.33333333333333331"},
		{"a whole number", `typeset -F 9 h; (( h = 3 )); echo "$h"`, "3.000000000"},
		// And the number travels with an arithmetic store for exactly that
		// store: a *plain* assignment of the same characters the name is
		// holding means those characters, and is read as an expression like
		// any other text.
		{
			"a plain assignment of the standing rendering",
			`typeset -F 1 f=3.14159265358979; f=3.1; echo "$(( f ))"`,
			"3.1000000000000001",
		},
		{
			"and an arithmetic store of the same characters is the number",
			`typeset -F 1 f; (( f = 3.14159265358979 )); echo "$f"; echo "$(( f ))"`,
			"3.1\n3.14159265358979",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := floatRun(t, tc.src, withExponentLetter(FloatFormatExponentWithPlaces), dg)
			if st != 0 || errs != "" {
				t.Fatalf("status %d stderr %q", st, errs)
			}
			if got := strings.TrimSuffix(out, "\n"); got != tc.want {
				t.Errorf("%s gave %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// The name a value that is not a number is written under is the dialect's,
// and it is one table for the expression and for the store — a second copy is
// how the two come to disagree.
func TestTheNameOfAnInfinityIsTheDialectsAndIsShared(t *testing.T) {
	dg := Diagnostics{ArithInfinity: "infinity", ArithNotANumber: "not-a-number", ArithFloatDigits: 17}
	for _, tc := range []struct{ name, src, want string }{
		{"in an expression", `echo "$(( 1.0/0.0 ))"`, "infinity"},
		{"in a float name", `typeset -F 9 g; (( g = 1.0/0.0 )); echo "$g"`, "infinity"},
		{"negated", `typeset -F 9 g; (( g = -1.0/0.0 )); echo "$g"`, "-infinity"},
		{"a NaN in an expression", `echo "$(( 0.0/0.0 ))"`, "not-a-number"},
		{"a NaN in a float name", `typeset -F 9 g; (( g = 0.0/0.0 )); echo "$g"`, "not-a-number"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := floatRun(t, tc.src, withExponentLetter(FloatFormatExponentWithPlaces), dg)
			if st != 0 || errs != "" {
				t.Fatalf("status %d stderr %q", st, errs)
			}
			if got := strings.TrimSuffix(out, "\n"); got != tc.want {
				t.Errorf("%s gave %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// A listing writes those two names in lowercase, where `$name` writes them as
// the dialect spells them, and writes every other value exactly as it stands.
func TestAListingLowercasesAnInfinityAndANaN(t *testing.T) {
	dg := Diagnostics{ArithInfinity: "Inf", ArithNotANumber: "NaN", ArithFloatDigits: 17}
	for _, tc := range []struct{ name, src, want string }{
		{"an infinity", `typeset -F 3 g; (( g = 1.0/0.0 )); typeset -p g`, `declare -- g="inf"`},
		{"a NaN", `typeset -F 3 g; (( g = 0.0/0.0 )); typeset -p g`, `declare -- g="nan"`},
		{"a negative infinity", `typeset -F 3 g; (( g = -1.0/0.0 )); typeset -p g`, `declare -- g="-inf"`},
		{"an ordinary float", `typeset -F 3 g; (( g = 1.5 )); typeset -p g`, `declare -- g="1.500"`},
		// A scalar with no float attribute holding the same letters is left
		// alone, which is what says the fold is the attribute's and not the
		// text's.
		{"a scalar holding the letters", `v=Inf; typeset -p v`, `declare -- v="Inf"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := floatRun(t, tc.src, withExponentLetter(FloatFormatExponentWithPlaces), dg)
			if st != 0 || errs != "" {
				t.Fatalf("status %d stderr %q", st, errs)
			}
			if got := strings.TrimSuffix(out, "\n"); got != tc.want {
				t.Errorf("%s gave %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
