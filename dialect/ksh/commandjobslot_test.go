// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// **A command holds no job number while it runs** (#5301): a job a brace
// group or a function starts is job 1. Measured 2026-10-01 under `-c` on
// bash 5.3.20, ksh93u+, dash 0.5.12 and BusyBox ash 1.37.0, which all print
// `one` and `two`; zsh 5.9.2 prints `none` first. See
// interp.Semantics.ACommandHoldsAJobSlot.
func TestACommandHoldsNoJobSlot(t *testing.T) {
	out, _ := runKsh(t, t.TempDir(), `{ /bin/sleep 0.2 & }; jobs %1 >/dev/null 2>&1 && echo one || echo none
f() { /bin/sleep 0.2 & }; f; jobs %2 >/dev/null 2>&1 && echo two || echo none; wait`)
	if want := "one\ntwo\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
