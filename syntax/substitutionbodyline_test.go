// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// What the parser does for [Dialect.SubstitutionBodyRead]: it gathers, and it
// gathers per spelling.
//
// A body cannot be parsed here — it is read with the alias tables, the
// options and the dialect as the *shell* holds them at that moment, and none
// of those are in this package — so the answer that crosses is the list, and
// the reader takes the bodies off it before running the line. See
// [File.Substitutions] (#2857).
func TestTheSpansAFlagGathersAreTheSpellingsItNames(t *testing.T) {
	withLine := func(v SubstitutionBodyRead) Dialect {
		d := Core()
		d.SubstitutionBodyRead = v
		return d
	}
	for _, c := range []struct {
		name string
		read SubstitutionBodyRead
		src  string
		want []string
	}{
		{
			"nothing is gathered under the core answer",
			SubstitutionBodyReadWhenItRuns,
			"v=$(a); w=`b`\n", nil,
		},
		{
			"the newer spelling alone",
			NewerSubstitutionBodyReadWithItsLine,
			"v=$(a); w=`b`\n",
			[]string{"a"},
		},
		{
			"both spellings",
			EverySubstitutionBodyReadWithItsLine,
			"v=$(a); w=`b`\n",
			[]string{"a", "b"},
		},
		{
			// The outer body alone, because its text is not parsed here: a
			// body inside it is gathered by the parse the *reader* makes of
			// that text, onto that File's own list. The recursion is the
			// reader's and the list is one level deep.
			"a nested body is left to the read of the body that holds it",
			NewerSubstitutionBodyReadWithItsLine,
			"v=$(echo $(a))\n",
			[]string{"echo $(a)"},
		},
		{
			// An operand's own substitution is the script's to read, so it
			// is gathered like any other.
			"a substitution in an operand",
			NewerSubstitutionBodyReadWithItsLine,
			"echo \"${v-$(a)}\"\n",
			[]string{"a"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := Parse(c.src, withLine(c.read))
			if err != nil {
				t.Fatalf("parse %q: %v", c.src, err)
			}
			got := make([]string, 0, len(f.Substitutions))
			for _, s := range f.Substitutions {
				got = append(got, s.Value)
			}
			if len(got) != len(c.want) {
				t.Fatalf("parsed %q: gathered %q, want %q", c.src, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("parsed %q: gathered %q, want %q", c.src, got, c.want)
				}
			}
		})
	}
}

// TestALineHandsBackOnlyItsOwnSubstitutions is the unit the reader works in.
//
// The list is drained by [Parser.NextLine], because a dialect that reads a
// body with its line reads it with *that* line and not with the file: a
// script whose first line runs before the second is refused is the measured
// shape, and a front end reads one line at a time.
func TestALineHandsBackOnlyItsOwnSubstitutions(t *testing.T) {
	d := Core()
	d.SubstitutionBodyRead = NewerSubstitutionBodyReadWithItsLine
	p := NewParser("v=$(a)\nw=$(b)\n", d)
	for _, want := range []string{"a", "b"} {
		f, ok := p.NextLine()
		if !ok {
			t.Fatalf("no line for %q", want)
		}
		if len(f.Substitutions) != 1 || f.Substitutions[0].Value != want {
			t.Fatalf("line for %q gathered %v", want, f.Substitutions)
		}
	}
}

// TestAGatheredListIsNotPartOfTheProgram is the round-trip half.
//
// [File.Substitutions] is an index over the tree rather than a part of it:
// every span it names is compared where it stands in the word that holds it,
// and printing may put two statements on lines the source did not, which
// moves the list and moves nothing about the program. See
// [SameProgram].
func TestAGatheredListIsNotPartOfTheProgram(t *testing.T) {
	d := Core()
	d.SubstitutionBodyRead = NewerSubstitutionBodyReadWithItsLine
	with, err := Parse("v=$(a)\n", d)
	if err != nil {
		t.Fatal(err)
	}
	if len(with.Substitutions) == 0 {
		t.Fatal("nothing gathered, so this proves nothing")
	}
	without, err := Parse("v=$(a)\n", Core())
	if err != nil {
		t.Fatal(err)
	}
	if len(without.Substitutions) != 0 {
		t.Fatalf("the core answer gathered %v", without.Substitutions)
	}
	if why, same := SameProgram(with, without); !same {
		t.Errorf("SameProgram = %q, want the list to be spelling only", why)
	}
}
