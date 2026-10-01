// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "testing"

// **A finished background job stays in the table until something reports
// it** — see interp.Semantics.FinishedJobLeavesTheTable. Measured against
// BusyBox ash 1.37.0: the ended job still answers `%%` and `%1` with its own status.
func TestAFinishedJobStaysInTheTable(t *testing.T) {
	out, st := run(t, `(exit 4) &
/bin/sleep 0.3
wait %%; echo "cur=$?"
(exit 3) & p=$!
/bin/sleep 0.3
wait %1; echo "spec=$?"
wait $p; echo "pid=$?"
(exit 5) & :; wait %%; echo "unnoticed=$?"`)
	if want := "cur=4\nspec=3\npid=3\nunnoticed=5\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
