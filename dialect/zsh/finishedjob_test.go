// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **A finished background job leaves the table here, once the shell has
// noticed it** — see interp.Semantics.FinishedJobLeavesTheTable. Measured
// against zsh 5.9.2 with this script as a file: a job spec naming the ended
// job is `no current job` or `no such job` at 127, the job's process id still
// answers its status, and a job that ended before the shell reaped any child
// in the foreground is still there.
func TestAFinishedJobLeavesTheTable(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `(exit 4) &
/bin/sleep 0.3
wait %%; echo "cur=$?"
(exit 3) & p=$!
/bin/sleep 0.3
wait %1; echo "spec=$?"
wait $p; echo "pid=$?"
(exit 5) & :; wait %%; echo "unnoticed=$?"`)
	want := "zsh:wait:3: no current job\ncur=127\nzsh:wait:6: %1: no such job\nspec=127\npid=3\nunnoticed=5\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
