// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A `$'…'` written inside a `${ … }` that stands in double quotes (#4169).
//
// Two questions, and the same axis answers both: where a quote quotes, the
// `$'…'` run is the quoting. This column is the one that reads a quote in
// *every* operand — syntax.Dialect.QuoteProtectsTheClosingBrace — so the run
// survives the double quotes in a word operand here and in no other column.
//
// Measured 2026-09-22 under `env -i PATH=/usr/bin:/bin LC_ALL=C` from script
// files, with `unset u` and `v="'"`:
//
//	case                          bash 5.3  bash 3.2  zsh 5.9.2  ksh93u+  ash
//	printf '[%s]' "${u:-$'\t'}"   TAB       TAB       $'\t'      $'\t'    $'\t'
//	printf '[%s]' "${v/$'\''/x}"  x         refused   x          x        x
//	printf '[%s]' ${v/$'\''/x}    x         x         x          x        x
//
// Row two is bash 3.2's one departure and is not modeled: 5.3 is the column
// this dialect follows, and the suite file that found all of this is 5.3's.
//
// The pattern operand is unanimous and is not this column's doing — it is
// read on its own terms everywhere — so the rows that only need the run's
// *end* found correctly are pinned in every dialect's own file.
func TestADollarSingleQuoteInAQuotedBraceOperand(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			// The word operand, which is this column's alone.
			"a default word inside double quotes",
			`unset u; printf "[%s]" "${u:-$'\t'}"`, "[\t]",
		},
		{
			"a default word outside them",
			`unset u; printf "[%s]" ${u:-$'\t'}`, "[\t]",
		},
		{
			// A plain double-quoted run is not the question and does not
			// move: the suite file has this line and it must stay eight
			// characters.
			"a plain double-quoted run",
			`printf "[%s]" "$'a\tb'"`, `[$'a\tb']`,
		},
		{
			// A here-document body is marked double-quoted so that nothing
			// in it is split, and that mark is not a pair of quotes.
			"a here-document body keeps the text",
			"unset u; IFS= read -r l <<EOF\n[${u:-$'\\t'}]\nEOF\nprintf '%s' \"$l\"",
			`[$'\t']`,
		},
		{
			// The pattern operand, whose run has to end at the *unescaped*
			// quote for the separator to be found at all.
			"a replacement pattern inside double quotes",
			`v="'"; printf "[%s]" "${v/$'\''/x}"`, `[x]`,
		},
		{
			"a replacement pattern outside them",
			`v="'"; printf "[%s]" ${v/$'\''/x}`, `[x]`,
		},
		{
			// And the replacement operand, which the suite file asks of
			// both sides at once.
			"a replacement operand inside double quotes",
			`v="'"; printf "[%s]" "${v/$'\''/$'\''}"`, `[']`,
		},
		{
			// The control that says what moved was the escape rather than
			// where a quoted run ends: this spelling of the same byte was
			// right before any of it.
			"the same byte written without an escaped quote",
			`v="'"; printf "[%s]" ${v/$'\x27'/x}`, `[x]`,
		},
		{
			// A `}` inside the run is still the run's, in both readings.
			"a brace inside the run",
			`unset u; printf "[%s]" "${u:-$'a}b'}"`, `[a}b]`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("%q said %q (status %d), want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}
