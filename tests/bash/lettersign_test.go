// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// Each letter on a declaration takes its own sign, not the last word's.
// Measured 2026-10-03 on bash 5.3.20 under `-c` (#5673).
func TestEachLetterTakesItsOwnSign(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`declare +t -x s=Bc; declare -p s`, "declare -x s=\"Bc\"\n"},
		{`declare -t +x s=Bc; declare -p s`, "declare -t s=\"Bc\"\n"},
		{`declare +l -x s=Bc; declare -p s`, "declare -x s=\"Bc\"\n"},
		{`declare -l +x s=Bc; declare -p s`, "declare -l s=\"bc\"\n"},
		{`declare +u -x s=Bc; declare -p s`, "declare -x s=\"Bc\"\n"},
		{`declare -u +x s=Bc; declare -p s`, "declare -u s=\"BC\"\n"},
		{`declare +c -x s=Bc; declare -p s`, "declare -x s=\"Bc\"\n"},
		{`declare +a -x s=Bc; declare -p s`, "declare -x s=\"Bc\"\n"},
		{`declare -a +x s=Bc; declare -p s`, "declare -a s=([0]=\"Bc\")\n"},
		{`declare +A -x s=Bc; declare -p s`, "declare -x s=\"Bc\"\n"},
		{`declare -A +x s=Bc; declare -p s`, "declare -A s=([0]=\"Bc\" )\n"},
		// A plus reference letter beside others: they land on the name.
		{`declare +n -x s=Bc; declare -p s`, "declare -x s=\"Bc\"\n"},
		{`declare +n -i s=1+2; declare -p s`, "declare -i s=\"3\"\n"},
		{`declare -n r; declare +n -x r=Bc; declare -p r`, "declare -x r=\"Bc\"\n"},
	} {
		if out, st := runBash(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}

// A letter written under both signs comes off, whichever came last. Measured
// 2026-10-03 on bash 5.3.20 under `-c` (#5673). See
// interp.Semantics.ALetterUnderBothSignsComesOff.
func TestALetterUnderBothSignsComesOff(t *testing.T) {
	for _, src := range []string{
		`declare -l +l s=Bc; declare -p s`,
		`declare +l -l s=Bc; declare -p s`,
		`declare +x -x s=Bc; declare -p s`,
		`declare +r -r s=Bc; declare -p s`,
		`declare +t -t s=Bc; declare -p s`,
	} {
		if out, st := runBash(t, t.TempDir(), src); out != "declare -- s=\"Bc\"\n" || st != 0 {
			t.Errorf("%s\n got %q at %d, want the plain row", src, out, st)
		}
	}
}
