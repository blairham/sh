// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// **A builtin's output comes after its complaints on one stream** (#5228).
// Measured 2026-10-01 on dash 0.5.12 under `-c` with both streams on one pipe:
// the `not found` first and the two listed aliases after it, where bash and
// BusyBox ash write it between them; the `eval` row is the control. See
// interp.Semantics.BuiltinOutputHeldUntilItReturns.
func TestABuiltinsOutputComesAfterItsComplaints(t *testing.T) {
	out, st := runDash(t, t.TempDir(), `alias aa=1 xx=3; alias aa nosuch xx
eval "echo a; echo b >&2; echo c"`)
	if want := "alias: nosuch not found\naa='1'\nxx='3'\na\nb\nc\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
