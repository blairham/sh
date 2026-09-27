// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// And the `unset` route into a valueless record, which this column reaches
// through the only spelling that scopes here — #4787, where the issue
// supposed ksh93 could not be asked at all.
//
// Measured 2026-09-27, same binary and environment: the row is silent at 0,
// with `${v-UNSET}` firing its default inside the call and the global back
// after it, so the shadow really is standing and this is the state the axis
// is about rather than a removal.
func TestUnsetOfALocalWritesNoRow(t *testing.T) {
	const src = `v=global
function f { typeset v; unset v; print "in=[${v-UNSET}]"; typeset -p v; print "st=$?"; }
f
print "after=[${v-UNSET}]"`
	const want = "in=[UNSET]\nst=0\nafter=[global]\n"
	out, st := runKsh(t, t.TempDir(), src)
	if out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q", out, st, want)
	}
}
