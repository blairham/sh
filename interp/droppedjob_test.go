// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// droppedJobs runs src with finished jobs leaving the table, a missing job
// spec answered in silence, and the memory of a reported job set to remember.
func droppedJobs(t *testing.T, remember Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		s := *r.Semantics
		s.FinishedJobLeavesTheTable = Yes
		s.WaitReportsAMissingJob = No
		s.WaitRemembersAReapedJob = remember
		r.Semantics = &s
	})
}

// **A dropped job's process id answers once, whatever the memory of a
// reported job says, and then nothing** — and a listing and an id lookup both
// let the table go first. See Runner.dropped.
func TestADroppedJobAnswersItsIdOnce(t *testing.T) {
	const src = `(exit 4) & p=$!; /bin/sleep 0.2; jobs; wait $p; echo a=$?; wait $p; echo b=$?
(exit 5) & /bin/sleep 0.2; jobs; wait %1; echo spec=$?
(exit 6) & q=$!; /bin/sleep 0.2; wait $q; echo c=$?; wait $q; echo d=$?`
	for _, remember := range []Answer{Yes, No} {
		out, st := droppedJobs(t, remember, src)
		if !strings.HasSuffix(out, "a=4\nb=127\nspec=0\nc=6\nd=127\n") || strings.Contains(out, "exit") || st != 0 {
			t.Errorf("remember %v: %q (status %d), want nothing listed, a=4, b=127 and the silent miss", remember, out, st)
		}
	}
}

// **The monitor keeps a finished job until something reports it**, with or
// without a terminal.
func TestTheMonitorKeepsAFinishedJob(t *testing.T) {
	out, st := droppedJobs(t, Yes, `set -m; (exit 3) & /bin/sleep 0.2; wait %1; echo st=$?`)
	if !strings.HasSuffix(out, "st=3\n") || st != 0 {
		t.Errorf("got %q (status %d), want st=3", out, st)
	}
	// And a listing reports it rather than letting it go unseen.
	out, st = droppedJobs(t, Yes, `set -m; (exit 3) & /bin/sleep 0.2; jobs`)
	if !strings.Contains(out, "exit 3") || st != 0 {
		t.Errorf("listing under the monitor: %q (status %d), want the finished job in it", out, st)
	}
}
