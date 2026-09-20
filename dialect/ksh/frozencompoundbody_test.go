// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A `readonly` over a name that holds nothing does not refuse the compound
// body that would first give it a value — and refuses one the moment there is
// a value to protect.
//
// Measured 2026-09-20 against AT&T ksh93u+ 2012-08-01 (`/bin/ksh`) from a
// script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on
// the null device. Every `stores` row is status 0 with nothing written and
// every `refused` row is `c: is read only`; this shell refused the first row
// too, which is #3915.
//
// The rows are grouped by what `${c+SET}` answers when the freeze happens,
// because that is the discriminator: `typeset c` leaves the name unset in
// this column and `typeset -C c` does not, and the rows part exactly there.
func TestAValuelessFreezeTakesACompoundBody(t *testing.T) {
	const read = `; print -r -- "[${c.a}]"`
	for _, tc := range []struct{ name, src, want string }{
		{"never mentioned", `readonly c; typeset c=(a=1)` + read, "[1]\n"},
		{"declared and unset", `typeset c; readonly c; typeset c=(a=1)` + read, "[1]\n"},
		{"the letter spelled", `readonly c; typeset -C c=(a=1)` + read, "[1]\n"},
		{"added to", `readonly c; c+=(a=1)` + read, "[1]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The controls, and they are what say this is not a loosening of `readonly`:
// a name that holds anything at all refuses the body, and a body that lands
// somewhere other than the name itself is refused however empty the name is.
//
// Measured in the same run. `c: is read only` in every row, ksh93u+ and here.
//
// `into an element` is a **pin** and not a control: a subscripted operand is
// refused before it reaches this rule at all, so no mutation of the rule can
// move it. Every other row fails when the rule is widened or when the
// set-ness it turns on is ignored.
func TestAFreezeOverAValueRefusesACompoundBody(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a scalar", `c=1; readonly c; typeset c=(a=1)`},
		{"the empty string", `c=; readonly c; typeset c=(a=1)`},
		{"a compound already declared", `typeset -C c; readonly c; typeset c=(a=1)`},
		{"a compound with a body", `typeset -C c=(z=9); readonly c; typeset c=(a=1)`},
		{"a body this rule admitted", `readonly c; typeset c=(a=1); typeset c=(b=2)`},
		{"into an element", `readonly c; typeset c[1]=(a=1)`},
		{"into a table", `readonly m; typeset -A m=(p=1)`},
		{"an array literal", `readonly c; typeset c=(1 2)`},
		{"a scalar assigned", `readonly c; typeset c=1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, "is read only") {
				t.Errorf("= %q, want a readonly refusal", out)
			}
		})
	}
}
