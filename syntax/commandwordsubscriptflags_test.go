// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A subscript's flag group is read in two positions and one flag takes it off
// the first. See [Dialect.CommandWordSubscriptHasNoFlagGroup], where the rows
// are.

func subscriptFlagsEverywhere() Dialect {
	d := Core()
	d.ArraySubscript = true
	d.BareSubscript = true
	d.ArraySubscriptFlags = true
	return d
}

func subscriptFlagsInASubstitutionOnly() Dialect {
	d := subscriptFlagsEverywhere()
	d.CommandWordSubscriptHasNoFlagGroup = true
	return d
}

func TestAFlagGroupLeavesACommandWordAndStaysInASubstitution(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		// refused once the group has left a command word.
		gone bool
	}{
		{"a flag group in a command word", "b[(r)y]=Q\n", true},
		{"another letter, same position", "b[(i)x]=Q\n", true},
		{"a flag group on a string", "s[(r)l]=Q\n", true},
		// The group in the other position, which is the half that does not
		// move and the reason this is a flag of its own rather than
		// ArraySubscriptFlags going off.
		{"a flag group in a substitution", "echo ${b[(r)y]}\n", false},
		// And the control: a subscript with no group in it is untouched, so
		// a flag that had made a command word's subscript ordinary text
		// rather than taking one construct out of it would show here.
		{"an ordinary subscript", "b[2]=Q\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.src, subscriptFlagsEverywhere()); err != nil {
				t.Errorf("the dialect with the group in both positions refused it: %v", err)
			}
			_, err := Parse(tc.src, subscriptFlagsInASubstitutionOnly())
			if got := err != nil; got != tc.gone {
				t.Errorf("with the group gone from a command word: refused=%v (%v), want %v",
					got, err, tc.gone)
			}
		})
	}
}
