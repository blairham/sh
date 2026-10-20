// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestABackgroundBodyReachesItsParentsProcesslessJob pins that the number a
// job with no process of its own answers to reaches the job from a background
// body, as a process id would, and that such a job killed by a signal reports
// 256 plus the signal (#5684). This shell's `sleep` is a builtin, so each
// `sleep` job here is one of those. Measured 2026-10-03 on ksh93u+ 2012-08-01.
func TestABackgroundBodyReachesItsParentsProcesslessJob(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`sleep 0.3 & p=$!; { kill -0 $p && echo alive; } & wait`, "alive\n"},
		{`sleep 5 & p=$!; { kill $p; } & wait $p 2>/dev/null; echo st=$?`, "st=271\n"},
		{`sleep 5 & p=$!; kill $p; wait $p 2>/dev/null; echo st=$?`, "st=271\n"},
		{`sleep 5 & p=$!; kill -9 $p; wait $p 2>/dev/null; echo st=$?`, "st=265\n"},
		// Ended, the number names nothing; and it is still not a child to wait for.
		{`sleep 0.1 & p=$!; wait; { kill -0 $p 2>/dev/null; echo st=$?; } & wait`, "st=1\n"},
		{`sleep 0.3 & p=$!; { wait $p; echo st=$?; } & wait`, "st=127\n"},
	} {
		got, _ := runKsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
