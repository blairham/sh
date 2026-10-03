// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// bash lists the same state as the array of one. Measured 2026-10-03 on bash
// 5.3.20 under `-c` (#5643). See
// interp.Semantics.ScalarHeldUnderTheArrayLetterListsAsAScalar.
func TestAScalarStoreIntoAnEmptyArrayListsAsAnArray(t *testing.T) {
	if out, st := runBash(t, t.TempDir(), `declare -a x; x=/y; declare -p x`); out != "declare -a x=([0]=\"/y\")\n" || st != 0 {
		t.Errorf("got %q at %d", out, st)
	}
}
