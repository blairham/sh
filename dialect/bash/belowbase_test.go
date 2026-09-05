// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// The first element is 0 here, so no non-negative subscript is below it — the
// refusal is reached by a negative one that counts back past the start. The
// script ends at 1. Measured against bash 5.3.15 (2026-09-05); bash 3.2.57
// says the same for the plain form.
func TestANegativeSubscriptPastTheStartIsRefused(t *testing.T) {
	for _, c := range []struct{ src, sub string }{
		{`a=(p q); a[-3]=x; echo ok`, "a[-3]"},
		{`a=(p q); a[-3]+=Q; echo ok`, "a[-3]"},
		{`a[-1]=x; echo ok`, "a[-1]"},
		// Named as it was written, not as the number it came to.
		{`x=1; a[x-2]=v; echo ok`, "a[x-2]"},
	} {
		out, st := runBash(t, t.TempDir(), c.src)
		want := "bash: line 1: " + c.sub + ": bad array subscript\n"
		if out != want || st != 1 {
			t.Errorf("%s = %q (status %d), want %q at 1", c.src, out, st, want)
		}
	}
}

// Through a literal the element is named as it stands between the
// parentheses, with no array name in front of it.
func TestARefusedLiteralSubscriptNamesTheElement(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `a=(p q [-5]=x); echo ok`)
	want := "bash: line 1: [-5]=x: bad array subscript\n"
	if out != want || st != 1 {
		t.Errorf("got %q (status %d), want %q at 1", out, st, want)
	}
}
