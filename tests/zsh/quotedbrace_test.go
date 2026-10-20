// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A single quote written inside a double-quoted `${ … }` quotes **nothing**
// here, in any operand, so the expansion always ends at the first `}`. This
// shell alone reads a *pattern* operand's quote as an ordinary character too;
// the other six protect the brace there.
//
// The substrate's tests name the flag — `QuoteProtectsTheClosingBrace` — and
// this one names the shell, which is the only place that is allowed: the core
// protects a pattern operand, so the narrower reading belongs to a preset.
func TestAQuoteProtectsTheClosingBraceInNoOperand(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The pattern operand is the row that is this shell's alone:
			// the pattern is `'a`, which matches nothing, and the half-quote
			// after the brace falls out into the word.
			"a pattern operand",
			`s=a}b; printf '[%s]' "${s#'a}'}" "${s%'}b'}"`,
			`[a}b'}][a}bb'}]`,
		},
		{
			// The word operand, where five other columns agree.
			"a word operand",
			`v=Vx}y; printf '[%s]' "${v-'a}b'}" "${u-'a}b'}"`,
			`[Vx}yb'}]['ab'}]`,
		},
		{
			// The replacement form's pattern, the same rule one operator
			// over.
			"a replacement operand's pattern",
			`v=xay; printf '[%s]' "${v/'a}'/z}"`,
			`[xay'/z}]`,
		},
		{
			// Unquoted every column protects, in both kinds, so this is the
			// boundary rather than the rule.
			"unquoted",
			`v=Vx}y; printf '[%s]' ${v-'a}b'} ${v#'V}'}`,
			`[Vx}y][Vx}y]`,
		},
		{
			// And a double quote protects here as it does everywhere.
			"a double quote",
			`v=SET; printf '[%s]' "${v-"a}b"}" "${u#"a}"}"`,
			`[SET][]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
