// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// ksh93 takes the quoting off an indexed subscript before the arithmetic, as
// it does for a key. Measured 2026-10-03 on ksh93u+ 2012-08-01 under `-c`
// (#5562). See interp.Semantics.IndexedSubscriptKeepsItsQuoting.
func TestAQuotedIndexedSubscriptLosesItsQuoting(t *testing.T) {
	out, st := runKshEmptyArray(t, `a=(x y z); echo ${a['2']} ${#a['1']} ${a[\1]}; a['1']=Q; echo ${a[@]}`)
	if want := "z 1 y\nx Q z\n"; out != want || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, want)
	}
}
