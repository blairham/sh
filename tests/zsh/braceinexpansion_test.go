// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A bare `{` inside an unquoted `${…}` opens a nesting level here, so the
// expansion ends at the brace that balances it.
//
// The substrate's tests name the flag — `BareBraceNestsInExpansion` — and
// this one names the shell, which is the only place that is allowed: the
// flag is off in the core, so without a preset turning it on the reading
// belongs to nobody.
//
// Measured 2026-09-10 on zsh 5.9.2 against the rest of the panel. This shell
// and ksh93 answer two fields; bash 5.3.15, bash 3.2.57 and dash answer one.
func TestABareBraceNestsInAnUnquotedExpansion(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			// The operand runs to `.z`, so the group is the operand's and
			// is expanded with the suffix on each field.
			"the group is inside the operand",
			`unset u; printf "[%s]" ${u:-{a,q}.z}`,
			"[a.z][q.z]",
		},
		{
			// No comma in it at all, which is what says the rule is about
			// the scan rather than about brace expansion: the `}` that
			// would have closed the expansion is inside the operand here.
			"a brace with nothing to expand still nests",
			`unset u; printf "[%s]" ${u:-a{b}c}`,
			"[a{b}c]",
		},
		{
			// The set case, where everything after the `}` that balances is
			// outside the word: nothing trails the value.
			"and the value takes the whole expansion with it",
			`v=q; printf "[%s]" ${v:-a{b}c}`,
			"[q]",
		},
		{
			// A trim pattern's operand is the same scan.
			"a trim operand too",
			`v=abc; printf "[%s]" ${v#a{b}}`,
			"[abc]",
		},
		{
			// In double quotes every column stops at the first `}`, so the
			// nesting is unquoted alone. Without this row the flag reads as
			// "a bare brace always nests here" (#1586).
			"and quoted it does not nest",
			`v=q; printf "[%s]" "${v:-a{b}c}"`,
			"[qc}]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
