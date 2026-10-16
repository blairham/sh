// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `[*]=` and `[@]=` written as a table literal's keys are ordinary keys here,
// as `m[@]=Z` is. Measured 2026-10-15 on bash 5.3.20 from a script file. See
// interp.Semantics.WholeArraySubscriptKeyInATableLiteral (#6686).
func TestAWholeArrayKeyInATableLiteralIsAnOrdinaryKey(t *testing.T) {
	out, st := runBash(t, t.TempDir(), "declare -A a; a=([k]=1 [*]=s [@]=t); declare -p a")
	if want := "declare -A a=([\"*\"]=\"s\" [\"@\"]=\"t\" [k]=\"1\" )\n"; out != want || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, want)
	}
}
