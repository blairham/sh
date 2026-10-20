// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// **A finished background job leaves the table once the shell has reaped a
// foreground child, and a job spec naming it is then the silent miss** — see
// interp.Semantics.FinishedJobLeavesTheTable and #5302. Measured against
// ksh93u+ 2012-08-01 with this script as a file, which writes these lines byte
// for byte: `%%` and `%1` answer 0 in silence and `jobs` lists nothing, the
// job's process id answers its status once and then 127, the builtin `sleep`
// reaps nothing and leaves the job there, a subshell notices for itself, and
// the monitor keeps the job until something reports it. And a job sleeping in
// the builtin is started as far as it will get, so `&` does not wait it out.
func TestAFinishedJobLeavesTheTable(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `(exit 4) &
/bin/sleep 0.3
wait %%; echo "cur=$?"
(exit 3) & p=$!
/bin/sleep 0.3
jobs
wait %1; echo "spec=$?"
wait $p; echo "pid=$?"
wait $p; echo "again=$?"
(exit 5) & sleep 0.3; wait %%; echo "builtin sleep=$?"
(exit 6) & :; wait %%; echo "unnoticed=$?"
( (exit 7) & /bin/sleep 0.3; wait %%; echo "subshell=$?" )
(exit 8) & x=$(/bin/sleep 0.3); wait %%; echo "after a substitution=$?"
set -m
(exit 9) & /bin/sleep 0.3; wait %%; echo "monitor=$?"
set +m
(sleep 0.3; exit 3) & /bin/sleep 0.01; wait %1; echo "settled=$?"`)
	want := "cur=0\nspec=0\npid=3\nagain=127\nbuiltin sleep=5\nunnoticed=6\nsubshell=0\nafter a substitution=0\nmonitor=9\nsettled=3\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
