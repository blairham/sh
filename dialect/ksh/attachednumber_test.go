// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A number attached to its letter ends where its digits do, and the rest of
// the word is more letters. Measured 2026-10-03 on ksh93u+ 2012-08-01 under
// `env -i … -c` (#5685). See
// interp.Semantics.AttachedNumberEndsAtTheFirstNonDigit.
func TestAnAttachedNumberEndsWhereItsDigitsDo(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -Z3x s=7; typeset -p s`, "typeset -x -Z 3 -R 3 s=007\n"},
		{`typeset -L4x s=ab; typeset -p s`, "typeset -x -L 4 s='ab  '\n"},
		{`typeset -i16x s=255; typeset -p s`, "typeset -x -i 16 s=16#ff\n"},
		{`typeset -F2x s=1; typeset -p s`, "typeset -x -F 2 s=1.00\n"},
		{`typeset -E3x s=1; typeset -p s`, "typeset -x -E 3 s=1\n"},
		{`typeset -R3l s=AB; typeset -p s`, "typeset -l -R 3 s=' ab'\n"},
		{`typeset -Z3xr s=7; typeset -p s`, "typeset -x -r -Z 3 -R 3 s=007\n"},
		// The letters left still take a detached number.
		{`typeset -Z3L 4 s=7; typeset -p s`, "typeset -Z 3 -L 3 s='7  '\n"},
	} {
		if out, st := runKshEmptyArray(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}
