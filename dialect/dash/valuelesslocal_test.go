// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// A valueless `local` of a name its own scope already holds lists nothing
// and keeps the value. Measured 2026-10-04 on dash 0.5.12.
func TestAValuelessLocalOfAHeldNameListsNothing(t *testing.T) {
	out, st := answersRun(t, `f() { local FOO=x; local FOO; echo "read=[${FOO-UNSET}]"; }; f`)
	if out != "read=[x]\n" || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, "read=[x]\n")
	}
}
