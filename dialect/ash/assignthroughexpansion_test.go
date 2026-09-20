// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "testing"

// An assignment written inside an expansion, onto a parameter no assignment
// can land on, is refused in *this* shell's words rather than in zsh's.
//
// `Diagnostics.AssignThroughExpansionBadName` falls back to zsh's `not an
// identifier: @` when a dialect leaves it unset, because zsh is the one shell
// whose grammar has `${name::=word}` — the operator that reaches this question
// on every name. ash has no such operator and reaches the question only through
// the conditional `${name:=word}` every dialect has, so the fallback handed
// this column the sentence of the shell furthest from it and nothing failed:
// an unset Diagnostics value is a wording nobody measured, not a wording nobody
// needs (#3910).
//
// Measured 2026-09-20, BusyBox v1.37.0 in the digest-pinned alpine image the
// oracle reaches, each line alone in a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`. Every row below was `<file>: line 1:`
// plus the sentence asserted here, at status 2.
//
// The assertions are on the **whole** of what the shell wrote, not on a
// substring: this is a wording bug, and `not an identifier: @` and
// `@: bad variable name` share no substring worth matching on — but the
// difference between the name and the whole word does, and a `Contains` on
// `@: bad variable name` would pass for a wording that wrote the word around
// it too.
func TestAnAssignmentThroughAnExpansionIsRefusedInThisShellsWords(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			// The row #3910 was reported on.
			"the whole list",
			`echo "${@:=abc}"`,
			"ash: @: bad variable name\n", 2,
		},
		{
			// Its joining spelling, which this shell refuses the same way.
			"and its joining spelling",
			`echo "${*:=abc}"`,
			"ash: *: bad variable name\n", 2,
		},
		{
			// The discriminator between the two verbs the field takes. This
			// shell names the parameter and not the word the expansion stands
			// in — ksh93 is the only column that blames the whole word, and a
			// `%[2]s` here would write `x${@:=abc}y` back instead.
			"a word with text around the expansion",
			`echo "x${@:=abc}y"`,
			"ash: @: bad variable name\n", 2,
		},
		{
			// A positional, which is the same sentence with the other
			// parameter in it: the sigil is left off in this column, so a
			// wording carrying one would write `$1` here.
			"a positional parameter",
			`echo "${1:=abc}"`,
			"ash: 1: bad variable name\n", 2,
		},
		{
			// The control, and it is the row that says the check is a run-time
			// one rather than a reading of the text: with the parameter set the
			// operator never fires, so a shell that refused this word on sight
			// would fail here while passing every row above it.
			"the parameter is there, so the operator never fires",
			`set -- p; echo "${@:=abc}"`,
			"p\n", 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runIn(t, tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("got %q (status %d), want %q at %d", out, st, tc.want, tc.status)
			}
		})
	}
}
