// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A subscript written on a name that holds nothing is still arithmetic, and
// it is evaluated **once**. Two reads of it — one for the refusal a subscript
// that will not evaluate earns, one for whether the position falls before the
// first element — each ran it, so `i=0; : ${x[i++]}; echo $i` was 2 wherever
// the reach is answered at all. Measured 2026-10-31 under `-c`, it is 1 in bash
// 5.3.20, bash 3.2.57, ksh93u+ and zsh 5.9.2, as it is here once the name holds
// anything (#7189).
func TestASubscriptOnANameHoldingNothingIsEvaluatedOnce(t *testing.T) {
	t.Parallel()
	policies := []struct {
		name string
		p    SubscriptBeforeStartPolicy
	}{
		{"the reach says nothing", SubscriptBeforeStartIsNothing},
		{"the reach is reported", SubscriptBeforeStartIsReported},
		{"the reach ends the script", SubscriptBeforeStartEndsTheScript},
	}
	rows := []string{
		`i=0; : ${x[i++]}; echo "i=$i"`,
		`i=0; echo "[${x[i++]}]"; echo "i=$i"`,
		`i=0; : ${x[i++]:-d}; echo "i=$i"`,
		`i=0; : ${x[i++]+a}; echo "i=$i"`,
		`i=0; : "${x[i++]-a}"; echo "i=$i"`,
		// The controls: a name holding anything was already read once.
		`x=(); i=0; : ${x[i++]}; echo "i=$i"`,
		`x=s; i=0; : ${x[i++]}; echo "i=$i"`,
	}
	for _, pc := range policies {
		for _, needs := range []Answer{Yes, No} {
			for _, src := range rows {
				out, _ := runBeforeStart(t, pc.p, needs, src)
				if !strings.HasSuffix(out, "i=1\n") {
					t.Errorf("%s, needs %v: %s\n got %q, want the subscript run once (i=1)",
						pc.name, needs, src, out)
				}
			}
		}
	}
}

// The position the one evaluation came to is the one the reach is asked
// about: a negative subscript that runs a side effect is reported once, and
// the side effect happens once (#7189).
func TestTheReachOnANameHoldingNothingIsAskedOfTheOneEvaluation(t *testing.T) {
	const src = `i=0; echo "[${x[-1+0*i++]}]"; echo "i=$i"`
	out, st := runBeforeStart(t, SubscriptBeforeStartIsReported, No, src)
	if st != 0 || !strings.Contains(out, "[]\n") || !strings.HasSuffix(out, "i=1\n") {
		t.Errorf("got %q (status %d), want the read empty and i=1", out, st)
	}
	if n := strings.Count(out, "subscript"); n != 1 {
		t.Errorf("got %q: the reach reported %d times, want once", out, n)
	}
	// The pair with nothing before its comma is the one shape the first read
	// does not evaluate, and the second still does: it is refused, once.
	out, _ = runBeforeStart(t, SubscriptBeforeStartIsReported, No, `i=0; : ${x[,i++]}; echo "i=$i"`)
	if strings.Contains(out, "i=") {
		t.Errorf("got %q, want `,i++` refused as arithmetic", out)
	}
}
