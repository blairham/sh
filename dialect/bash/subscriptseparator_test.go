// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A subscript written at command position runs to its matching `]`, so a key
// may hold a blank (#2410). Measured against bash 5.3.15 on 2026-09-12, every
// row read back with `typeset -p m`.
func TestASubscriptAtCommandPositionHoldsItsSeparators(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			// The issue's own line.
			"a blank",
			`typeset -A m; m[foo bar]=qux; echo "[${m[foo bar]}]"`,
			"[qux]\n",
		},
		{
			// The same cut showed on the append spelling.
			"an append",
			`typeset -A m; m[foo bar]=qux; m[foo bar]+=" blat"; echo "[${m[foo bar]}]"`,
			"[qux blat]\n",
		},
		{
			// Not blanks but brackets: an operator inside them is a
			// character of the key.
			"a semicolon",
			`typeset -A m; m[a; b]=v; echo "[${m[a; b]}]"`,
			"[v]\n",
		},
		{
			// It is the *matching* `]`, so brackets nest.
			"a nested bracket",
			`typeset -A m; m[a [b] c]=v; echo "[${m[a [b] c]}]"`,
			"[v]\n",
		},
		{
			// An assignment prefix is command position too.
			"behind another assignment",
			`typeset -A m; a=1 m[foo bar]=v; echo "[${m[foo bar]}]$a"`,
			"[v]1\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, c.want)
			}
		})
	}
}

// An argument is not command position, and bash ends one at the blank: the
// two fields are what `printf` is handed. Without this row the grammar flag
// would read as "a subscript holds its blanks", which is a larger claim than
// the shell makes.
func TestAnArgumentsSubscriptStillEndsAtTheBlank(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `printf "<%s>" m[foo bar]=v; echo`)
	const want = "<m[foo><bar]=v>\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}
