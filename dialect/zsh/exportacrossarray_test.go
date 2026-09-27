// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A name **re-created** by an array assignment loses the export attribute,
// and one merely retyped does not.
//
// It is not a rule of its own and gets no axis of its own: the panel's split
// over the export attribute is exactly the panel's split over what counts as
// a re-creation, which three measured axes already hold. The export attribute
// was simply in neither the list a re-creation clears nor the guard that asks
// the question (#4676).
//
// Measured 2026-09-26 on zsh 5.9.2 under `-f`; see
// Runner.clearAttributesAReCreationDrops for the nine cells across the three
// columns.
func TestAReCreatedNameLosesTheExportAttribute(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a literal over an exported scalar",
			`export a=1; a=(x y); typeset -p a`,
			"typeset -a a=( x y )\n",
		},
		{
			// An append over a scalar re-creates the name in this column and
			// not in ksh93, which is the row that says the rule cannot be
			// "an array literal".
			"an append over an exported scalar",
			`export a=1; a+=(x y); typeset -p a`,
			"typeset -a a=( 1 x y )\n",
		},
		{
			// A literal over a name that is *already* an array re-creates it
			// in ksh93 and not here, which is the row that says the rule
			// cannot be "the kind changed" either.
			"a literal over an exported array keeps it",
			`export a=(p q); a=(x y); typeset -p a`,
			"typeset -ax a=( x y )\n",
		},
		{
			// A declaration's own operand is not a re-creation here.
			"a declaration keeps it",
			`export a=1; typeset -a a=(x y); typeset -p a`,
			"typeset -ax a=( x y )\n",
		},
		{
			"and so does a table's",
			`export a=1; typeset -A a=(k v); typeset -p a`,
			"typeset -Ax a=( [k]=v )\n",
		},
		// The two controls from the issue, and both are what say this is
		// about the assignment rather than about arrays and exports.
		{
			"export applied to a name that is already an array keeps it",
			`a=(x y); export a; typeset -p a`,
			"typeset -ax a=( x y )\n",
		},
		{
			// The attribute is really off and not merely withheld from one
			// listing: `export` names it before the assignment and not
			// after. Written as a pair rather than as an absence, so the row
			// is seen to produce the positive it claims.
			"the export listing agrees, before and after",
			`export a=1
			 export | while IFS= read -r l; do case $l in (*a=*) print -r -- "before:[$l]";; esac; done
			 a=(x y)
			 export | while IFS= read -r l; do case $l in (*a=*) print -r -- "after:[$l]";; esac; done
			 print -r -- "done"`,
			"before:[a=1]\ndone\n",
		},
		{
			"and the type word follows the listing",
			`export a=1; print -r -- "${(t)a}"; a=(x y); print -r -- "${(t)a}"`,
			"scalar-export\narray\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
