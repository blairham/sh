// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **Two job-marker rows** (#5349). Measured 2026-10-02 on zsh 5.9.2 under
// `-f -c`, byte for byte.
func TestTheMarkersAJobLeavesBehind(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a job leaving inside an eval sends no - outward",
			`f() { eval "$1" }; f '/bin/sleep 0 & wait'; f2() { wait %- 2>&1 }; f2`,
			"f2:wait: no previous job\n",
		},
		{
			"and at the top",
			`f() { eval "$1" }; f '/bin/sleep 0 & wait'; jobs %- 2>&1`,
			"zsh:jobs:1: no previous job\n",
		},
		{
			"control: the - on a number an eval holds",
			`f() { eval "$1" }; f '/bin/sleep 0 & wait'; g() { eval 'wait %- 2>&1' }; g`,
			"(eval):wait:1: %-: no such job\n",
		},
		{
			"the - on a number a nested eval holds again",
			`f() { eval "$1" }; f '/bin/sleep 0 & wait'; { eval 'jobs %- 2>&1' }`,
			"(eval):jobs:1: %-: no such job\n",
		},
		{
			"the - on a number a nested if holds again",
			`f() { eval "$1" }; f '/bin/sleep 0 & wait'; if :; then if :; then jobs %- 2>&1; fi; fi`,
			"zsh:jobs:1: %-: no such job\n",
		},
		{
			"one if holds only the first number",
			`f() { eval "$1" }; f '/bin/sleep 0 & wait'; if :; then jobs %- 2>&1; jobs %% 2>&1; fi`,
			"zsh:jobs:1: no previous job\nzsh:jobs:1: %%: no such job\n",
		},
		{
			"control: a brace group's number",
			`/bin/sleep 1 & { /bin/sleep 1 & }; jobs %- 2>&1; kill %1 %3`,
			"zsh:jobs:1: %-: no such job\n",
		},
		{
			// Job 2 outlives job 1 by construction rather than by luck: it
			// polls job 1's pid until the shell has reaped it. Two bare
			// `sleep 0`s left the order to the scheduler, and on a loaded
			// runner job 1 was sometimes still running when the wait for job
			// 2 returned — in this shell and in zsh alike, which is no
			// difference to measure (#5854). zsh 5.9.2 under `-f -c` writes
			// the same two lines for this shape.
			"a wait notices every job that ended",
			`/bin/sleep 0 & /bin/sh -c 'while kill -0 $1 2>/dev/null; do /bin/sleep 0.01; done' - $! & wait $!; wait %% 2>&1; wait %- 2>&1`,
			"zsh:wait:1: no current job\nzsh:wait:1: no previous job\n",
		},
		{
			"control: one still running",
			`/bin/sleep 1 & /bin/sleep 0 & wait $!; jobs; kill %1`,
			"[1]  + running    /bin/sleep 1\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := runZshC(c.src); got != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, got, c.want)
			}
		})
	}
}
