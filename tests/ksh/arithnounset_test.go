// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// `set -u` reaches arithmetic here, and it is the wider of the two refusals
// this dialect already makes about a name in an expression: with the option
// off, only a name inside a subscript or one reached through another name's
// value is refused, and with it on every name is.
//
// Measured 2026-09-18 against ksh93u+ 2012-08-01, `env -i HOME=…
// PATH=/usr/bin:/bin LC_ALL=C` from a script file, with `b` never set (#3574).
func TestTheOptionReachesEveryArithmeticReadOfAName(t *testing.T) {
	for _, src := range []string{
		`set -u; : $((b)); echo OK`,
		`set -u; a=1; : $((a+b)); echo OK`,
		`set -u; for ((i=0;i<b;i++)); do :; done; echo OK`,
	} {
		out, st := answersRun(t, src)
		if st == 0 || strings.Contains(out, "OK") {
			t.Errorf("%s = %q status %d, want a refusal and no OK", src, out, st)
		}
		if want := "b: parameter not set"; !strings.Contains(out, want) {
			t.Errorf("%s = %q, want it to name %q", src, out, want)
		}
	}
	// With the option off a bare name at the top of the expression is still
	// zero here, which is what says the option is what widened the refusal.
	if out, st := answersRun(t, `printf "[%s]" "$((b))"`); out != "[0]" || st != 0 {
		t.Errorf("nounset off = %q status %d, want [0] at 0", out, st)
	}
}

// The refusal is the expression's failure rather than the shell's, so the
// construct answers — and here that parts `(( ))` from `let`, because this is
// the one dialect where a failed `(( ))` ends the input and a failed `let`
// does not.
//
// Measured 2026-09-18, ksh93u+ 2012-08-01: `set -u; (( b ))` stops the script
// at 1, and `set -u; let "x=b"` writes `let: b: parameter not set`, leaves 1
// and runs on (#3574).
func TestTheConstructAnswersForTheArithmeticNounsetRefusal(t *testing.T) {
	if out, st := answersRun(t, `set -u; (( b )); echo OK`); st == 0 || strings.Contains(out, "OK") {
		t.Errorf("(( b )) = %q status %d, want the input ended", out, st)
	}
	out, st := answersRun(t, `set -u; let "x=b"; printf "[%s]" "$?"`)
	if st != 0 || !strings.Contains(out, "[1]") {
		t.Errorf(`let "x=b" = %q status %d, want [1] and the line running on`, out, st)
	}
	if want := "let:"; !strings.Contains(out, want) {
		t.Errorf("= %q, want the builtin to name itself with %q", out, want)
	}
}

// The two axes.
func TestTheArithmeticNounsetAxes(t *testing.T) {
	s := ksh.Semantics()
	if got := s.ArithUnsetNameUnderNounsetIsRefused; got != interp.Yes {
		t.Errorf("ArithUnsetNameUnderNounsetIsRefused = %v, want yes", got)
	}
	if got := s.ArithNounsetRefusalIsFatal; got != interp.No {
		t.Errorf("ArithNounsetRefusalIsFatal = %v, want no", got)
	}
}
