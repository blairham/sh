// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A width past the C `int` it is stored in is refused here, and the refusal
// ends the builtin where the conversion stood.
//
// Measured 2026-09-15 on bash 5.3.15, `env -i PATH=/usr/bin:/bin` with a
// scratch HOME:
//
//	$ bash -c 'printf "A[%21474836470s]B" x; echo "|st=$?"'
//	bash: line 1: printf: Value too large to be stored in data type
//	A[|st=1
//
// The text already written stays, so it is the pass stopping rather than the
// output being taken back. The boundary is INT_MAX *itself* — `%2147483646s`
// is two billion characters here and `%2147483647s` is the complaint — which
// is one below where BusyBox ash turns.
//
// This is the axis #3008 was filed against: we honored the width instead, and
// 21 GB of padding is a hang rather than a slow answer.
func TestAWidthBeyondAnIntIsRefusedHere(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`printf 'A[%21474836470s]B' x; echo "|st=$?"`, "A[|st=1"},
		{`printf 'A[%4294967306s]B' x; echo "|st=$?"`, "A[|st=1"},
		{`printf 'A[%2147483647s]B' x; echo "|st=$?"`, "A[|st=1"},
		// A precision is refused in the same words.
		{`printf 'A[%.21474836470f]B' 1; echo "|st=$?"`, "A[|st=1"},
		// The format is not reused past the refusal: one operand's worth of
		// output and then the builtin is over.
		{`printf '[%21474836470s]' a b; echo "|st=$?"`, "[|st=1"},
		// Just inside the edge is not refused. Asserted on the scanner in
		// interp rather than here, because the only spelling that could ask
		// it from outside is a width that lays two billion columns out — and
		// a precision in that band is #3017, a separate defect this axis
		// does not reach.
	} {
		out, _ := answersRun(t, tc.src)
		got := strings.TrimSpace(out)
		if !strings.HasSuffix(got, tc.want) {
			t.Errorf("%s: said %q, want it to end %q", tc.src, got, tc.want)
		}
	}
}

// The same number arriving through a `*` is a different complaint, and the
// field is still laid out.
//
// Measured 2026-09-15 on bash 5.3.15:
//
//	$ bash -c 'printf "A[%*s]B" 21474836470 x; echo "|st=$?"'
//	bash: line 1: printf: 21474836470: Result too large
//	A[x]B|st=1
//
// So the complaint is about the operand's *range* rather than the field's,
// and where the literal spelling stops the builtin at `A[`, this one writes
// `A[x]B` whole and reports 1. An operand of exactly INT_MAX is the field's
// complaint instead, since that value fits the int and the width check is
// what turns it away.
func TestAStarOperandBeyondAnIntIsOutOfRangeHere(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`printf 'A[%*s]B' 21474836470 x; echo "|st=$?"`, "A[x]B|st=1"},
		{`printf 'A[%*s]B' -21474836470 x; echo "|st=$?"`, "A[x]B|st=1"},
		{`printf 'A[%.*f]B' 21474836470 1; echo "|st=$?"`, "A[1.000000]B|st=1"},
		// At the edge it is the width that refuses, and the builtin stops.
		{`printf 'A[%*s]B' 2147483647 x; echo "|st=$?"`, "A[|st=1"},
	} {
		out, _ := answersRun(t, tc.src)
		got := strings.TrimSpace(out)
		if !strings.HasSuffix(got, tc.want) {
			t.Errorf("%s: said %q, want it to end %q", tc.src, got, tc.want)
		}
	}
}
