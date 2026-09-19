// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// An option word ends the options in every column and ends the *lookup* in
// one — see Semantics.AliasOptionEndsTheLookup. A name behind one is named
// there and nothing else: no line, no complaint, and 0 whether the table
// holds it or not.
//
// Both answers are asked over the same snippets, which is the whole of what
// the axis is. The rows that speak are the ones that say the separator is not
// merely being swallowed: `alias r` writes the entry's line either way, and
// only `alias -- r` parts them.
func aliasSeparator(yes Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.AliasOptionEndsTheLookup = yes
	}
}

// TestASeparatorEndsTheAliasLookup is the dialect whose separator does.
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

// And it is every option and not the separator alone (#3677).
//
// The axis was written from `--` because that was the shape measured, and
// the letters answer the same way. Measured 2026-09-18 on ksh93u+ 2012-08-01,
// script files under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input
// on /dev/null:
//
//	alias zz=1; alias zz          zz=1, 0            the control
//	alias zz=1; alias -p zz       silent, 0
//	alias zz=1; alias -t zz       silent, 0
//	alias zz=1; alias -x zz       silent, 0
//	alias zz=1; alias -- zz       silent, 0
//	alias -p nosuch               silent, 0          where bare is 1
//	alias -p nosuch zz            silent, 0          and both operands
//	alias -p zz=2; alias zz       zz=2, 0            a definition still lands
//	ls; alias -t ls               silent, 0          even where `-t` lists it
//	ls; alias -t                  ls=/bin/ls, 0
//
// The last two are the pair that says the silence is not a narrower table
// being searched: the bare `-t` listing has the entry and `-t ls` will not
// report it. And `alias -p zz=2` defining says what the option ends is the
// reading of an operand as a *question*, leaving the naming behind it —
// which is what the separator rows already said.
func TestAnOptionLetterEndsTheAliasLookupToo(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the prefixed listing of a name it has",
			`alias r=1; alias -p r; echo $?`,
			"0\n",
		},
		{
			"and of a name it has not got",
			`alias -p nosuch; echo $?`,
			"0\n",
		},
		{
			"two operands behind one option",
			`alias r=1; alias -p nosuch r; echo $?`,
			"0\n",
		},
		{
			// An option *and* the separator in one call, which is the shape
			// the axis was written from with a letter added in front of it.
			"an option and the separator together",
			`alias r=1; alias -p -- r; echo $?`,
			"0\n",
		},
		{
			"a definition behind an option still defines",
			`alias -p z=1; echo $?; alias z`,
			"0\nz='1'\n",
		},
		{
			// The control that keeps this from being "the listing is
			// broken": an option with no operand behind it is the listing,
			// and it speaks.
			"an option with nothing behind it is the listing",
			`alias r=1; alias -p; echo $?`,
			"alias r='1'\n0\n",
		},
		{
			// And the same lookup with no option in front of it.
			"the same lookup with no option",
			`alias r=1; alias r; echo $?`,
			"r='1'\n0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, _ := aliasRun(t, aliasSeparator(Yes), Diagnostics{AliasNotFound: "%[2]s: alias not found"}, c.src)
			if got != c.want {
				t.Errorf("%s =\n%q\nwant\n%q", c.src, got, c.want)
			}
		})
	}
	// The other answer reads the same words as a lookup, which is what says
	// every row above is the axis and not the option reader losing an
	// operand.
	for _, c := range []struct{ name, src, want string }{
		{
			"the prefixed listing reports there",
			`alias r=1; alias -p r; echo $?`,
			"alias r='1'\n0\n",
		},
		{
			"and complains about a name it has not got",
			`alias -p nosuch; echo $?`,
			"testsh: nosuch: alias not found\n1\n",
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
