// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// Every quoting construct holds a `]` back from ending a subscript here,
// `$'…'` included — the widest row of the panel, and the one cell where this
// shell and bash part. Measured on ksh93u+ 2012 (2026-09-15): each line
// answers `1`.
func TestEveryQuotingProtectsASubscriptsBracket(t *testing.T) {
	for _, src := range []string{
		`typeset -A a; a['x]y']=1; print -r -- "${a['x]y']}"`,
		`typeset -A a; a['x]y']=1; print -r -- "${a["x]y"]}"`,
		`typeset -A a; a['x]y']=1; print -r -- "${a[x\]y]}"`,
		`typeset -A a; a['x]y']=1; print -r -- "${a[$'x]y']}"`,
	} {
		out, st := runKsh(t, t.TempDir(), src)
		if out != "1\n" || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", src, out, st, "1\n")
		}
	}
}
