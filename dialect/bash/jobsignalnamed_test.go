// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"regexp"
	"testing"
)

var pidRun = regexp.MustCompile(`[0-9]{4,}`)

// TestAJobASignalEndedIsNamedBySignal pins the `jobs` row for a job a signal
// ended, and the notice a named `wait` writes over one. Measured 2026-10-02
// on bash 5.3.20 (#5389).
func TestAJobASignalEndedIsNamedBySignal(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{"/bin/sleep 1 & /bin/sleep 0.2; kill %1; /bin/sleep 0.2; jobs", "[1]+  Terminated: 15             /bin/sleep 1\n"},
		{"/bin/sleep 1 & /bin/sleep 0.2; kill -HUP %1; /bin/sleep 0.2; jobs", "[1]+  Hangup: 1                  /bin/sleep 1\n"},
		{"{ /bin/sleep 1; } & /bin/sleep 0.2; kill $!; /bin/sleep 0.2; jobs", "[1]+  Terminated: 15             { /bin/sleep 1; }\n"},
		{
			"/bin/sh -c 'kill -USR1 $$' & wait $!; echo st=$?",
			"bash: line 1: PID User defined signal 1: 30  /bin/sh -c 'kill -USR1 $$'\nst=158\n",
		},
		// TERM is the one a named wait leaves unsaid.
		{"/bin/sh -c 'kill -TERM $$' & wait $!; echo st=$?", "st=143\n"},
		// And a job that exited with the same status a signal would leave
		// was not killed, and is not reported.
		{"/bin/sh -c 'exit 158' & wait $!; echo st=$?", "st=158\n"},
	} {
		out, _ := runBash(t, t.TempDir(), tc.src)
		if got := pidRun.ReplaceAllString(out, "PID"); got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
