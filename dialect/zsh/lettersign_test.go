// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// Each letter on a declaration takes its own sign, not the last word's.
// Measured 2026-10-03 on zsh 5.9.2 under `-fc` (#5673).
func TestEachLetterTakesItsOwnSign(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset +t -x s=Bc; typeset -p s`, "export s=Bc\n"},
		{`typeset -t +x s=Bc; typeset -p s`, "typeset -t s=Bc\n"},
		{`typeset +l -x s=Bc; typeset -p s`, "export s=Bc\n"},
		{`typeset -l +x s=Bc; typeset -p s`, "typeset -l s=Bc\n"},
		{`typeset +u -x s=Bc; typeset -p s`, "export s=Bc\n"},
		{`typeset +U -x s=Bc; typeset -p s`, "export s=Bc\n"},
		{`typeset -U +x s=Bc; typeset -p s`, "typeset -U s=Bc\n"},
		{`typeset +H -x s=Bc; typeset -p s`, "export s=Bc\n"},
		{`typeset -H +x s=Bc; typeset -p s`, "typeset s\n"},
		{`typeset +a -x s=Bc; typeset -p s`, "export s=Bc\n"},
		// The width and float letters are one attribute each, read off the
		// last of their letters written.
		{`typeset +Z 3 -x s=12; typeset -p s`, "export s=12\n"},
		{`typeset -Z 3 +x s=12; typeset -p s; echo $s`, "typeset -Z3 s=12\n012\n"},
		{`typeset -L3 +x s=12; typeset -p s`, "typeset -L3 s=12\n"},
		{`typeset +R 3 -x s=12; typeset -p s`, "export s=12\n"},
		{`typeset +F -x s=12; typeset -p s`, "export s=12\n"},
		{`typeset -F 2 +x s=12; typeset -p s`, "typeset -F s=12.00\n"},
		{`typeset -E 2 +x s=12; echo $s`, "1.2e+01\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}

// A letter written under both signs takes the last of them. Measured
// 2026-10-03 on zsh 5.9.2 under `-fc` (#5673). See
// interp.Semantics.ALetterUnderBothSignsComesOff.
func TestALetterUnderBothSignsTakesTheLast(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -l +l s=Bc; typeset -p s`, "typeset s=Bc\n"},
		{`typeset +l -l s=Bc; typeset -p s`, "typeset -l s=Bc\n"},
		{`typeset +x -x s=Bc; typeset -p s`, "export s=Bc\n"},
		{`typeset +r -r s=Bc; typeset -p s`, "typeset -r s=Bc\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}

// The tie letter's plus form is a refusal that ends the script, and it is the
// letter's own sign that asks for it: `typeset +T -x s=Bc` refused the tie as
// a minus one ("-T requires names of scalar and array") and ran on. Measured
// 2026-10-03 on zsh 5.9.2 under `-fc` (#5673).
func TestAPlusTieLetterBeforeAMinusWordIsRefused(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset +T -x s=Bc; echo after`)
	if !strings.Contains(out, "use unset to remove tied variables") || strings.Contains(out, "after") || st != 1 {
		t.Errorf("got %q at %d, want the untie refusal ending the script at 1", out, st)
	}
}
