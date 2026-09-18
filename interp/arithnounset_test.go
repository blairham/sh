// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// arithNounset sets the two axes `set -u` in an expression is decided by.
func arithNounset(refused, fatal Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.ArithUnsetNameUnderNounsetIsRefused = refused
		s.ArithNounsetRefusalIsFatal = fatal
		r.Semantics = &s
	}
}

// Whether `set -u` reaches a name an expression reads is an axis, and the
// expression is zero at the other value.
//
// Every route into an arithmetic read of a name is here, because the axis is a
// seam rather than a row: they all go through one lookup, so a fix that
// reached only the expansion would pass a test written on `$(( ))` alone.
func TestAnUnsetNameInAnExpressionUnderNounset(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"an expansion", `set -u; echo $((b)); echo OK`},
		{"an operand of one", `set -u; a=1; echo $((a+b)); echo OK`},
		{"an array subscript", `set -u; a=(x); echo "${a[b]}"; echo OK`},
		{"a length over one", `set -u; a=(x); echo ${#a[b]}; echo OK`},
		{"an arithmetic command", `set -u; (( b )); echo OK`},
		{"a C-style for header", `set -u; for ((i=0;i<b;i++)); do :; done; echo OK`},
		{"a let operand", `set -u; let "x=b"; echo OK`},
		{"an integer declaration's value", `set -u; typeset -i n; n=b; echo OK`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, arithNounset(Yes, Yes))
			if st == 0 || strings.Contains(out, "OK") {
				t.Errorf("refused = %q status %d, want a refusal and no OK", out, st)
			}
			if want := "b: "; !strings.Contains(out, want) {
				t.Errorf("refused = %q, want it to name %q and not the name the brackets hang off", out, want)
			}
			if out, st := run(t, tc.src, arithNounset(No, No)); !strings.Contains(out, "OK") || st != 0 {
				t.Errorf("allowed = %q status %d, want it to carry on at 0", out, st)
			}
		})
	}
}

// It is **unset** and not empty, at either value of the axis — which is what
// makes it a lookup rule rather than a reading of the text.
func TestAnEmptyNameInAnExpressionIsZeroUnderNounset(t *testing.T) {
	for _, a := range []Answer{Yes, No} {
		src := `set -u; b=; echo $((b+7)); echo OK`
		out, st := run(t, src, arithNounset(a, a))
		if strings.TrimSpace(out) != "7\nOK" || st != 0 {
			t.Errorf("at %v = %q status %d, want 7 then OK at 0", a, out, st)
		}
	}
}

// And with the option off the refusal is never raised, whatever the axis says
// — so the axis widens `set -u` rather than changing what arithmetic reads.
func TestTheExpressionIsZeroWithTheOptionOff(t *testing.T) {
	out, st := run(t, `echo $((b+7)); echo OK`, arithNounset(Yes, Yes))
	if strings.TrimSpace(out) != "7\nOK" || st != 0 {
		t.Errorf("= %q status %d, want 7 then OK at 0", out, st)
	}
}

// Whether the refusal stops the shell wherever it was written is the second
// axis, and the two constructs that answer for their own failures are where it
// shows: at Yes the shell is gone, at No the construct's status is left behind
// and the next command runs.
func TestWhetherTheArithmeticNounsetRefusalIsFatal(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"an arithmetic command", `set -u; (( b )); echo OK`},
		{"a let operand", `set -u; let "x=b"; echo OK`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := run(t, tc.src, arithNounset(Yes, Yes)); strings.Contains(out, "OK") || st == 0 {
				t.Errorf("fatal = %q status %d, want the shell stopped", out, st)
			}
			out, st := run(t, tc.src, arithNounset(Yes, No))
			if !strings.Contains(out, "OK") || st != 0 {
				t.Errorf("not fatal = %q status %d, want the line to run on", out, st)
			}
			if !strings.Contains(out, "b: ") {
				t.Errorf("not fatal = %q, want the refusal still written", out)
			}
		})
	}
	// A word expansion is fatal at either value, because what ends there is
	// the command rather than the shell being asked to carry on.
	for _, a := range []Answer{Yes, No} {
		if out, st := run(t, `set -u; x=$((b)); echo OK`, arithNounset(Yes, a)); strings.Contains(out, "OK") || st == 0 {
			t.Errorf("a word expansion at %v = %q status %d, want it stopped", a, out, st)
		}
	}
}
