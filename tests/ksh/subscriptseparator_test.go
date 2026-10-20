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

// And in the other position: an element of a compound array literal that
// opens with `[` runs to its matching `]` here too (#2299). Measured against
// ksh93u+ 2012-08-01 on 2026-09-13.
//
// This shell reaches further than bash does in this position — it spans for
// `a=( pre[1 2]=x )` as well, where bash makes two fields — and that extra
// reach is recorded in docs/spec and deliberately not implemented: the flag
// says where a subscript may stand, and the front of the element is the shape
// every shell that spans here agrees on.
func TestAnArrayLiteralElementHoldsItsSubscriptsSeparators(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a blank",
			`typeset -A m; m=( [one]=1 [two words]=2 ); echo "[${m[two words]}][${m[one]}]"`,
			"[2][1]\n",
		},
		{
			"a semicolon",
			`typeset -A m; m=( [a; b]=v ); echo "[${m[a; b]}]"`,
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
