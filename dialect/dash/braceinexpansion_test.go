// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// A bare `{` inside an unquoted `${…}` opens nothing here either, and this
// shell is the one that shows the rule is about the *scan* rather than about
// brace expansion: it has no brace expansion at all and still has an answer.
//
// The substrate's tests name the flag — `BareBraceNestsInExpansion` — and
// this one names the shell (#1587).
//
// Measured 2026-09-10 on dash against the rest of the panel: one field here,
// in bash 5.3.15 and in bash 3.2.57; two in zsh 5.9.2 and ksh93.
func TestABareBraceDoesNotNestInAnUnquotedExpansion(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			"the expansion ends at the first brace",
			`unset u; printf "[%s]" ${u:-{a,q}.z}`,
			"[{a,q.z}]",
		},
		{
			"and the text after it stays in the word",
			`v=q; printf "[%s]" ${v:-a{b}c}`,
			"[qc}]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runDash(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
