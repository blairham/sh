// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A write to one element of an attributed array leaves the other elements as
// they are here too — and this column is where that is hard to see, because
// an attribute **arriving** over a standing array folds every element of it
// before any write happens. So `typeset -i b=5` over `b=(p q r)` is `5 0 0`
// in ksh93 and `5 q r` in bash, and both of those are the same rule: each
// column's own valueless answer, with the value written over one element.
//
// That distinction is what this column is pinned for. The rows were right
// here by accident — the store folded whatever array it was handed, which
// produced ksh93's answer for the wrong reason and bash's answer wrongly
// (#3888) — so a change that fixed bash by dropping the fold had to put the
// **arrival** where it belongs to keep this column still.
//
// Measured 2026-09-20 against AT&T ksh93u+ 2012-08-01 (`/bin/ksh` here),
// script files under `env -i PATH=/usr/bin:/bin` with a scratch HOME.
func TestAnAttributeArrivesOverTheElementsAndTheWriteReachesOne(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			// The row a value made unreachable: only the valueless form ever
			// asked whether the attribute reaches the standing elements.
			"a declaration carrying a value folds what was there and writes one",
			`b=(p q r); typeset -i b=5; echo "[${b[*]}]"`,
			"[5 0 0]\n",
		},
		{
			"a table likewise",
			`typeset -A m=([k]=v [j]=w); typeset -i m=5; typeset -p m`,
			"typeset -A -i m=([0]=5 [j]=0 [k]=0)\n",
		},
		{
			"the arrival, then an element written on its own line",
			`b=(p q r); typeset -i b; b[0]=5; echo "[${b[*]}]"`,
			"[5 0 0]\n",
		},
		{
			"a table's element written after the arrival",
			`typeset -A m=([k]=v [j]=w); typeset -i m; m[k]=5; typeset -p m`,
			"typeset -A -i m=([j]=0 [k]=5)\n",
		},
		// The controls.
		{
			"the control: the valueless declaration reaches every element",
			`b=(p q r); typeset -i b; echo "[${b[*]}]"`,
			"[0 0 0]\n",
		},
		{
			// The other half of the same axis, and the row that says the
			// arrival above is not a literal's own fold: this column does
			// not fold a literal's words, where bash does.
			"the control: a literal's words are not folded here",
			`typeset -i b; b=(p q r); echo "[${b[*]}]"`,
			"[p q r]\n",
		},
		{
			"the control: the value written still goes through the attribute",
			`typeset -i b=(1 2 3); b[1]=4+4; echo "[${b[*]}]"`,
			"[1 8 3]\n",
		},
		{
			"the control: so does an appended word",
			`typeset -i b=(1 2); b+=(3+3); echo "[${b[*]}]"`,
			"[1 2 6]\n",
		},
		{
			"the control: an array of numbers keeps them",
			`b=(1 2 3); typeset -i b=5; echo "[${b[*]}]"`,
			"[5 2 3]\n",
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
