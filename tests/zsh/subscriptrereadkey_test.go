// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A subscript re-read out of a value finds the element here too, and it
// reaches that answer from the other side: no subscript of this preset is a
// quoting context, so nothing is ever removed and there is nothing for a
// mark to protect.
//
// Measured 2026-09-20 against zsh 5.9.2 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// with `typeset -A m; k="q'r'z"; m[$k]=4; b='a\c'; m[$b]=11` in front of
// each. The last row is the one this column does not share with bash 5.3.20:
// the apostrophes spelled in the re-read text are two characters of the key
// here and a quotation there, which is
// Semantics.SubscriptIsAQuotingContext (#3902).
func TestASubscriptReReadOutOfAValueNamesTheKeyItHolds(t *testing.T) {
	const table = `typeset -A m; k="q'r'z"; m[$k]=4; b='a\c'; m[$b]=11; `
	for _, tc := range []struct{ name, src, want string }{
		{"apostrophes", `e='m[$k]'; printf "[%s]" "$(( $e ))"`, "[4]"},
		{"a backslash", `e='m[$b]'; printf "[%s]" "$(( $e ))"`, "[11]"},
		{"the store side", `e='m[$k]'; (( $e = 99 )); printf "[%s]" "${m[$k]}"`, "[99]"},
		{"quoting the re-read text spelled", `g="m[q'r'z]"; printf "[%s]" "$(( $g ))"`, "[4]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, table+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
