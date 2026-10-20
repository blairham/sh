// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A subscript re-read out of a value has its own expansion performed here,
// and what that expansion produced is a character of the key rather than
// quoting — so a key holding apostrophes is found through `e='m[$k]'`.
//
// Measured 2026-09-20 against bash 5.3.20 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// with `declare -A m; k="q'r'z"; m[$k]=4; b='a\c'; m[$b]=11` in front of
// each. ksh93u+ 2012-08-01 and zsh 5.9.2 answer every row below the same
// way, so the rows are the core's; they are pinned per column because the
// corpus reaches neither `declare -A` nor this shape (#3902).
func TestASubscriptReReadOutOfAValueKeepsItsExpansionsQuotes(t *testing.T) {
	const table = `declare -A m; k="q'r'z"; m[$k]=4; b='a\c'; m[$b]=11; `
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

// The neighboring row, and the one this column answers alone: the same two
// apostrophes *spelled in the re-read text* are a quotation here and the
// element is not found.
//
// Measured in the same run: `g="m[q'r'z]"; $(( $g ))` is 0 here and 4 in
// ksh93u+ and zsh 5.9.2, which is Semantics.SubscriptIsAQuotingContext and
// not the rule above. The two rows differ by exactly where the characters
// came from, which is what says the element is found above by marking what
// the expansion produced rather than by leaving a quotation alone.
func TestQuotingSpelledInAReReadSubscriptIsRemovedHere(t *testing.T) {
	const src = `declare -A m; k="q'r'z"; m[$k]=4; g="m[q'r'z]"; printf "[%s]" "$(( $g ))"`
	if out, st := answersRun(t, src); out != "[0]" || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, "[0]")
	}
}
