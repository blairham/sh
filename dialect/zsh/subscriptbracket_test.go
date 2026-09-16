// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The backslash holds a `]` back from ending a subscript here, and a quote
// does not — the narrowest row of the panel, and the reason the quoted
// spellings are asked in the bash and ksh files instead. Read it beside
// subscriptquote_test.go, which is the other half of the same shell's answer:
// a subscript is not a quoting context here, so what a quote does to the
// *key* and what it does to the *scan* are both narrower than bash's.
//
// Measured on zsh 5.9.2 (2026-09-15): the backslash form answers `1`, and
// `${a['x]y']}` stops at the bracket inside the quotes and is a bad
// substitution.
func TestOnlyABackslashProtectsASubscriptsBracket(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -A a; a[x\]y]=1; print -r -- "${a[x\]y]}"`)
	if out != "1\n" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "1\n")
	}
	out, st = runZsh(t, t.TempDir(), `typeset -A a; a[x\]y]=1; print -r -- "${a['x]y']}"`)
	if out == "1\n" {
		t.Errorf("got %q (status %d), want the quoted bracket to end the subscript", out, st)
	}
}
