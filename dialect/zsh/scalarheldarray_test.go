// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// zsh replaces the array with the scalar, so the name is a plain scalar
// afterwards and never holds the state the axis is about. Measured 2026-10-03
// on zsh 5.9 under `-c` (#5643). See
// interp.Semantics.ScalarHeldUnderTheArrayLetterListsAsAScalar.
func TestAScalarStoreIntoAnEmptyArrayListsAsAnArray(t *testing.T) {
	if out, st := runZsh(t, t.TempDir(), `typeset -a x; x=/y; typeset -p x`); out != "typeset x=/y\n" || st != 0 {
		t.Errorf("got %q at %d", out, st)
	}
}
