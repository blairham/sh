// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "testing"

// TestAWaitLeavesTheJobForTheListing: a job any `wait` reported stays in the
// table until a listing reports it or a new job takes its number, and a
// listing takes each finished row out as it writes it. Measured 2026-10-03 on
// BusyBox v1.37.0 in the pinned image. See interp.Semantics.WaitLeavesTheJobForTheListing,
// BareWaitLeavesJobsForTheListing and JobsListingForgetsEachRowAsItGoes.
func TestAWaitLeavesTheJobForTheListing(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"false & wait %1; echo w=$?; wait %1; echo w=$?; jobs; jobs", "w=1\nw=1\n[1]+  Done(1)                    \n"},
		{"/bin/sleep 0.05 & p=$!; wait $p; jobs; wait $p; echo w=$?", "[1]+  Done                       \nw=127\n"},
		{"( exit 3 ) & wait; jobs", "[1]+  Done(3)                    \n"},
		{"true & true & true & /bin/sleep 0.2; jobs", "[3]+  Done                       \n[2]+  Done                       \n[1]+  Done                       \n"},
		{"/bin/sh -c 'exit 7' & p=$!; wait $p; /bin/sh -c 'exit 4' & wait %1; echo st=$?", "st=4\n"},
		{"true & /bin/sleep 0.1; false & wait %2; /bin/sleep 0.3 & jobs; wait", "[2]+  Running                    \n[1]-  Done                       \n"},
	} {
		if out, _ := run(t, tc.src); out != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
}
