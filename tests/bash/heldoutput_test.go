// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// **A builtin's output and its complaints interleave on one stream**, each
// written as it is made (#5228). Measured 2026-10-01 on bash 5.3.20 with both
// streams on one pipe: the `not found` between the two listed aliases. See
// interp.Semantics.BuiltinOutputHeldUntilItReturns.
func TestABuiltinsOutputInterleavesItsComplaints(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `alias aa=1 xx=3; alias aa nosuch xx`)
	if want := "alias aa='1'\nbash: line 1: alias: nosuch: not found\nalias xx='3'\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
