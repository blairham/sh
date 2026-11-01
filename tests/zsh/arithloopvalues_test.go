// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A loop evaluates one arithmetic text many times, and each evaluation reads
// its own values: the spans are kept per text, so the nodes in them are the
// same every time, and what one evaluation held about a node must not answer
// the next. Kept before that was so, the second row came back `set:x` alone
// and the first without either miss — each iteration answered with the
// first one's element. Measured on zsh 5.9.2: both rows as written here.
func TestALoopsArithmeticReadsEachIterationsValues(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{
			`arr=(a); for p in a b c; do (( ! $arr[(I)$p] )) && print -n "miss:$p "; done; echo`,
			"miss:b miss:c \n",
		},
		{
			`typeset -A A; A=(x 1 y 0 z 2); for k in x y z w; do (( ${A[$k]:-0} )) && print -n "set:$k "; done; echo`,
			"set:x set:z \n",
		},
	} {
		if out, st := answersRun(t, tc.src); out != tc.want || st != 0 {
			t.Errorf("%s\ngave %q at %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}
