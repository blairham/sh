// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// One dialect has an `-x` for `alias`, which marks an entry and narrows a
// listing to the entries it has marked — see Semantics.AliasHasExportOption.
//
// The letter is not a kind: an `-x` alias is in the plain listing beside the
// rest, is looked up by the same name, and expands the same way. What the
// letter adds is a mark and a filter, and the suite below is written so that
// each row separates the two.
func exportOption(yes Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.AliasHasExportOption = yes
		// The lookup rows are about what `-x` prints, and `alias` speaking
		// or not about a missing name is a different axis. So is `unalias`
		// speaking, which the mark rows reach on their way to a listing.
		s.AliasReportsNotFound = No
		s.UnaliasReportsNotFound = No
	}
}

// TestAliasExportOptionMarksAndFilters is the dialect that has the letter.
func TestAliasExportOptionMarksAndFilters(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// The issue's own row: the call succeeds and defines an ordinary
		// alias, which the plain listing shows.
		{
			"a definition is taken and is an ordinary alias",
			`alias -x xx=1; echo $?; alias xx`,
			"0\nxx='1'\n",
		},
		// The filter, which is the half a mark alone could not show.
		{
			"the listing is narrowed to the marked entries",
			`alias a=1; alias -x b=2; alias -x`,
			"b='2'\n",
		},
		{
			"and the plain listing still has both",
			`alias a=1; alias -x b=2; alias`,
			"a='1'\nb='2'\n",
		},
		// A name already defined is *marked* rather than looked up: nothing
		// is printed and the status is 0.
		{
			"a bare name marks what the table holds",
			`alias a=1; alias -x a; echo $?; alias -x`,
			"0\na='1'\n",
		},
		// And a name the table does not hold is not a complaint and not a
		// definition either — 0, with nothing listed by either form.
		{
			"a bare name the table has not got defines nothing",
			`alias -x zz; echo $?; alias -x; alias; echo done`,
			"0\ndone\n",
		},
		// The mark is on the name rather than on the value: a plain
		// redefinition keeps it.
		{
			"the mark survives a redefinition",
			`alias -x e=3; alias e=4; alias -x`,
			"e='4'\n",
		},
		// A removal takes it off, because the entry goes with it.
		{
			"a removal takes the mark off",
			`alias -x e=3; unalias e; alias -x; echo done`,
			"done\n",
		},
		// `-p` and `-x` are a listing form and a filter, so they combine.
		{
			"the print form of the narrowed listing",
			`alias a=1; alias -x e=3; alias -px`,
			"alias e='3'\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, _ := aliasRun(t, exportOption(Yes), Diagnostics{}, c.src)
			if got != c.want {
				t.Errorf("%s =\n%q\nwant\n%q", c.src, got, c.want)
			}
		})
	}
}

// TestAliasExportOptionIsRefusedWhereItIsNotAnOption is the same letter under
// the answer the rest of the panel gives, which is what makes the suite above
// a measurement of an axis: the letter is refused, and the refusal names it.
func TestAliasExportOptionIsRefusedWhereItIsNotAnOption(t *testing.T) {
	got, _ := aliasRun(t, exportOption(No), Diagnostics{}, `alias -x xx=1; echo $?; alias xx; echo $?`)
	// The refusal, the status it leaves — 2, this tree's answer for a bad
	// option word — and the proof that nothing was defined behind it.
	const want = "testsh: alias: -x: invalid option\n2\n1\n"
	if got != want {
		t.Errorf("alias -x with no such letter =\n%q\nwant\n%q", got, want)
	}
}

// And the two answers meet: a name `alias -x` named is remembered by the
// dialect that remembers names, even though the call defined nothing. That is
// the one place the two axes of #2926 and #2927 touch, and it is measured —
// `alias -x zz; unalias zz` is 0 on ksh93u+.
func TestABareExportOperandLeavesTheNameBehind(t *testing.T) {
	both := func(s *Semantics) {
		s.AliasHasExportOption = Yes
		s.AliasRemembersTheNamesItNames = Yes
		s.UnaliasReportsNotFound = No
	}
	got, _ := aliasRun(t, both, Diagnostics{}, `alias -x zz; unalias zz; echo $?`)
	if want := "0\n"; got != want {
		t.Errorf("alias -x zz; unalias zz = %q, want %q", got, want)
	}
}

// A mark put on a name the table has not got is a state of its own: no alias
// is defined, nothing is looked up, and the only reader is the **prefixed**
// listing, which writes the prefix for it and then nothing at all — no name,
// no `=` and no newline, so the entry after it glues onto the same line. See
// Runner.markedAliasNames for the measurement this reproduces.
//
// Written as one suite rather than a row in the two above because it is the
// listing's bytes that are the answer here, and the rows that print *nothing*
// are as load-bearing as the one that prints the glue: a mark with no value
// is in the plain listing and in the narrowed one alike is not there.
func TestAMarkWithNoValueBehindItIsOnlyInThePrefixedListing(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the prefixed listing writes the prefix and glues",
			`alias a=1; alias -x b; alias c=3; alias -p`,
			"alias a='1'\nalias alias c='3'\n",
		},
		{
			"and it sorts where the name would have sorted",
			`alias a=1; alias -x zz; alias -p`,
			"alias a='1'\nalias ",
		},
		{
			"the narrowed prefixed listing has it too",
			`alias a=1; alias -x mm; alias -px`,
			"alias ",
		},
		{
			"the plain listing has not",
			`alias a=1; alias -x mm; alias`,
			"a='1'\n",
		},
		{
			"nor has the narrowed one",
			`alias a=1; alias -x mm; alias -x`,
			"",
		},
		{
			"nor is the name an alias",
			`alias -x mm; alias mm; echo $?`,
			"1\n",
		},
		// The two ways the mark goes, both measured: a definition takes it
		// over and a removal drops it, and the listing is whole afterwards.
		{
			"a definition takes the mark over",
			`alias -x mm; alias mm=2; alias -p`,
			"alias mm='2'\n",
		},
		{
			"and it is an exported entry afterwards",
			`alias -x mm; alias mm=2; alias -x`,
			"mm='2'\n",
		},
		{
			"a removal drops it",
			`alias a=1; alias -x mm; unalias mm; alias -p`,
			"alias a='1'\n",
		},
		{
			"and so does removing everything",
			`alias -x mm; unalias -a; alias -p`,
			"",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, _ := aliasRun(t, exportOption(Yes), Diagnostics{}, c.src)
			if got != c.want {
				t.Errorf("%s =\n%q\nwant\n%q", c.src, got, c.want)
			}
		})
	}
}
