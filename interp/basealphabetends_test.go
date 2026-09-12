// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The bases at the two ends of an output alphabet (#1308).
//
// Neither is an axis. The only dialect that reaches either takes any base in
// silence, and the other one with the attribute refuses everything outside
// two to thirty-six by name — so a field here would have a second value
// nothing could hold. What decides is the alphabet's length and the base's,
// which are already the vector's.

func baseRun(t *testing.T, src string) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		s := testSemantics()
		s.IntegerAttributeTakesABase = Yes
		s.IntegerBaseDigits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		s.IntegerBaseTenIsNoBase = Yes
		// A later declaration over a name that already holds a value reads
		// it again, which the zero row reaches on its way to the base.
		s.AttributeRereadsTheValueItFinds = Yes
		r.Semantics = &s
	})
}

// A base past the end of the alphabet is kept and rendered in ten, with the
// mark still on — so the listing carries the base and the value does not
// carry its digits.
func TestABasePastTheAlphabetRendersInTenWithTheMark(t *testing.T) {
	// The base is *stored*, which the second assignment is what shows: a
	// fallback that forgot it would print a plain 200. The listing's spelling
	// of the base word is the dialect's and is pinned in dialect/ksh.
	out, _ := baseRun(t, `typeset -i65 d=100; echo "[$d]"; d=200; echo "[$d]"`)
	want := "[10#100]\n[10#200]\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	// The last base the alphabet does spell is the control: it renders its
	// own digits, which is what says the fallback is about the end of the
	// alphabet and not about being large.
	out, _ = baseRun(t, `typeset -i36 f=100; echo "[$f]"`)
	if strings.TrimSpace(out) != "[36#2S]" {
		t.Errorf("got %q, want the base spelled", out)
	}
}

// A base below two records nothing — and one and zero are not one rule.
func TestABaseBelowTwoRecordsNothing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a fresh name",
			`typeset -i1 b=5; echo "[$b]"; b=6; echo "[$b]"`,
			"[5]\n[6]\n",
		},
		{
			// One takes the base off a name that has one, the way ten and a
			// bare letter do.
			"one over a base",
			`typeset -i16 a=255; typeset -i1 a; echo "[$a]"; a=16; echo "[$a]"`,
			"[255]\n[16]\n",
		},
		{
			// Zero leaves it exactly where it is, which is the row that says
			// the two are not one rule.
			"zero over a base",
			`typeset -i16 e=255; typeset -i0 e; echo "[$e]"; e=16; echo "[$e]"`,
			"[16#FF]\n[16#10]\n",
		},
		{
			"zero over no base",
			`typeset -i0 c=5; echo "[$c]"; c=6; echo "[$c]"`,
			"[5]\n[6]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := baseRun(t, tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
