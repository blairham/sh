// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "testing"

// **A builtin's output and its complaints interleave on one stream**, each
// written as it is made (#5228). Measured 2026-10-01 on BusyBox ash 1.37.0 (alpine@sha256:28bd5fe8…, `exec 2>&1`) with both
// streams on one pipe: the `not found` between the two listed aliases. See
// interp.Semantics.BuiltinOutputHeldUntilItReturns.
func TestABuiltinsOutputInterleavesItsComplaints(t *testing.T) {
	out, _ := runIn(t, `alias aa=1 xx=3; alias aa nosuch xx`)
	if want := "aa='1'\nalias: nosuch not found\nxx='3'\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
