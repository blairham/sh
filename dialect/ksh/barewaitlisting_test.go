// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestABareWaitLeavesItsJobsForTheNextListing pins what ksh93 lists after a
// bare `wait`, and how it names a job a signal ended: the reaped jobs stay
// for one listing, a normally ended one reads `Running` without the monitor,
// and a signaled one reads by its signal (#5687, #5698). Measured
// 2026-10-03 on ksh93u+ 2012-08-01.
func TestABareWaitLeavesItsJobsForTheNextListing(t *testing.T) {
	const run = "[1] +  Running                 <command unknown>\n"
	const term = "[1] + Terminated               <command unknown>\n"
	for _, tc := range []struct{ src, want string }{
		{`(exit 3) & wait; jobs; echo --; jobs`, run + "--\n"},
		{`(exit 3) & wait; wait %1; echo st=$?`, "st=3\n"},
		{`/bin/sleep 5 & kill %1; wait; echo st=$?; jobs`, "st=0\n" + term},
		{`/bin/sleep 5 & kill -9 %1; wait; jobs`, "[1] + Killed                   <command unknown>\n"},
		{`/bin/sleep 5 & kill %1; sleep 0.3; jobs`, term},
		{`/bin/sleep 5 & kill %1; wait %1 2>/dev/null; jobs; echo end`, "end\n"},
		{`(exit 3) & wait; /bin/sleep 0.1; jobs; echo end`, "end\n"},
	} {
		got, _ := runKsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
