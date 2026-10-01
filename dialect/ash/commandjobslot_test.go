// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "testing"

// **A command holds no job number while it runs** (#5301): a job a brace
// group or a function starts is job 1. Measured 2026-10-01 under `-c` on
// BusyBox ash 1.37.0 (alpine@sha256:28bd5fe8…), which prints `one` and
// `two`; zsh 5.9.2 prints `none` first. See
// interp.Semantics.ACommandHoldsAJobSlot.
func TestACommandHoldsNoJobSlot(t *testing.T) {
	out, _ := runIn(t, `{ /bin/sleep 0.2 & }; jobs %1 >/dev/null 2>&1 && echo one || echo none
f() { /bin/sleep 0.2 & }; f; jobs %2 >/dev/null 2>&1 && echo two || echo none; wait`)
	if want := "one\ntwo\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
