// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A subscript re-read out of a value has its own expansion performed here,
// and what that expansion produced is a character of the key rather than
// quoting — so a key holding apostrophes is found through `e='m[$k]'`.
//
// Measured 2026-09-20 against ksh93u+ 2012-08-01 (`/bin/ksh`) from a script
// file under `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the
// null device, with `typeset -A m; k="q'r'z"; m[$k]=4; b='a\c'; m[$b]=11` in
// front of each. bash 5.3.20 and zsh 5.9.2 answer every row below the same
// way, so the rows are the core's; they are pinned per column because the
// corpus reaches neither `typeset -A` nor this shape (#3902).
//
// The neighboring row is **not** pinned here and is a live divergence: the
// same apostrophes spelled in the re-read text — `g="m[q'r'z]"; $(( $g ))` —
// are 4 in ksh93u+ and 0 in this column, because a subscript that arrived
// already expanded is no quoting context in that shell. That is
// Semantics.LetOperandSubscriptIsAQuotingContext's question reached without
// the builtin, it is measured and filed rather than folded in here, and
// nothing about it moves with this rule.
func TestASubscriptReReadOutOfAValueKeepsItsExpansionsQuotes(t *testing.T) {
	const table = `typeset -A m; k="q'r'z"; m[$k]=4; b='a\c'; m[$b]=11; `
	for _, tc := range []struct{ name, src, want string }{
		{"apostrophes", `e='m[$k]'; printf "[%s]" "$(( $e ))"`, "[4]"},
		{"a backslash", `e='m[$b]'; printf "[%s]" "$(( $e ))"`, "[11]"},
		{"the store side", `e='m[$k]'; (( $e = 99 )); printf "[%s]" "${m[$k]}"`, "[99]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, table+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
