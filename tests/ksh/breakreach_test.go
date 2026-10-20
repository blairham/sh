// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// How far a `break` reaches through a function call and out of `( )`.
//
// A call is a boundary and the parentheses are not, silently in both cases — the pairing that makes the two questions two fields.
//
// Measured against ksh93u+ 2012-08-01, 2026-09-11. Run rather than read off the vector,
// because the two axes meet #1236's question here — a boundary leaves the
// word with no loop at all — and only the run shows which of the two answers
// produced the output.
func TestHowFarABreakReaches(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"through a function call",
			"f(){ break; }; for i in 1 2; do f; echo body; done; echo after",
			"body body after",
		},
		{
			"out of a subshell",
			"for i in 1 2; do ( break; echo insub ); echo body; done; echo after",
			"body body after",
		},
		{
			// The count is clamped at a boundary rather than refused.
			"a count that would cross a call",
			"f(){ for j in 1; do break 2; done; echo infunc; }; for i in 1 2; do f; echo body; done; echo after",
			"infunc body infunc body after",
		},
		{
			"a count that would cross a subshell",
			"for i in 1 2; do ( for j in 1; do break 2; done; echo insub ); echo body; done; echo after",
			"body body after",
		},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.Join(strings.Fields(out), " "); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
