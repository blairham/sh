// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"regexp"
	"testing"
)

// TestABareWaitReportsSignalDeathsUnderTheMonitor pins that under `set -m` a
// bare `wait` says, for each job a signal ended, what a `wait` naming it says
// — and nothing for an INT, or with the monitor off (#5687). Through the
// binary, which grants the monitor. Measured 2026-10-03 on ksh93u+
// 2012-08-01.
func TestABareWaitReportsSignalDeathsUnderTheMonitor(t *testing.T) {
	pids := regexp.MustCompile(`[0-9]{3,}`)
	for _, tc := range []struct{ src, out, errs string }{
		{
			"set -m; /bin/sleep 5 & /bin/sleep 5 & kill %1 %2; wait; echo st=$?\n", "st=0\n",
			"s.sh[1]: wait: N: Terminated\ns.sh[1]: wait: N: Terminated\n",
		},
		{"set -m; /bin/sleep 5 & kill -9 %1; wait; echo st=$?\n", "st=0\n", "s.sh[1]: wait: N: Killed\n"},
		{"set -m; /bin/sleep 5 & kill -INT %1; wait; echo st=$?\n", "st=0\n", ""},
		{"/bin/sleep 5 & kill %1; wait; echo st=$?\n", "st=0\n", ""},
	} {
		out, errs, _ := runKshScript(t, tc.src)
		errs = pids.ReplaceAllString(errs, "N")
		if out != tc.out || errs != tc.errs {
			t.Errorf("%s\n got %q %q\nwant %q %q", tc.src, out, errs, tc.out, tc.errs)
		}
	}
}
