// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// An apostrophe inside a subscript that arrived already expanded stops the
// expansion it holds here, and the apostrophes are then removed — so the key
// is the three characters `$kq` and not the one character `q`.
//
// Measured 2026-09-20 against bash 5.3.20 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// with `declare -A m; kq=q` in front of each. Every row stores through the
// re-read subscript — `e='m[<text>]'` then `(( $e = 42 ))` — and lists the key
// the table ended up holding, which is the only reading that tells the two
// answers apart.
//
// ksh93u+ 2012-08-01 gives the same key in every row below; the columns part
// on rows that hold no expansion, which is #3917 and not this. zsh 5.9.2
// parts on all of them, over Semantics.SubscriptIsAQuotingContext (#3918).
func TestAnApostropheInAReReadSubscriptStopsItsExpansion(t *testing.T) {
	for _, tc := range []struct{ name, text, want string }{
		{"around the expansion", `'\$kq'`, "<$kq>"},
		{"a double quotation around it", `\"\$kq\"`, "<q>"},
		{"no quotation at all", `\$kq`, "<q>"},
		{"an apostrophe mid-subscript", `q'\$kq'z`, "<q$kqz>"},
		{"a backslash it holds", `'\\\$kq'`, `<\$kq>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `declare -A m; kq=q; e="m[` + tc.text + `]"; (( $e = 42 ))` +
				`; for k in "${!m[@]}"; do printf "<%s>" "$k"; done`
			out, st := answersRun(t, src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
