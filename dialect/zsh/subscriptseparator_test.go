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
