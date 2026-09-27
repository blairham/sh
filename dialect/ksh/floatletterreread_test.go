// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A declaration that changes a float name's **letter** re-reads the
// characters the name is holding here, where one that changes only its
// **precision** keeps the number.
//
// Measured 2026-09-26 on ksh93u+ 2012-08-01 at `/bin/ksh`, from a script
// file, with `x=3.14159265358979`. The table is built so that the noun is
// under test rather than merely varied: every row starts from a rendering
// that threw digits away and then asks for a wider one, so keeping the number
// and re-reading the rendering give different answers. The rows that move
// only the precision are the same shape with the letter held fixed, and there
// the digits come back.
//
// The last two rows are the control that says this is not "the shell rounds
// on assignment": under a narrow letter the number is still whole, and it
// stays whole across a precision change. #4475 measured the rows both shells
// share; this is the one they do not, and Semantics.FloatLetterChange-
// RereadsTheRendering is where it lives. zsh keeps the number across both —
// see dialect/zsh/floatformat_test.go (#4486).
func TestAChangeOfFloatLetterRereadsTheRendering(t *testing.T) {
	dir := t.TempDir()
	const x = "3.14159265358979"
	for _, tc := range []struct{ name, src, want string }{
		{
			"places to figures",
			`typeset -F1 f=` + x + `; typeset -E10 f; print -- $f`,
			"3.1\n",
		},
		{
			"figures to places",
			`typeset -E3 f=` + x + `; typeset -F14 f; print -- $f`,
			"3.14000000000000\n",
		},
		{
			"places to figures, the other way round",
			`typeset -F3 f=` + x + `; typeset -E5 f; print -- $f`,
			"3.142\n",
		},
		{
			// And the number really is gone, not merely unprinted.
			"the arithmetic value after a letter change",
			`typeset -F1 f=` + x + `; typeset -E10 f; print -- $(( f ))`,
			"3.1\n",
		},
		{
			"a letter change twice over",
			`typeset -F3 f=` + x + `; typeset -E5 f; typeset -p f; typeset -F f; typeset -p f`,
			"typeset -E 5 f=3.142\ntypeset -F f=3.1420000000\n",
		},
		// The precision moves and the letter does not.
		{
			"the same letter, a wider precision",
			`typeset -F1 f=` + x + `; typeset -F14 f; print -- $f`,
			"3.14159265358979\n",
		},
		{
			"the same letter, wider figures",
			`typeset -E3 f=` + x + `; typeset -E10 f; print -- $f`,
			"3.141592654\n",
		},
		{
			"the same letter at the same precision",
			`typeset -E3 f=` + x + `; typeset -E3 f; print -- $f`,
			"3.14\n",
		},
		{
			"the arithmetic value after a precision change",
			`typeset -F1 f=` + x + `; typeset -F14 f; print -- $(( f ))`,
			"3.14159265358979\n",
		},
		{
			"the arithmetic value under a narrow rendering",
			`typeset -E3 f=` + x + `; print -- $(( f ))`,
			x + "\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
