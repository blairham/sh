// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"regexp"
	"testing"
)

// This shell writes `-i` for both of the produced parameters it lists, which
// is where it parts company with the bash 5.3 this tree also models: that one
// writes `-i` for `RANDOM` and no letter at all for `LINENO`. Measured
// 2026-09-12 on ksh93u+ 2012-08-01, `env -i PATH=/usr/bin:/bin` with a
// scratch HOME, over a script file (#2451):
//
//	typeset -p RANDOM    typeset -i RANDOM=7000
//	typeset -p LINENO    typeset -i LINENO=1
//	typeset -p SECONDS   typeset -F 3 SECONDS=0.001
//
// `SECONDS` is deliberately not registered and the row above says why: the
// places after `-F` are a word no listing form here writes (#1461), so the
// choice is between the `not found` it says today and a `typeset -F
// SECONDS=…` that is closer and still not what the shell writes. This test
// pins the two that can be right and leaves the third to the issue that owns
// it — a row asserted as "closer" is a row nobody can grade.
func TestAProducedParameterListsBack(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{"typeset -p RANDOM", `^typeset -i RANDOM=[0-9]+\n$`},
		{"typeset -p LINENO", `^typeset -i LINENO=1\n$`},
	} {
		out, st := runKsh(t, dir, tc.src)
		if !regexp.MustCompile(tc.want).MatchString(out) || st != 0 {
			t.Errorf("%s = %q at %d, want a match for %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// And the listing with **no operands** carries the reading, where bash's
// writes the name alone. Measured 2026-09-13 on ksh93u+ 2012-08-01, `env -i
// PATH=/usr/bin:/bin` with a scratch HOME, over a script file, with nothing
// having read the parameter first:
//
//	typeset -p | grep RANDOM   typeset -i RANDOM=18168
//
// It re-reads the producer to get it, which is observable: two listings a
// line apart hold `RANDOM=32035` and then `RANDOM=1241`. Measure that with a
// redirection and not a pipe — `typeset -p | grep` forks the listing, and two
// forks off one state draw the same number (#2518).
func TestABareListingCarriesTheProducedReading(t *testing.T) {
	dir := t.TempDir()
	out, st := runKsh(t, dir, "typeset -p")
	if want := regexp.MustCompile(`(?m)^typeset -i RANDOM=[0-9]+$`); !want.MatchString(out) || st != 0 {
		t.Errorf("typeset -p = %q at %d, want a `typeset -i RANDOM=<n>` row at 0", out, st)
	}
	// `LINENO` beside it, because a row keyed off `RANDOM` alone would pass
	// on a listing that had reached one producer and stopped.
	if want := regexp.MustCompile(`(?m)^typeset -i LINENO=[0-9]+$`); !want.MatchString(out) {
		t.Errorf("typeset -p = %q, want a `typeset -i LINENO=<n>` row too", out)
	}
}
