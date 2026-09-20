// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// The whole-array spelling is the **typed characters** and nothing else: a
// subscript that merely *expands* to `@` is an expression like any other, and
// this column refuses it and ends the input.
//
// Pinned here as well as in bash's suite because the two columns agree, which
// is what says the reading is the core's and not an axis. Measured 2026-09-20
// against AT&T ksh93u+ 2012-08-01 (`/bin/ksh` here), script files under
// `env -i PATH=/usr/bin:/bin` with a scratch HOME. Every refused row answered
// `p q r` at status 0 here (#3889).
//
// The table half is unreachable in this column and that is the measurement
// rather than an omission: `m[@]=Z` is `@: invalid subscript in assignment`
// and ends the input, so there is no key stored to read back. bash's suite
// carries that row.
func TestOnlyATypedWholeArraySubscriptNamesEveryElement(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"an expanded subscript is an expression",
			`a=(p q r); K=@; echo "[${a[$K]}]"`,
			"sh: @: arithmetic syntax error\n",
		},
		{
			"the braced spelling likewise",
			`a=(p q r); K=@; echo "[${a[${K}]}]"`,
			"sh: @: arithmetic syntax error\n",
		},
		{
			// The blanks are quoted back as they were written, which is the
			// same sentence this column gives any subscript it cannot read.
			"blanks around the characters take the spelling away",
			`a=(p q r); echo "[${a[ @ ]}]"`,
			"sh:  @ : arithmetic syntax error\n",
		},
		{
			"the star spelling asks the same question",
			`a=(p q r); K=*; echo "[${a[$K]}]"`,
			"sh: *: arithmetic syntax error\n",
		},
		// The controls. The typed spellings must not move.
		{
			"the control: the typed subscript is the array",
			`a=(p q r); echo "[${a[@]}][${a[*]}]"; echo "${#a[@]} [${!a[@]}]"`,
			"[p q r][p q r]\n3 [0 1 2]\n",
		},
		{
			"the control: a table reads an expanded subscript as a key",
			`typeset -A m=([k]=v [j]=w); K=k; echo "[${m[$K]}]"; echo "[${m[@]}]"`,
			"[v]\n[w v]\n",
		},
		{
			"the control: an ordinary expanded subscript still indexes",
			`a=(p q r); K=1; echo "[${a[$K]}]"`,
			"[q]\n",
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
