// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// **`hist` with no history refuses the range it would have reached.**
// Measured 2026-10-03 on ksh93u+ with an empty history file; see
// registerHist for the table. `fc` is this shell's alias for it.
func TestHistWithNoHistoryRefusesItsRange(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`fc -l; print "st=$?"`, "ksh: hist: 1-0: invalid range\nst=1\n"},
		{`hist -l 1 2; print "st=$?"`, "ksh: hist: 1-1: invalid range\nst=1\n"},
		{`hist -l -3; print "st=$?"`, "ksh: hist: 1-0: invalid range\nst=1\n"},
		{`hist 3; print "st=$?"`, "ksh: hist: 3-0: invalid range\nst=1\n"},
		{`hist -s; print "st=$?"`, "ksh: hist: 0-0: invalid range\nst=1\n"},
	} {
		out, _ := runKsh(t, t.TempDir(), c.src)
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.src, out, c.want)
		}
	}
}
