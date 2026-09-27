// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A redirection target the braces fanned is refused by the reading that
// expands a target as an ordinary word — `> {a,b}` names two files and so
// names none — and that reading walks the names a **second** time on the way
// to refusing them.
//
// Measured 2026-09-26 under `env -i PATH=/usr/bin:/bin` with a scratch HOME,
// each case in a directory of its own, on bash 5.3.20 and bash 3.2.57 alike —
// `go version -m` says *not a Go executable* for both. The instrument counts
// **runs**, as a line a substitution appends to a file of its own, rather than
// what reached standard output: a refused redirection produces no file and no
// output, so counting what survived would count nothing however many times the
// word ran.
//
//	: > {x}$(f)          1 name, not refused   1 run
//	: > {x,y}$(f)        2 names               4 runs
//	: > {x,y,w}$(f)      3 names               6 runs
//	: > {a,b}{c,d}$(f)   4 names               8 runs
//
// Two per name, and the single name is the control that says it is the
// refusal's road rather than the fan's: an argument's fan runs the word once
// per name in that column already — Semantics.BraceFanExpandsEachNameOnItsOwn
// — and `echo {x,y}$(f)` is 2 there and here (#4705).

// refusingFanSemantics is fanSemantics at the reading that refuses a fanned
// target, which is the one thing that separates these rows from #4694's.
func refusingFanSemantics(t *testing.T) func(*Runner) {
	t.Helper()
	base := fanSemantics(t, Yes)
	return func(r *Runner) {
		base(r)
		sem := *r.Semantics
		sem.RedirectTargetIsAnOrdinaryWord = Yes
		sem.RedirectTargetFailureIsTheRedirections = No
		r.Semantics = &sem
	}
}

func TestARefusedFanRunsTheWordTwicePerName(t *testing.T) {
	for _, tc := range []struct {
		target string
		runs   int
	}{
		{"{x,y}", 4},
		{"{x,y,w}", 6},
		{"{a,b}{c,d}", 8},
	} {
		t.Run(tc.target, func(t *testing.T) {
			out, st := run(t, ": > "+tc.target+tickSubst+"\n", refusingFanSemantics(t))
			if n := countTicks(out); n != tc.runs {
				t.Errorf("out = %q: %d runs, want %d", out, n, tc.runs)
			}
			if !strings.Contains(out, "ambiguous redirect") || st != 1 {
				t.Errorf("out = %q status %d, want the refusal at 1", out, st)
			}
		})
	}
}

// The single name is the control, and it is the one that fails if the second
// walk is made unconditionally: one name is not refused, so the target opens
// and the word runs once.
func TestAFanOfOneNameIsNotRefusedAndRunsOnce(t *testing.T) {
	out, st := run(t, ": > {x}"+tickSubst+"\n", refusingFanSemantics(t))
	if n := countTicks(out); n != 1 || st != 0 {
		t.Errorf("out = %q status %d: %d runs, want 1 at status 0", out, st, n)
	}
	if strings.Contains(out, "ambiguous redirect") {
		t.Errorf("out = %q, want no refusal for a single name", out)
	}
}

// And the second walk is never made over a value that would not expand: a
// failed expansion is **one** sentence, and a second walk would write the same
// complaint twice.
func TestARefusedFanWithAFailedValueIsOneSentence(t *testing.T) {
	out, _ := run(t, ": > {x,y}$((1/0))\n", refusingFanSemantics(t))
	if n := strings.Count(out, "division"); n != 1 {
		t.Errorf("out = %q: %d sentences, want 1", out, n)
	}
	if strings.Contains(out, "ambiguous redirect") {
		t.Errorf("out = %q, want the failure alone and no count complaint", out)
	}
}

// The road, held fixed. The reading that takes a target as a **list of names**
// opens the files rather than refusing them, and walks the fan once — so the
// second walk is the refusal's and not the fan's.
func TestAFanTheReadingAcceptsIsWalkedOnce(t *testing.T) {
	out, st := run(t, ": > {x,y}"+tickSubst+"\n", fanSemantics(t, Yes))
	if n := countTicks(out); n != 2 || st != 0 {
		t.Errorf("out = %q status %d: %d runs, want 2 at status 0", out, st, n)
	}
}
