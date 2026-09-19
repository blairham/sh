// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// `alias -p` is the whole listing and stops reading, in the one column that
// answers so — see Semantics.AliasPrintOptionIgnoresItsOperands.
//
// **Every row here is written with two aliases defined**, and that is the
// discriminator rather than a flourish. With one alias defined, `alias -p a`
// cannot be told from a lookup of `a` that was merely silent about a miss;
// with two it can, because the answer holds a name nobody asked for.
//
// The same shape is what made the issue's first reading — "`-p` silences the
// not-found report" — fit every probe it was measured on and still be wrong.
func aliasPrintIgnores(yes Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.AliasPrintOptionIgnoresItsOperands = yes
	}
}

const twoAliases = `alias a=1 b=2; `

func TestThePrintOptionThrowsItsOperandsAway(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a name the table holds is not a filter: the whole table comes back",
			twoAliases + `alias -p a; echo $?`,
			"alias a='1'\nalias b='2'\n0\n",
		},
		{
			"a name it has not got is neither reported nor counted",
			twoAliases + `alias -p nosuch; echo $?`,
			"alias a='1'\nalias b='2'\n0\n",
		},
		{
			"a definition behind the letter defines nothing",
			twoAliases + `alias -p z=9; echo $?; alias z`,
			"alias a='1'\nalias b='2'\n0\ntestsh: z: alias not found\n",
		},
		{
			"and does not redefine a name the table already holds",
			twoAliases + `alias -p a=9; alias a`,
			"alias a='1'\nalias b='2'\na='1'\n",
		},
		{
			"a name this shell would refuse is not checked either",
			twoAliases + `alias -p 'a b'=echo; echo $?`,
			"alias a='1'\nalias b='2'\n0\n",
		},
		{
			"several operands are one listing, not one each",
			twoAliases + `alias -p a b nosuch; echo $?`,
			"alias a='1'\nalias b='2'\n0\n",
		},
		// The controls. The same operands without the letter filter, report
		// and define, and a bare separator is not this letter.
		{
			"the same name with nothing in front of it",
			twoAliases + `alias a; echo $?`,
			"a='1'\n0\n",
		},
		{
			"and behind a separator",
			twoAliases + `alias -- a; echo $?`,
			"a='1'\n0\n",
		},
		{
			"a missing name behind the separator still reports",
			twoAliases + `alias -- nosuch; echo $?`,
			"testsh: nosuch: alias not found\n1\n",
		},
		{
			"the letter with nothing behind it is the same listing",
			twoAliases + `alias -p; echo $?`,
			"alias a='1'\nalias b='2'\n0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, _ := aliasRun(t, aliasPrintIgnores(Yes), Diagnostics{AliasNotFound: "%[2]s: alias not found"}, c.src)
			if got != c.want {
				t.Errorf("%s =\n%q\nwant\n%q", c.src, got, c.want)
			}
		})
	}
}

// The other answer over the same snippets, which is the mutation: with the
// field off, `-p` is a listing *shape* and the operands are obeyed.
func TestThePrintOptionObeysItsOperandsWhereItIsNotTheWholeListing(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a name the table holds is the only line",
			twoAliases + `alias -p a; echo $?`,
			"alias a='1'\n0\n",
		},
		{
			"a name it has not got is reported and counted",
			twoAliases + `alias -p nosuch; echo $?`,
			"testsh: nosuch: alias not found\n1\n",
		},
		{
			"a definition behind the letter defines",
			twoAliases + `alias -p z=9; echo $?; alias z`,
			"0\nz='9'\n",
		},
		{
			"and the bare letter is the same whole listing either way",
			twoAliases + `alias -p; echo $?`,
			"alias a='1'\nalias b='2'\n0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, _ := aliasRun(t, aliasPrintIgnores(No), Diagnostics{AliasNotFound: "%[2]s: alias not found"}, c.src)
			if got != c.want {
				t.Errorf("%s =\n%q\nwant\n%q", c.src, got, c.want)
			}
		})
	}
}

// And this is not the field that ends the lookup, which is the probe that can
// tell the two silences apart.
//
// Both are quiet about a name the table has not got, so a reader could fold
// one into the other and every miss row above would still pass. They part on
// a name the table *holds*: one writes the whole table for it and the other
// writes nothing at all.
func TestEndingTheLookupAndThrowingTheOperandsAwayAreNotOneField(t *testing.T) {
	dg := Diagnostics{AliasNotFound: "%[2]s: alias not found"}
	ends := func(s *Semantics) { s.AliasOptionEndsTheLookup = Yes }
	if got, _ := aliasRun(t, ends, dg, twoAliases+`alias -p a; echo $?`); got != "0\n" {
		t.Errorf("the lookup-ending field: %q, want nothing listed at 0", got)
	}
	if got, _ := aliasRun(t, aliasPrintIgnores(Yes), dg, twoAliases+`alias -p a; echo $?`); got != "alias a='1'\nalias b='2'\n0\n" {
		t.Errorf("the operand-discarding field: %q, want the whole table at 0", got)
	}
	// And they part on a definition too: one still defines behind the
	// option and the other does not.
	if got, _ := aliasRun(t, ends, dg, `alias -p z=9; alias z`); got != "z='9'\n" {
		t.Errorf("the lookup-ending field: %q, want the definition to land", got)
	}
	if got, _ := aliasRun(t, aliasPrintIgnores(Yes), dg, `alias -p z=9; alias z`); got != "testsh: z: alias not found\n" {
		t.Errorf("the operand-discarding field: %q, want nothing defined", got)
	}
}
