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
			// A `}` inside the run ends the expansion, because the value is
			// what the scan for the brace reads — see the test below, which
			// this row is the cheapest case of. It said `[a}b]` until #4207,
			// which is the answer finding the run's *end* correctly gives when
			// nothing reads the value afterwards.
			"a brace inside the run",
			`unset u; printf "[%s]" "${u:-$'a}b'}"`, `[ab}]`,
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

// What bash does *after* reading that `$'…'`: it puts the escape's value where
// the escape was written and reads the division off that text, so a value
// holding a `}` ends the expansion early and a value that is a quote character
// hides the `}` behind it (#4207).
//
// Measured 2026-09-23 under `env -i PATH=/usr/bin:/bin LC_ALL=C` from script
// files, with `unset u`, `v="'"` and `w=Q`. bash 5.3.20 and bash 3.2 agree on
// every row, and the four columns without the construct reach none of it and
// print the dollar and the quotes back — `printf '[%s]' "${u:-x$'\x27'}"` is
// `[x$'\x27']` in dash, ksh93, zsh and the same bash invoked as `sh`.
//
//	written                            bash 5.3.20 and 3.2
//	printf '[%s]' "${u:-$'a}b'}"       [ab}]
//	printf '[%s]' "${u:-$'\x7d'}"      [}]
//	printf '[%s]' "${u:-$'\x27'A}B'}"  ['A}B']
//	printf '[%s]' "${u:-x$'\x27'}"     no closing `}' in "${u:-x'}"
//	printf '[%s]' "${u:-x$'\x22'}"     no closing `}' in "${u:-x"}"
//	printf '[%s]' "${u:-x$'\x5c'}"     no closing `}' in "${u:-x\}"
//	printf '[%s]' "${v/$'\x27'/x}"     [x]
//	printf '[%s]' "${w/Q/$'\x27'}"     [']
//
// The diagnostic is the evidence, and it is what makes this a re-reading rather
// than a rule about the escape: bash quotes the word back with the escape
// already gone. Row three is the row saying the re-reading can also *succeed* —
// the produced quote protects a brace and the expansion ends at the next one, a
// division no reading of the written text produces.
//
// The last two rows are the bound. A pattern operand and a replacement operand
// are not re-read, so a produced quote there reaches neither the scan nor a
// refusal; the word operand is, which is the operand
// syntax.Dialect.QuoteProtectsTheClosingBrace already answers for.
//
// One shape is measured and deliberately not modeled: a value holding `$(`
// opens a command substitution and bash reports that instead of the brace —
// `command substitution: line 2: unexpected EOF while looking for matching`.
// Both refuse at status 1 and this shell says `no closing }` there.
func TestADollarSingleValueIsWhatTheBraceScanReads(t *testing.T) {
	for _, c := range []struct {
		name, src, want, diag string
		status                int
	}{
		{
			name: "a value holding the brace ends the expansion",
			src:  `unset u; printf "[%s]" "${u:-$'a}b'}"`, want: `[ab}]`,
		},
		{
			// The cheap spelling of the same thing, and the row that is not on
			// its own evidence: an empty word operand with a leftover `}` after
			// it prints what a protected `}` in the operand would print too.
			name: "a value that is only the brace",
			src:  `unset u; printf "[%s]" "${u:-$'\x7d'}"`, want: `[}]`,
		},
		{
			// The re-reading succeeding rather than refusing: the produced
			// quote protects the first `}` and the second ends the expansion,
			// so the operand is text the written word did not hold.
			name: "a produced quote protects a brace behind it",
			src:  `unset u; printf "[%s]" "${u:-$'\x27'A}B'}"`, want: `['A}B']`,
		},
		{
			name:   "a produced quote with nothing left to close it",
			src:    `unset u; printf "[%s]" "${u:-x$'\x27'}"`,
			status: 1, diag: "bash: line 1: bad substitution: no closing `}' in \"${u:-x'}\"",
		},
		{
			name:   "a produced double quote",
			src:    `unset u; printf "[%s]" "${u:-x$'\x22'}"`,
			status: 1, diag: "bash: line 1: bad substitution: no closing `}' in \"${u:-x\"}\"",
		},
		{
			// A produced backslash quotes the brace rather than opening a run,
			// which is the same scan reaching the same end by another road.
			name:   "a produced backslash",
			src:    `unset u; printf "[%s]" "${u:-x$'\x5c'}"`,
			status: 1, diag: "bash: line 1: bad substitution: no closing `}' in \"${u:-x\\}\"",
		},
		{
			name: "a pattern operand is not re-read",
			src:  `v="'"; printf "[%s]" "${v/$'\x27'/x}"`, want: `[x]`,
		},
		{
			name: "a replacement operand is not re-read",
			src:  `w=Q; printf "[%s]" "${w/Q/$'\x27'}"`, want: `[']`,
		},
		{
			// The crossing: the escape belongs to what was *read* and the brace
			// to the run, so a body read outside POSIX mode and called inside
			// it converts the escape and then finds the `}` the mode stopped
			// protecting.
			name: "read outside POSIX mode and called inside it",
			src: "unset u\n" +
				`f(){ printf "[%s]" "${u:-x$'\x27'}"; }` + "\n" +
				"set -o posix\nf",
			want: `[x']`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), c.src)
			if st != c.status {
				t.Errorf("%q said status %d, want %d", c.src, st, c.status)
			}
			if c.diag != "" {
				// The whole line, location and all: a fragment check cannot
				// see a prefix that went missing in front of it. The
				// location is the harness's route rather than the wording —
				// real bash under `-c` names the path in `$0` there.
				wantWholeLines(t, out, c.diag)
				return
			}
			if out != c.want {
				t.Errorf("%q said %q, want %q", c.src, out, c.want)
			}
		})
	}
}
