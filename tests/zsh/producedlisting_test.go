// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"regexp"
	"testing"
)

// How this shell lists a produced parameter, which is a third shape and a
// third *answer* (#2451). Measured 2026-09-12, `env -i PATH=/usr/bin:/bin`
// with a scratch HOME, over a script file, zsh 5.9.2:
//
//	typeset -p RANDOM        typeset -i10 RANDOM=13859
//	typeset -p SECONDS       typeset -i10 SECONDS=0
//	typeset -p LINENO        nothing at all, status 0
//
// The base rides on the letter here where bash writes no base at all, and the
// `LINENO` row is neither a listing nor a refusal — which is why a filter
// could not have produced it and the dialect states it instead.
func TestAProducedParameterListsBack(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{src: "typeset -p RANDOM", want: `^typeset -i10 RANDOM=[0-9]+\n$`},
		{src: "typeset -p SECONDS", want: `^typeset -i10 SECONDS=[0-9]+\n$`},
		{src: "typeset -p LINENO", want: `^$`},
		{src: `typeset -p LINENO; echo "st=$?"`, want: "^st=0\n$"},
	} {
		out, st := runZsh(t, dir, tc.src)
		if !regexp.MustCompile(tc.want).MatchString(out) || st != tc.status {
			t.Errorf("%s = %q at %d, want a match for %q at %d", tc.src, out, st, tc.want, tc.status)
		}
	}
}

// `zsh/datetime`'s parameters are readonly, which is what put them into a
// listing before anything could say how one lists — and a mark meaning
// "cannot be assigned to" was never going to say which type the name has.
// Measured the same day: `typeset -ir EPOCHSECONDS` and
// `typeset -Fr EPOCHREALTIME`, both with no value, where this wrote
// `typeset -r` for each.
func TestTheClockParametersListWithTheirLetters(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{"zmodload zsh/datetime; typeset -p EPOCHSECONDS", "typeset -ir EPOCHSECONDS\n"},
		{"zmodload zsh/datetime; typeset -p EPOCHREALTIME", "typeset -Fr EPOCHREALTIME\n"},
		{"zmodload zsh/datetime; typeset -p epochtime", "typeset -ar epochtime\n"},
	} {
		out, st := runZsh(t, dir, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// And the listing with **no operands** carries the readings, where bash's
// writes the names alone. Measured 2026-09-13, zsh 5.9.2, `env -i
// PATH=/usr/bin:/bin` with a scratch HOME, over a script file, with nothing
// having read the parameters first:
//
//	typeset -p | grep RANDOM    typeset -i10 RANDOM=11798
//	typeset -p | grep SECONDS   typeset -i10 SECONDS=0
//	typeset -p | grep LINENO    nothing
//
// `LINENO` stays out because it is silent to the `-p` word, which is a fact
// of the parameter and not of this listing — so the two rules meet here, and
// the row asserting its absence is what says the bare listing goes through
// the same silence the named form does (#2518).
//
// The reading is re-drawn on each listing, as ksh93's is: two of them a line
// apart hold `RANDOM=22537` and then `RANDOM=21677`. That has to be measured
// with a redirection rather than a pipe, because `typeset -p | grep` forks
// the listing and two forks off one state draw the same number.
func TestABareListingCarriesTheProducedReadings(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, "typeset -p")
	if st != 0 {
		t.Fatalf("typeset -p answered %d, want 0: %q", st, out)
	}
	for _, want := range []string{
		`(?m)^typeset -i10 RANDOM=[0-9]+$`,
		`(?m)^typeset -i10 SECONDS=[0-9]+$`,
	} {
		if !regexp.MustCompile(want).MatchString(out) {
			t.Errorf("typeset -p = %q, want a row matching %q", out, want)
		}
	}
	if regexp.MustCompile(`(?m)^\S.*\bLINENO\b`).MatchString(out) {
		t.Errorf("typeset -p = %q, want no LINENO row — it is silent to `-p`", out)
	}
}
