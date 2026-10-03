// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// bash reads a minus word after a plus one by its own sign. Measured
// 2026-10-03 on bash 5.3.20 under `-c` (#5667). See
// interp.Semantics.EarlierPlusMakesALaterLetterARemoval.
func TestALaterMinusWordStillAddsItsLetters(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`declare -x s=Ab; declare +x -i s=1+2; declare -p s`, "declare -i s=\"3\"\n"},
		// The minus letter after the plus one is added. Every letter reads
		// its own sign; see TestEachLetterTakesItsOwnSign (#5673).
		{`declare +i -x s=1+2; declare -p s`, "declare -x s=\"1+2\"\n"},
	} {
		if out, st := runBash(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}
