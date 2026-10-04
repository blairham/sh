// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// ArithKeyReadCreatesTheElement: an expression reading a key an association
// does not have adds it, empty, in ksh93u+ — measured 2026-10-03 — and leaves
// the table alone in bash and zsh. The expansion `${m[q]}` adds nothing in
// either reading, which is what places the rule on the expression's read.
func TestAnExpressionReadingAMissingKeyCanCreateIt(t *testing.T) {
	const src = `typeset -A m; m[k]=9; : $(( m[x] )); echo "${#m[@]}"; : "${m[q]}"; echo "${#m[@]}"; echo "[$(( m[*] ))][${#m[@]}]"`
	for _, tc := range []struct {
		a    Answer
		want string
	}{
		{Yes, "2\n2\n[0][3]\n"},
		{No, "1\n1\n[0][1]\n"},
	} {
		s := testSemantics()
		s.ArithKeyReadCreatesTheElement = tc.a
		s.ArithWholeArraySubscriptIsTheSlice = No
		out, st := run(t, src, withSem(s))
		if out != tc.want || st != 0 {
			t.Errorf("answered %v: got %q status %d, want %q and 0", tc.a, out, st, tc.want)
		}
	}
	t.Run("a key that is there never asks", func(t *testing.T) {
		s := testSemantics()
		s.ArithKeyReadCreatesTheElement = Unspecified
		if out, st := run(t, `typeset -A m; m[k]=9; echo $(( m[k] + 1 ))`, withSem(s)); out != "10\n" || st != 0 {
			t.Errorf("got %q status %d, want 10 at 0", out, st)
		}
	})
}
