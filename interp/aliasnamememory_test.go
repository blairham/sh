// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// One dialect keeps a name in the table after `alias` has *named* it, so a
// second `unalias` of that name succeeds where the other four report there
// was nothing to remove — see Semantics.AliasRemembersTheNamesItNames.
//
// Both answers are asked over the same snippets, because that is the whole of
// what the axis is: a table that answered the same either way would be a
// field nothing reads.
//
// The two "not found" answers are off in these suites so that every row is
// about a *status*. `alias` and `unalias` each have an axis of their own for
// whether they speak, they are not this one, and a complaint in the buffer
// would make these rows read as tests of the wording.
func rememberedNames(yes Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.AliasRemembersTheNamesItNames = yes
		s.AliasReportsNotFound = No
		s.UnaliasReportsNotFound = No
	}
}

// TestUnaliasCountsANameAliasHasNamed is the dialect that remembers.
func TestUnaliasCountsANameAliasHasNamed(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// The issue's own row, and one past it: the name does not go when
		// the value does, so the third removal succeeds as well.
		{
			"a removed alias is removable again",
			`alias h=1; unalias h; echo $?; unalias h; echo $?; unalias h; echo $?`,
			"0\n0\n0\n",
		},
		// Naming is enough. A lookup that *failed* leaves the name behind,
		// which is what makes this wider than "an alias that was defined".
		{
			"a failed lookup leaves the name",
			`alias z; echo $?; unalias z; echo $?`,
			"1\n0\n",
		},
		{
			"the second name of a two-name lookup",
			`alias y z; unalias z; echo $?`,
			"0\n",
		},
		// The control: a name nothing ever named fails, and a failed
		// `unalias` does not put one there either.
		{
			"a name nobody named",
			`unalias q; echo $?; unalias q; echo $?`,
			"1\n1\n",
		},
		// `unalias -a` clears the names along with the table.
		{
			"removing everything forgets the names",
			`alias h=1; unalias h; unalias -a; unalias h; echo $?`,
			"1\n",
		},
		// A remembered name is not an alias: no lookup finds it and no
		// listing has it. Only `unalias` can see it at all.
		{
			"a remembered name is in no listing",
			`alias h=1; unalias h; alias h; echo $?; alias; echo done`,
			"1\ndone\n",
		},
		// And not inside a subshell, measured — the subshell's own
		// remembered name does not count there and neither does the
		// parent's.
		{
			"the parent's name does not count inside a subshell",
			`alias h=1; unalias h; ( unalias h; echo $? )`,
			"1\n",
		},
		{
			"nor a name the subshell itself named",
			`( alias z; unalias z; echo $? )`,
			"1\n",
		},
		// And the other half of the same boundary, which is the one the
		// axis's doc comment used to record as left undone: what a `( … )`
		// or a `$( … )` *named* is remembered out here afterwards, where
		// what it defined is not. See Runner.adoptAliasNames.
		{
			"a name the parentheses looked up counts in the parent",
			`( alias z ); unalias z; echo $?`,
			"0\n",
		},
		{
			"a name a command substitution looked up counts too",
			`x=$(alias z); unalias z; echo $?`,
			"0\n",
		},
		{
			"a name the parentheses defined counts, and the value does not",
			`( alias z=1 ); alias z; echo $?; unalias z; echo $?`,
			"1\n0\n",
		},
		{
			"nested parentheses reach the top",
			`( ( alias z ) ); unalias z; echo $?`,
			"0\n",
		},
		// The carry-over is the two of them and not every subshell. A
		// pipeline element is one and does not carry, which is what says
		// this is a boundary rather than "a subshell".
		{
			"a pipeline element's name does not",
			`alias z | :; unalias z; echo $?`,
			"1\n",
		},
		{
			"and the parent's still does not count inside one",
			`( alias z ); ( unalias z; echo $? )`,
			"1\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, _ := aliasRun(t, rememberedNames(Yes), Diagnostics{}, c.src)
			if got != c.want {
				t.Errorf("%s =\n%q\nwant\n%q", c.src, got, c.want)
			}
		})
	}
}

// TestUnaliasForgetsANameEverywhereElse is the same snippets under the answer
// the other four columns give, which is what makes the suite above a
// measurement of an axis rather than of a shell.
func TestUnaliasForgetsANameEverywhereElse(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a removed alias is gone",
			`alias h=1; unalias h; echo $?; unalias h; echo $?`,
			"0\n1\n",
		},
		{
			"a failed lookup leaves nothing",
			`alias z; echo $?; unalias z; echo $?`,
			"1\n1\n",
		},
		{
			"removing everything is the same either way",
			`alias h=1; unalias h; unalias -a; unalias h; echo $?`,
			"1\n",
		},
		// And nothing crosses the boundary where nothing is remembered in
		// the first place, which is what keeps adoptAliasNames behind the
		// axis rather than on the common path.
		{
			"a name the parentheses looked up is gone with them",
			`( alias z ); unalias z; echo $?`,
			"1\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, _ := aliasRun(t, rememberedNames(No), Diagnostics{}, c.src)
			if got != c.want {
				t.Errorf("%s =\n%q\nwant\n%q", c.src, got, c.want)
			}
		})
	}
}
