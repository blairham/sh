// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A store through a resolved reference whose text carries the whole-array
// spelling — `v='x[@]'; ${(P)v::=Z}` — is this column's ordinary taking
// answer over an array and its slice refusal over a table.
//
// Measured 2026-09-20 on zsh 5.9.2, script files under `env -i
// PATH=/usr/bin:/bin`. Both walked the two characters on to the arithmetic
// evaluator here and answered `bad math expression: illegal character: @`,
// which **ends the script** — over a line this shell completes at 0.
//
// The `(P)` route and bash's `declare -n` route are one helper for that
// reason: the same two characters, the same evaluator, and a fix written into
// only one of them would have left the other ending the script. See
// Runner.storeWholeArraySubscriptThroughAReference.
func TestAWholeArraySubscriptThroughAReferenceIsASlice(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"an array takes the whole name",
			`x=(p q); v='x[@]'; : ${(P)v::=Z}; echo "same=$? [${x[*]}]"`,
			"same=0 [Z]\n",
		},
		{
			"and the star spelling with it",
			`y=(p q); w='y[*]'; : ${(P)w::=Y}; echo "same=$? [${y[*]}]"`,
			"same=0 [Y]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := answersRun(t, c.src)
			if out != c.want {
				t.Errorf("wrote %q, want %q", out, c.want)
			}
		})
	}
}
