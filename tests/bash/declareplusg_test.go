// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// **`declare +g` in a function is a local**, not a global. Measured against
// bash 5.3: a global x=1 is left alone by `f(){ declare +g x; x=2; }`, and a
// `-g` anywhere on the line still reaches the global — `-g +g` and `+g -g`
// both assign it.
func TestDeclarePlusGIsALocal(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `x=1
f(){ declare +g x; x=2; }; f; echo "plus=$x"
f2(){ declare -g +g x; x=3; }; f2; echo "minusplus=$x"
f3(){ declare +g -g x; x=4; }; f3; echo "plusminus=$x"`)
	if want := "plus=1\nminusplus=3\nplusminus=4\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
