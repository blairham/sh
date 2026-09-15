// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// This is the column that says a width written out and a width taken from a
// `*` are two questions rather than one.
//
// Measured 2026-09-15 on dash, `env -i PATH=/usr/bin:/bin` with a scratch
// HOME. The literal spelling is refused, in dash's own words and at its own
// status:
//
//	$ dash -c 'printf "A[%21474836470s]B" x; echo "|st=$?"'
//	dash: 1: printf: xvsnprintf failed
//	A[|st=2
//
// and the same number arriving through a star is wrapped into the int in
// silence — 21474836470 is -10 as an int32, and a negative width is a
// left-justified one:
//
//	$ dash -c 'printf "A[%*s]B" 21474836470 x; echo "|st=$?"'
//	A[x         ]B|st=0
//
// A single answer over both routes would have to be wrong about this shell
// once, which is why Semantics.PrintfFieldBeyondAnInt and
// Semantics.PrintfStarBeyondAnInt are two axes.
func TestTheTwoWidthRoutesPartHere(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`printf 'A[%21474836470s]B' x; echo "|st=$?"`, "A[|st=2"},
		{`printf 'A[%.21474836470f]B' 1; echo "|st=$?"`, "A[|st=2"},
		{`printf 'A[%*s]B' 21474836470 x; echo "|st=$?"`, "A[x         ]B|st=0"},
		{`printf 'A[%*s]B' 4294967306 x; echo "|st=$?"`, "A[         x]B|st=0"},
		// A wrapped-negative precision is C's "as if it were omitted", so
		// the default six places stand.
		{`printf 'A[%.*f]B' 21474836470 1; echo "|st=$?"`, "A[1.000000]B|st=0"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); !strings.HasSuffix(got, tc.want) {
			t.Errorf("%s: said %q, want it to end %q", tc.src, got, tc.want)
		}
	}
}
