// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// zsh reads a minus word after a plus one by its own sign, and a plus
// numeric letter takes its number. Measured 2026-10-03 on zsh 5.9 under `-c`
// (#5667). See interp.Semantics.EarlierPlusMakesALaterLetterARemoval.
func TestALaterMinusWordStillAddsItsLetters(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -x s=Ab; typeset +x -i s=1+2; typeset -p s`, "typeset -i s=3\n"},
		// The minus letter after the plus one is added. Letters that read
		// the last word's sign rather than their own are #5673.
		{`typeset +i -x s=1+2; typeset -p s`, "export s=1+2\n"},
		{`typeset +F 3 v=1.5; echo $?; typeset -p v`, "0\ntypeset v=1.5\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}
