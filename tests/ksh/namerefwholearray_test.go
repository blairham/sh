// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A store through a reference aimed at `m[@]` takes the key here, which is
// neither of this column's two answers to the spelling written bare.
//
// Measured 2026-09-20 on ksh93u+ 2012-08-01, script files under `env -i
// PATH=/usr/bin:/bin`: `typeset -A m=([k]=v); nameref n=m[@]` is taken at 0,
// `n=Z` leaves `[@]=Z` beside `[k]=v` and the rest of the line runs — where
// the bare `m[@]=Z` is `@: invalid subscript in assignment` and ends the
// input. That disagreement with itself is why
// Semantics.WholeArraySubscriptThroughAReferenceToATable is a field of its
// own rather than a reading of WholeArraySubscriptAssigningATable.
func TestAWholeArraySubscriptThroughAReferenceTakesTheKey(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"the key is stored and the line runs on",
			`typeset -A m=([k]=v); nameref n=m[@]; n=Z; echo "same=$? [${m[@]}][${m[k]}]"`,
			"same=0 [Z v][v]\n",
		},
		{
			"the star spelling is a key of its own",
			`typeset -A m=([k]=v); nameref s=m[*]; s=Y; echo "same=$? [${m[k]}]"`,
			"same=0 [v]\n",
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

// And the array half of the same route is unreachable here, which is what
// keeps that half of the axis unanswered: a subscript that is not a key is
// evaluated when the reference is **aimed**, so the reference never comes
// into being. Measured the same day: `x=(p q); nameref b=x[@]` is `typeset:
// @: arithmetic syntax error` and the input ends.
func TestAWholeArraySubscriptCannotAimAReference(t *testing.T) {
	t.Parallel()
	out, status := answersRun(t, `x=(p q); nameref b=x[@]; echo REACHED`)
	if status == 0 {
		t.Errorf("wrote %q at 0, want the aim refused and the input ended", out)
	}
	for _, want := range []string{"arithmetic syntax error"} {
		if !strings.Contains(out, want) {
			t.Errorf("wrote %q, want it to carry %q", out, want)
		}
	}
	if strings.Contains(out, "REACHED") {
		t.Errorf("wrote %q, want the input to have ended before the echo", out)
	}
}
