// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The tail of the field-splitting rule in this dialect, which is the one shell
// in the panel where a non-whitespace separator closing a value delimits
// instead of being absorbed.
//
// The substrate's tests name the axis; this one names the shell, because the
// answer here is the whole of what makes it visible — five shells and the
// specification's own preset give one field fewer for every value in this
// file. Measured 2026-09-07 on zsh 5.9.2 with a scratch HOME and ZDOTDIR.
//
// Each row asserts the field count *with* the fields, because the wrong answer
// is a plausible count at status 0: `[a]` and `[a][]` are the same characters
// once the boundaries are gone, and nothing downstream can tell that a `for`
// loop ran one iteration fewer.
const fields = `f(){ printf "%d:" "$#"; printf "[%s]" "$@"; }; `

func TestATrailingSeparatorDelimitsInThisDialect(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		// Reachable three ways, and the option is the first: this shell does
		// not split an unquoted expansion until it is asked to.
		{
			"under shwordsplit",
			"setopt shwordsplit\n" + fields + `IFS=:; v="a:"; f $v`,
			`2:[a][]`,
		},
		{
			"two separators at the tail",
			"setopt shwordsplit\n" + fields + `IFS=:; v="a::"; f $v`,
			`3:[a][][]`,
		},
		{
			"a PATH-shaped value",
			"setopt shwordsplit\n" + fields + `IFS=:; v="a:b:"; f $v`,
			`3:[a][b][]`,
		},
		{
			"a leading separator still delimits, and so does the trailing one",
			"setopt shwordsplit\n" + fields + `IFS=:; v=":a:"; f $v`,
			`3:[][a][]`,
		},
		{
			"nothing but a separator is two empty fields",
			"setopt shwordsplit\n" + fields + `IFS=:; v=":"; f $v`,
			`2:[][]`,
		},
		// The second route: the split flag, which splits whatever the option
		// says.
		{
			"the split flag, unquoted",
			fields + `IFS=:; v="a:"; f ${=v}`,
			`2:[a][]`,
		},
		// The third: an unquoted command substitution, which this shell
		// splits as it comes — so the divergence is live here with no option
		// set and no flag written.
		{
			"an unquoted command substitution",
			fields + `IFS=:; f $(printf "a:")`,
			`2:[a][]`,
		},
		// And `read` into an array, which feeds the same splitter.
		{
			"read into an array",
			`IFS=:; printf 'a:b:\n' | { read -A arr; printf "%d:" ${#arr}; printf "[%s]" "${arr[@]}"; }`,
			`3:[a][b][]`,
		},
		// The guards, and they are what a fix in the wrong place breaks.
		// Whitespace is absorbed at both ends in this shell as well, so a
		// change that stopped absorbing anything would break the shell it was
		// trying to match.
		{
			"whitespace at the tail is still absorbed",
			"setopt shwordsplit\n" + fields + `v=" a "; f $v`,
			`1:[a]`,
		},
		{
			"and a whitespace run in a mixed IFS",
			"setopt shwordsplit\n" + fields + `IFS=" :"; v="a  "; f $v`,
			`1:[a]`,
		},
		{
			"but the separator in front of it is not hidden by it",
			"setopt shwordsplit\n" + fields + `IFS=" :"; v="a: "; f $v`,
			`2:[a][]`,
		},
		{
			"an empty value is no field at all",
			"setopt shwordsplit\n" + fields + `IFS=:; v=""; f $v`,
			`0:[]`,
		},
		{
			"an escaped separator is data, so read keeps one field",
			`IFS=:; printf 'a\\:\n' | { read -A arr; printf "%d:" ${#arr}; printf "[%s]" "${arr[@]}"; }`,
			`1:[a:]`,
		},
		// Without the option there is no split to have a tail, which is the
		// row that says this is the splitter's question and not a rule about
		// values.
		{
			"and with no splitting there is no tail",
			fields + `IFS=:; v="a:"; f $v`,
			`1:[a:]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
