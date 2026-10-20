// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A bare `{` inside an unquoted `${…}` opens a nesting level here, the same
// as in zsh and unlike the three bash-family columns.
//
// The substrate's tests name the flag — `BareBraceNestsInExpansion` — and
// this one names the shell, which is the only place that is allowed.
//
// Measured 2026-09-10 on ksh93 (AJM 93u+ 2012-08-01) against the rest of the
// panel: this shell and zsh 5.9.2 answer two fields, bash 5.3.15, bash
// 3.2.57 and dash the single field `[{a,q.z}]`.
func TestABareBraceNestsInAnUnquotedExpansion(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			"the group is inside the operand",
			`unset u; printf "[%s]" ${u:-{a,q}.z}`,
			"[a.z][q.z]",
		},
		{
			// No comma, so the answer is about where the expansion ends
			// rather than about what a group produces.
			"a brace with nothing to expand still nests",
			`unset u; printf "[%s]" ${u:-a{b}c}`,
			"[a{b}c]",
		},
		{
			"and quoted it does not nest",
			`v=q; printf "[%s]" "${v:-a{b}c}"`,
			"[qc}]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runKsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
