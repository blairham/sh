// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A single quote protects the closing brace in a **pattern** operand and not
// in a word one — the reading five of the seven panel columns take, and the
// one the core takes with them. Recorded here because ksh93 has the
// replacement form that dash does not, so its `/` operand is a pattern where
// dash's is nothing at all.
func TestAQuoteProtectsTheClosingBraceInAPatternOperand(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a pattern operand",
			`s=a}b; printf '[%s]' "${s#'a}'}" "${s%'}b'}"`,
			`[b][a]`,
		},
		{
			"a replacement operand's pattern",
			`v=xay; printf '[%s]' "${v/'a}'/z}"`,
			`[xay]`,
		},
		{
			"a word operand",
			`v=Vx}y; printf '[%s]' "${v-'a}b'}" "${u-'a}b'}"`,
			`[Vx}yb'}]['ab'}]`,
		},
		{
			"unquoted, where every column protects",
			`v=Vx}y; printf '[%s]' ${v-'a}b'} ${v#'V}'}`,
			`[Vx}y][Vx}y]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runKsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
