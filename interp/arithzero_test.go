// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// An `(( ))` whose value is zero leaves 1 everywhere; whether that 1 is a
// failure `set -e` and the ERR trap see is Semantics.ArithCommandZeroIsAFailure
// (#3348). Each row is one of that field's measurements.

func arithZeroSem(a Answer) Semantics {
	s := errSem()
	s.ArithCommandZeroIsAFailure = a
	return s
}

func TestAZeroArithmeticCommandIsJudgedByTheAxis(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		// survives under Yes and under No
		yes, no bool
	}{
		{"the command", `(( 0 ))`, false, true},
		{"a comparison", `x=1; (( x == 2 ))`, false, true},
		{"the last operand of a chain", `true && (( 0 ))`, false, true},
		{"a group ending in one", `{ (( 0 )); }`, false, true},
		{"a call whose body ended in one", `f() { (( 0 )); }; f`, false, false},
		{"an eval of one", `eval '(( 0 ))'`, false, false},
		{"a subshell of one", `( (( 0 )) )`, false, false},
		{"let with the same value", `let 0`, false, false},
		{"a test after one in a chain", `(( 0 )) || [[ a == b ]]`, false, false},
		{"a nonzero value", `(( 1 ))`, true, true},
	} {
		for _, a := range []Answer{Yes, No} {
			want := tc.yes
			if a == No {
				want = tc.no
			}
			out, _ := runGrammar(t, "set -e; "+tc.body+"; echo survived", enableTestAndArith, withSem(arithZeroSem(a)))
			if got := out == "survived\n"; got != want {
				t.Errorf("%s under %v: got %q, want survived=%v", tc.name, a, out, want)
			}
		}
	}
}

func TestAZeroArithmeticCommandFiresTheErrTrapByTheAxis(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		yes, no    string
	}{
		{"the command", `(( 0 ))`, "E\ndone\n", "done\n"},
		// Under Yes the body's failure fires and the call refires, which is
		// ErrTrapRefiresForTheCommandItFiredInside; under No only the call.
		{"a call whose body ended in one", `f() { (( 0 )); }; f`, "E\nE\ndone\n", "E\ndone\n"},
	} {
		for _, a := range []Answer{Yes, No} {
			want := tc.yes
			if a == No {
				want = tc.no
			}
			s := arithZeroSem(a)
			s.ErrTrapRunsInsideFunctions = Yes
			if out, _ := runGrammar(t, "trap 'echo E' ERR; "+tc.body+"; echo done", enableTestAndArith, withSem(s)); out != want {
				t.Errorf("%s under %v: got %q, want %q", tc.name, a, out, want)
			}
		}
	}
}

func TestAZeroArithmeticCommandAsksNothingWithNothingToJudge(t *testing.T) {
	s := errSem()
	s.ArithCommandZeroIsAFailure = Unspecified
	if out, st := runGrammar(t, "(( 0 )); echo $?", enableTestAndArith, withSem(s)); out != "1\n" || st != 0 {
		t.Errorf("got %q status %d, want 1 and no refusal", out, st)
	}
}
