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
// Measured on ksh93 with `f1` and `f2` present: `printf '[%s]' $g` is
// `[f1][f2]` and `printf '[%s]' @{1..$g}@` is the single `[@{1..f*}@]`. zsh
// agrees on the second and cannot testify to the first, since it does not
// glob an expansion's result at all.
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
	const setup = `: > f1; : > f2; g='f*'; `
	if got := run(setup + `printf '[%s]' $g`); got != "[f1][f2]" {
		t.Errorf("got %q, want the value matched in an ordinary word", got)
	}
	if got := run(setup + `printf '[%s]' @{1..$g}@`); got != "[@{1..f*}@]" {
		t.Errorf("got %q, want the failed range left as text", got)
	}
}
