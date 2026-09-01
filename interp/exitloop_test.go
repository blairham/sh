// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// `exit` ends the shell from inside a loop as surely as from anywhere else.
//
// It did not. The loop carried on, the next round found the shell refusing to
// run anything, and `while` fell out of its condition and set the status to 0
// — so `exit 3` exited 0. `until` read the same refusal as its condition
// still holding and span forever.
func TestExitLeavesALoop(t *testing.T) {
	for _, tc := range []struct {
		src    string
		status int
	}{
		{`g() { exit 3; }; while :; do g; done; echo after`, 3},
		{`g() { exit 3; }; until false; do g; done; echo after`, 3},
		{`g() { exit 3; }; for i in 1 2 3; do g; done; echo after`, 3},
		{`g() { exit 3; }; for ((;;)); do g; done; echo after`, 3},
		{`while :; do exit 3; done; echo after`, 3},
		// Out through every loop it is inside.
		{`g() { exit 3; }; while :; do while :; do g; done; done; echo after`, 3},
	} {
		out, status := run(t, tc.src, nil)
		if status != tc.status {
			t.Errorf("%s: status %d, want %d", tc.src, status, tc.status)
		}
		if strings.Contains(out, "after") {
			t.Errorf("%s: kept going — %q", tc.src, out)
		}
	}
}

// And the ones that are *not* an exit still behave as they did: `break` stops
// as many loops as it was asked to and no more, `continue` starts the next
// round, and a loop that ends on its own reports 0.
func TestTheOtherLoopControlsAreUnchanged(t *testing.T) {
	for _, tc := range []struct {
		src    string
		out    string
		status int
	}{
		{`while :; do break; done; echo ok`, "ok\n", 0},
		{`for i in 1 2 3; do continue; done; echo ok`, "ok\n", 0},
		{`while :; do while :; do break 2; done; echo inner; done; echo after`, "after\n", 0},
		{`while :; do while :; do break; done; break; done; echo after`, "after\n", 0},
		{`f() { for i in 1 2; do return 5; done; echo no; }; f`, "", 5},
		{`i=0; while [ $i -lt 3 ]; do i=$((i+1)); done; echo $i`, "3\n", 0},
		// A loop that runs no rounds still reports 0 rather than whatever
		// the condition produced.
		{`while false; do echo no; done; echo ok`, "ok\n", 0},
	} {
		out, status := run(t, tc.src, nil)
		if out != tc.out || status != tc.status {
			t.Errorf("%s: got %q status %d, want %q status %d", tc.src, out, status, tc.out, tc.status)
		}
	}
}
