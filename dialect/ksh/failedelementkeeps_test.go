// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A chained element write whose value would not expand keeps the element it
// names. The failure ends the script, so an exit trap reads what is left.
// Measured 2026-10-15 on ksh93u+ 2012-08-01 under `-c`, these three lines:
// the division on line 3, then
// `typeset -a a=(1 ([0]=2 [2]=old) )` and status 1. It stored the empty
// string under `[2]` (#6579).
func TestAFailedChainedElementWriteKeepsTheElement(t *testing.T) {
	src := "trap 'typeset -p a' EXIT\na=(1 2); typeset -a a[1][2]=old\na[1][2]=$((1/0))\n"
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if want := "ksh: line 3: 1/0: divide by zero\ntypeset -a a=(1 ([0]=2 [2]=old) )\n"; out != want || st != 1 {
		t.Errorf("got %q at %d\nwant %q at 1", out, st, want)
	}
}
