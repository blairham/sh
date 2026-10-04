// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// TestAStrictMaskedStatusRefusesASignAndMasksTheRest: digits only, as the
// strict reading has it, and eight bits of what was read.
func TestAStrictMaskedStatusRefusesASignAndMasksTheRest(t *testing.T) {
	s := permissive()
	s.StatusArgument = StatusArgStrictMasked
	s.BadOptionToSpecialBuiltinFatal = No
	for _, tc := range []struct{ op, want string }{
		{"300", "st=44"},
		{"256", "st=0"},
		{"-1", "st=2"},
	} {
		out, _ := run(t, "f() { return "+tc.op+"; }\nf\necho \"st=$?\"\n", withSem(s))
		if !strings.Contains(out, tc.want) {
			t.Errorf("return %s: said %q, want %q", tc.op, out, tc.want)
		}
	}
}

// TestASignedShiftCountIsANumberOrNot: `shift +1` shifts one where a sign is
// read, and is not a number where it is not; `-0` is the sharp case, since
// it is no negative count for the out-of-range rule to catch.
func TestASignedShiftCountIsANumberOrNot(t *testing.T) {
	for _, tc := range []struct {
		signed Answer
		op     string
		want   string
	}{
		{Yes, "+1", "st=0 n=2"},
		{Yes, "-0", "st=0 n=3"},
		{No, "+1", "st=2 n=3"},
		{No, "-0", "st=2 n=3"},
	} {
		s := permissive()
		s.ShiftCountTakesASign = tc.signed
		s.ShiftNegativeIsOutOfRange = No
		s.ShiftOptionWords = ShiftOptionWordsNone
		s.BadOptionToSpecialBuiltinFatal = No
		out, _ := run(t, "set -- a b c; shift "+tc.op+"; echo \"st=$? n=$#\"\n", withSem(s))
		if !strings.Contains(out, tc.want) {
			t.Errorf("signed=%v shift %s: said %q, want %q", tc.signed, tc.op, out, tc.want)
		}
	}
	// And an unsigned count asks nothing.
	s := permissive()
	s.ShiftCountTakesASign = Unspecified
	if out, _ := run(t, "set -- a b c; shift 2; echo \"n=$#\"\n", withSem(s)); !strings.Contains(out, "n=1") {
		t.Errorf("shift 2 with the axis unanswered: said %q", out)
	}
}

// TestALetFailureCostsTheDialectsStatus: a division by zero is 1 unless the
// dialect says otherwise.
func TestALetFailureCostsTheDialectsStatus(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   string
	}{{0, "st=1"}, {2, "st=2"}} {
		out, _ := run(t, "let '1/0'; echo \"st=$?\"\n", func(r *Runner) {
			s := permissive()
			r.Semantics = &s
			r.Diagnostics = &Diagnostics{LetFailureStatus: tc.status}
		})
		if !strings.Contains(out, tc.want) {
			t.Errorf("LetFailureStatus %d: said %q, want %q", tc.status, out, tc.want)
		}
	}
}
