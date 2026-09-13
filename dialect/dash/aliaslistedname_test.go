// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// This shell takes any alias name at all and leaves the name **bare** in a
// listing however it is spelled, which is the other side of the disagreement
// Semantics.AliasListingQuotesTheName records: zsh 5.9.2 quotes the same name
// and this one does not.
//
// Measured 2026-09-12 and re-measured 2026-09-13 on dash 0.5.12, `env -i
// PATH=/usr/bin:/bin` over a script file. It is worth pinning here rather
// than only where it changed: the axis was added for one column's behavior,
// and a column that answers `No` by nothing having asked it is not an
// answer (#2579).
func TestAListingLeavesAnAliasNameBare(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`alias 'a$b'=echo; alias 'a$b'`, "a$b='echo'\n"},
		{`alias 'a b'=echo; alias 'a b'`, "a b='echo'\n"},
		{`alias 'a#b'=echo; alias 'a#b'`, "a#b='echo'\n"},
		{`alias 'a$b'=echo; command -v 'a$b'`, "alias a$b='echo'\n"},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}
