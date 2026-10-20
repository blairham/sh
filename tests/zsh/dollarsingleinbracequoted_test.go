// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A `$'…'` written inside a `${ … }` that stands in double quotes (#4169).
//
// The run has to end at the first *unescaped* quote for a separator behind it
// to be found at all, and that is unanimous: read as a plain `'…'` run, the
// run of a backslashed quote ends at that quote, the one behind it opens a
// second run, and the rest of the line goes into it.
//
// What this column does **not** do is read the construct in a *word* operand
// inside double quotes. That is bash's alone, and it is the same axis —
// syntax.Dialect.QuoteProtectsTheClosingBrace — asked one construct earlier.
//
// Measured 2026-09-22 under `env -i PATH=/usr/bin:/bin LC_ALL=C` from script
// files, with `unset u` and a variable holding one quote:
//
//	case                       bash 5.3  zsh 5.9.2  ksh93u+  ash 1.37.0
//	"${u:-$'\t'}" in quotes    a tab     the text   the text the text
//	"${v/…/x}"    in quotes    x         x          x        x
//	${v/…/x}      outside      x         x          x        x
func TestADollarSingleQuoteInAQuotedBraceOperand(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			// The word operand keeps the text here: the quotes are two
			// characters of it and the `$` is an ordinary dollar.
			"a default word inside double quotes",
			`unset u; printf "[%s]" "${u:-$'\t'}"`, `[$'\t']`,
		},
		{
			// And outside them it is the construct, which is what says the
			// row above is about the quoting and not about the escape.
			"a default word outside them",
			`unset u; printf "[%s]" ${u:-$'\t'}`, "[\t]",
		},
		{
			"a replacement pattern inside double quotes",
			`v="'"; printf "[%s]" "${v/$'\''/x}"`, `[x]`,
		},
		{
			"a replacement pattern outside them",
			`v="'"; printf "[%s]" ${v/$'\''/x}`, `[x]`,
		},
		{
			// The control: the same byte spelled without an escaped quote
			// was matched before any of this.
			"the same byte written without an escaped quote",
			`v="'"; printf "[%s]" ${v/$'\x27'/x}`, `[x]`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("%q said %q (status %d), want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}
