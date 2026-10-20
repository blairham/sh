// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A brace range reads its endpoints after the expansions written in them —
// this shell's answer, and zsh's, against bash's literal `{1..3}` (#1679).
func TestABraceRangeReadsItsEndpointsAfterExpanding(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`n=3; echo {1..$n}`, "1 2 3"},
		{`a=1; b=4; echo {$a..$b}`, "1 2 3 4"},
		{`n=3; echo {1..'3'}`, "1 2 3"},
		{`echo {1..3}`, "1 2 3"},
		{`q=abc; echo {1..$q}`, "{1..abc}"},
	} {
		out, st := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want || st != 0 {
			t.Errorf("%s:\n  said %q status %d\n  want %q", tc.src, got, st, tc.want)
		}
	}
}

// The text a failed range leaves behind is not matched against the
// filesystem, and this is the shell that can say so: it globs an expansion's
// result in an ordinary word, and still leaves `{1..$g}` alone.
//
// The probe needs files the failed text would actually match, which is the
// discrimination the first attempt at it lacked: with `f1` and `f2` present
// the text `@{1..f*}@` matches nothing either way, so a glob and a refusal to
// glob look identical and the mutation that unquoted the span survived.
// Measured on ksh93 with `{1..a}` and `{1..b}` present and `g='*'`:
// `printf '[%s]' $g` lists both names and `printf '[%s]' {1..$g}` is the
// single field `[{1..*}]`.
//
// A redirection is the same claim from the other side. Its target is
// expanded once and then counted, and counting it with the endpoints live
// would run the substitution again — ksh93 prints `ran` once and creates one
// file, named `{1..2}`.
func TestAFailedExpandedRangeIsNotAPattern(t *testing.T) {
	run := func(src string) string {
		t.Helper()
		out, st, err := preset.Combined(t, dialecttest.Base{
			Name: "sh", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
		}, src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if st != 0 {
			t.Errorf("%s: status %d, output %q", src, st, out)
		}
		return out
	}
	const setup = `: > '{1..a}'; : > '{1..b}'; g='*'; `
	if got := run(setup + `printf '[%s]' $g`); got != "[{1..a}][{1..b}]" {
		t.Errorf("got %q, want the value matched in an ordinary word", got)
	}
	if got := run(setup + `printf '[%s]' {1..$g}`); got != "[{1..*}]" {
		t.Errorf("got %q, want the failed range left as text rather than matched", got)
	}
	got := run(`f() { echo ran >&2; echo 2; }; echo hi > {1..$(f)}; echo "st=$?"; ` +
		`for n in *; do printf '<%s>' "$n"; done`)
	if got != "ran\nst=0\n<{1..2}>" {
		t.Errorf("got %q, want the target expanded once and named literally", got)
	}
}
