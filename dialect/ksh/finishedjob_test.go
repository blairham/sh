// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// **A finished background job stays in the table** — see
// interp.Semantics.FinishedJobLeavesTheTable. Measured against ksh93u+ under
// `-c`: the ended job still answers `%%` and `%1` with its own status.
//
// The delay is a loop of arithmetic and the notice a `$(:)`, because both are
// builtins there and fork nothing. With `/bin/sleep` in the middle the
// reference answers both specs with 0 rather than the status — the job is
// still in the table, holding a status it lost to the foreground reap — and
// that is a different divergence, #5302.
func TestAFinishedJobStaysInTheTable(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `(exit 4) & i=0; while (( i < 300000 )); do (( i++ )); done; x=$(:); wait %%; echo cur=$?
(exit 3) & i=0; while (( i < 300000 )); do (( i++ )); done; x=$(:); wait %1; echo spec=$?`)
	if want := "cur=4\nspec=3\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
