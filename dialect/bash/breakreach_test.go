// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// How far a `break` reaches through a function call and out of `( )`.
//
// Both are boundaries here, and the word says so each time it crosses one — which is bash 3.2 answering the opposite way on both, so neither is the core's.
//
// Measured against bash 5.3.15, 2026-09-11. Run rather than read off the vector,
// because the two axes meet #1236's question here — a boundary leaves the
// word with no loop at all — and only the run shows which of the two answers
// produced the output.
func TestHowFarABreakReaches(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"through a function call",
			"f(){ break; }; for i in 1 2; do f; echo body; done; echo after",
			"sh: line 1: break: only meaningful in a `for', `while', or `until' loop body sh: line 1: break: only meaningful in a `for', `while', or `until' loop body after",
		},
		{
			"out of a subshell",
			"for i in 1 2; do ( break; echo insub ); echo body; done; echo after",
			"sh: line 1: break: only meaningful in a `for', `while', or `until' loop insub body sh: line 1: break: only meaningful in a `for', `while', or `until' loop insub body after",
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
			"insub body insub body after",
		},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.Join(strings.Fields(out), " "); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
