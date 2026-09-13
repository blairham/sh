// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// This shell is the panel's holdout on a command word's subscript: it ends
// the word at the blank and then refuses the piece it is left with (#2410).
//
// Measured on zsh 5.9.2, 2026-09-12: `typeset -A m; m[foo bar]=qux` is
// `bad pattern: m[foo` at status 1, where bash 5.3.15 and ksh93u+ read
// through to the matching `]` and store the key `foo bar`. The row is here so
// that the grammar flag that gives those two the longer word cannot be turned
// on for this one without something objecting — the shells have the same
// arrays, so nothing about having subscripts implies the same word boundary.
func TestASubscriptHoldingABlankIsRefused(t *testing.T) {
	for _, src := range []string{
		`typeset -A m; m[foo bar]=qux; echo "[${m[foo bar]}]"`,
		`typeset -A m; m[foo bar]+=qux`,
		`typeset -A m; m[a; b]=v`,
	} {
		out, st := runZsh(t, t.TempDir(), src)
		if st == 0 || !strings.Contains(out, "bad pattern: m[") {
			t.Errorf("%s = %q (status %d), want a refused pattern at nonzero", src, out, st)
		}
	}
}

// The holdout holds in the other position too: an array literal's element is
// cut at the blank and the bracket is then read as a glob (#2299).
//
// Measured on zsh 5.9.2, 2026-09-13: `typeset -A m; m=( [two words]=2 )` is
// `bad pattern: [two` at status 1, where bash 5.3.15 and ksh93u+ store the
// key `two words`. Here so that the flag giving those two the longer element
// cannot be turned on for this one in silence.
func TestAnArrayLiteralElementHoldingABlankIsRefused(t *testing.T) {
	for _, src := range []string{
		`typeset -A m; m=( [two words]=2 )`,
		`m=( [1 2]=x )`,
	} {
		out, st := runZsh(t, t.TempDir(), src)
		if st == 0 || !strings.Contains(out, "bad pattern: [") {
			t.Errorf("%s = %q (status %d), want a refused pattern at nonzero", src, out, st)
		}
	}
}
