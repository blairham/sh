// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An apostrophe inside a subscript that arrived already expanded stops
// nothing here, and stays in the key: no subscript of this preset is a
// quoting context, so `m['$kq']` reached through a value names the key `'q'`
// — the expansion performed, the apostrophes still there.
//
// This is the column the rule parts on, and it is the reason the rule is
// Semantics.SubscriptIsAQuotingContext read at a second site rather than an
// axis of its own: a preset that stopped the expansion and kept the quotes
// would be a fourth answer, and no column gives one.
//
// Measured 2026-09-20 against zsh 5.9.2 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// with `typeset -A m; kq=q` in front of each. bash 5.3.20 and ksh93u+
// 2012-08-01 answer every row below with the quotes removed (#3918).
//
// **None of these rows moves with the fix**, which is what makes them worth
// having: they say the change is confined to the columns that answer the axis
// yes. The row that can fail under a mutation lives in interp, where the
// same three shapes are run with the axis set both ways and the `no` column
// is what stops the fix reaching this preset.
func TestAnApostropheInAReReadSubscriptStopsNothingHere(t *testing.T) {
	for _, tc := range []struct{ name, text, want string }{
		{"around the expansion", `'\$kq'`, "<'q'>"},
		{"a double quotation around it", `\"\$kq\"`, `<"q">`},
		{"no quotation at all", `\$kq`, "<q>"},
		{"an apostrophe mid-subscript", `q'\$kq'z`, "<q'q'z>"},
		{"a backslash it holds", `'\\\$kq'`, `<'$kq'>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `typeset -A m; kq=q; e="m[` + tc.text + `]"; (( $e = 42 ))` +
				`; printf "<%s>" "${(k)m[@]}"`
			out, st := answersRun(t, src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
