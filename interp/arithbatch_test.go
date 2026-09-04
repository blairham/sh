// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The base, overflow, empty-expression and consumed-prefix questions, each
// named by its axis and never by a shell.

func arithRun(t *testing.T, src string, set func(*Semantics), dg Diagnostics) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		sem := CoreSemantics()
		if set != nil {
			set(&sem)
		}
		r.Semantics, r.Diagnostics = &sem, &dg
	})
}

func TestBasesRunToSixtyFourWhereAdmitted(t *testing.T) {
	out, st := arithRun(t, `echo $((36#z)) $((64#z)) $((64#Z)) $((64#_))`, func(s *Semantics) {
		s.ArithBaseAbove36 = Yes
	}, Diagnostics{})
	if st != 0 || strings.TrimSpace(out) != "35 35 61 63" {
		t.Errorf("out=%q st=%d, want the full alphabet read", out, st)
	}
	out, st = arithRun(t, `echo $((37#1))`, func(s *Semantics) {
		s.ArithBaseAbove36 = No
	}, Diagnostics{ArithInvalidBase: "invalid base (must be 2 to 36 inclusive): %[1]s"})
	if st == 0 || !strings.Contains(out, "invalid base (must be 2 to 36 inclusive): 37") {
		t.Errorf("out=%q st=%d, want the capped dialect's refusal", out, st)
	}
}

func TestOverflowSaturatesWhereAsked(t *testing.T) {
	out, _ := arithRun(t, `echo $((9223372036854775807 + 1))`, func(s *Semantics) {
		s.ArithOverflowSaturates = Yes
	}, Diagnostics{})
	if strings.TrimSpace(out) != "9223372036854775807" {
		t.Errorf("out=%q, want the clamp at the maximum", out)
	}
	out, _ = arithRun(t, `echo $((-9223372036854775807 - 2))`, func(s *Semantics) {
		s.ArithOverflowSaturates = Yes
	}, Diagnostics{})
	if strings.TrimSpace(out) != "-9223372036854775808" {
		t.Errorf("out=%q, want the clamp at the minimum", out)
	}
	out, _ = arithRun(t, `echo $((9223372036854775807 + 1))`, func(s *Semantics) {
		s.ArithOverflowSaturates = No
	}, Diagnostics{})
	if strings.TrimSpace(out) != "-9223372036854775808" {
		t.Errorf("out=%q, want the wrap", out)
	}
}

func TestAnEmptyExpressionIsAnAxis(t *testing.T) {
	out, st := arithRun(t, `echo $(( ))`, func(s *Semantics) {
		s.EmptyArithExpressionIsAnError = No
	}, Diagnostics{})
	if st != 0 || strings.TrimSpace(out) != "0" {
		t.Errorf("out=%q st=%d, want zero", out, st)
	}
	out, st = arithRun(t, `echo $(( )); echo after`, func(s *Semantics) {
		s.EmptyArithExpressionIsAnError = Yes
	}, Diagnostics{})
	if st == 0 || !strings.Contains(out, "expecting primary") || strings.Contains(out, "after") {
		t.Errorf("out=%q st=%d, want the primary wanted and the script stopped", out, st)
	}
}

func TestTheErrorNamesTheConsumedPrefix(t *testing.T) {
	dg := Diagnostics{
		ArithError:               `%[1]s: %[2]s (error token is "%[3]s")`,
		DigitTooGreatForBase:     "value too great for base",
		ArithErrorNamesThePrefix: true,
	}
	set := func(s *Semantics) { s.ArithLeadingZeroIsOctal = Yes; s.ArithInvalidOctalDigitIsError = Yes }
	out, _ := arithRun(t, `echo $((08+1))`, set, dg)
	if !strings.Contains(out, `08: value too great for base (error token is "08")`) ||
		strings.Contains(out, "08+1:") {
		t.Errorf("out=%q, want the consumed prefix alone", out)
	}
	out, _ = arithRun(t, `echo $((1+08))`, set, dg)
	if !strings.Contains(out, `1+08: value too great for base (error token is "08")`) {
		t.Errorf("out=%q, want everything consumed named", out)
	}
}
