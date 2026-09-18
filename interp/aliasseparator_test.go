// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// `--` ends the options in every column and ends the *lookup* in one — see
// Semantics.AliasSeparatorEndsTheLookup. A name behind the separator is
// named there and nothing else: no line, no complaint, and 0 whether the
// table holds it or not.
//
// Both answers are asked over the same snippets, which is the whole of what
// the axis is. The rows that speak are the ones that say the separator is not
// merely being swallowed: `alias r` writes the entry's line either way, and
// only `alias -- r` parts them.
func aliasSeparator(yes Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.AliasSeparatorEndsTheLookup = yes
	}
}

// TestASeparatorEndsTheAliasLookup is the dialect whose `--` does.
func TestASeparatorEndsTheAliasLookup(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a name the table has not got is not reported",
			`alias -- nosuch; echo $?`,
			"0\n",
		},
		{
			"and a name it has is not listed either",
			`alias r=1; alias -- r; echo $?`,
			"0\n",
		},
		{
			"a definition behind the separator still defines",
			`alias -- z=1; echo $?; alias z`,
			"0\nz='1'\n",
		},
		{
			"the prefixed listing is silenced with the rest",
			`alias r=1; alias -p -- r; echo $?`,
			"0\n",
		},
		{
			"a dash-word behind it is an operand and is silent too",
			`alias -- -q; echo $?`,
			"0\n",
		},
		{
			"the separator with nothing behind it is the plain listing",
			`alias r=1; alias --; echo $?`,
			"r='1'\n0\n",
		},
		// The control is one line up in every row above: without the
		// separator the same operand speaks and counts.
		{
			"the same lookup without the separator",
			`alias nosuch; echo $?`,
			"testsh: nosuch: alias not found\n1\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, _ := aliasRun(t, aliasSeparator(Yes), Diagnostics{AliasNotFound: "%[2]s: alias not found"}, c.src)
			if got != c.want {
				t.Errorf("%s =\n%q\nwant\n%q", c.src, got, c.want)
			}
		})
	}
}

// TestASeparatorOnlyEndsTheOptions is the same snippets under the answer the
// other four columns give, where `--` ends the options and the operand behind
// it is looked up exactly as one written without it.
func TestASeparatorOnlyEndsTheOptions(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a name the table has not got is reported",
			`alias -- nosuch; echo $?`,
			"testsh: nosuch: alias not found\n1\n",
		},
		{
			"and a name it has is listed",
			`alias r=1; alias -- r; echo $?`,
			"r='1'\n0\n",
		},
		{
			"a definition behind the separator still defines",
			`alias -- z=1; echo $?; alias z`,
			"0\nz='1'\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, _ := aliasRun(t, aliasSeparator(No), Diagnostics{AliasNotFound: "%[2]s: alias not found"}, c.src)
			if got != c.want {
				t.Errorf("%s =\n%q\nwant\n%q", c.src, got, c.want)
			}
		})
	}
}
