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
