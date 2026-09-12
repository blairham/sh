// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// This shell reads a command word's subscript through to its matching `]` as
// bash does, so a key may hold a blank (#2410). Measured against ksh93u+
// 2012-08-01 on 2026-09-12, each row read back with `typeset -p m`.
//
// It is a separate row from bash's rather than a shared helper, because the
// two shells agree here by measurement and not by derivation: zsh has the
// arrays and refuses the same line, so nothing about having subscripts
// implies this.
func TestASubscriptAtCommandPositionHoldsItsSeparators(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a blank",
			`typeset -A m; m[foo bar]=qux; echo "[${m[foo bar]}]"`,
			"[qux]\n",
		},
		{
			"an append",
			`typeset -A m; m[foo bar]=qux; m[foo bar]+=" blat"; echo "[${m[foo bar]}]"`,
			"[qux blat]\n",
		},
		{
			"a semicolon",
			`typeset -A m; m[a; b]=v; echo "[${m[a; b]}]"`,
			"[v]\n",
		},
		{
			"a nested bracket",
			`typeset -A m; m[a [b] c]=v; echo "[${m[a [b] c]}]"`,
			"[v]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, c.want)
			}
		})
	}
}
