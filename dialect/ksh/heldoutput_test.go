// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// **A builtin's output and its complaints interleave on one stream** for
// `type` (#5228). Measured 2026-10-01 on ksh93u+ under `-c` with both streams
// on one pipe: the `not found` between the two answers. ksh93's `alias` puts
// its complaint first instead, which is not one rule and is recorded on
// interp.Semantics.BuiltinOutputHeldUntilItReturns rather than modeled.
func TestABuiltinsOutputInterleavesItsComplaints(t *testing.T) {
	out, _ := runKsh(t, t.TempDir(), `f() { :; }; type f nosuchcmd type`)
	if want := "f is a function\nksh: whence: nosuchcmd: not found\ntype is an alias for 'whence -v'\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
