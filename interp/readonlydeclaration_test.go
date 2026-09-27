// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Which of the two things `readonly` is — see Semantics.ReadonlyWord. Named
// for the axis and never for a shell.
//
// Every row states a letter or a shape and asks for it under **both**
// readings, because the defect this axis closes is not "a letter was
// missing": it is that a word with an operand loop of its own drifts from the
// word it is a second name for. A row that only asserted the declaration
// reading would pass just as well against a `readonly` that had grown a
// thirteenth branch of its own.
func TestReadonlyWordDecidesWhetherTheLettersAreADeclarations(t *testing.T) {
	for _, tc := range []struct {
		name, src        string
		asDeclaration    func(out string) bool
		asAttribute      func(out string) bool
		whyDeclaration   string
		whyAttributeToo  string
		declarationSetup func(*Semantics)
	}{
		{
			name: "an attribute letter the declaration word has",
			src:  `readonly -i v=2+3; echo "[$v]"`,
			asDeclaration: func(out string) bool {
				return strings.Contains(out, "[5]")
			},
			asAttribute: func(out string) bool {
				return strings.Contains(out, "[2+3]")
			},
			whyDeclaration: "the letter is the declaration's and reaches the integer " +
				"attribute, so the value is arithmetic",
			whyAttributeToo: "the letter is in the set and the loop that took it records " +
				"nothing, which is the accepted-and-ignored shape the axis exists " +
				"to stop being the only way to widen the set",
		},
		{
			name: "the kind gate a special parameter meets",
			src:  `readonly -a SPECIAL; echo "after st=$?"`,
			asDeclaration: func(out string) bool {
				return strings.Contains(out, "can't change type") &&
					!strings.Contains(out, "after")
			},
			asAttribute: func(out string) bool {
				return strings.Contains(out, "after st=0")
			},
			whyDeclaration: "the operands go through the declaration's loop, so the gate " +
				"that loop meets is this word's too",
			whyAttributeToo: "the POSIX reading freezes the name it is given and asks nothing " +
				"about what kind it is",
		},
		{
			name: "the freeze itself, which neither reading may lose",
			src:  `readonly v=1; v=2; echo "reached v=$v"`,
			asDeclaration: func(out string) bool {
				return strings.Contains(out, "readonly variable") &&
					!strings.Contains(out, "reached")
			},
			asAttribute: func(out string) bool {
				return strings.Contains(out, "readonly variable") &&
					!strings.Contains(out, "reached")
			},
			whyDeclaration:  "the word is the `r` letter however the line was written",
			whyAttributeToo: "and it is POSIX's freeze under the other reading",
		},
		{
			name: "a letter in neither set is still a bad option",
			src:  `readonly -Q v; echo "after st=$?"`,
			asDeclaration: func(out string) bool {
				return strings.Contains(out, "-Q") && strings.Contains(out, "option")
			},
			asAttribute: func(out string) bool {
				return strings.Contains(out, "-Q") && strings.Contains(out, "option")
			},
			whyDeclaration: "the control: the declaration reading widens the set and does " +
				"not open it, so a letter outside it is refused as it always was",
			whyAttributeToo: "and the same under the reading that was never widened",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, reading := range []struct {
				word ReadonlyWordReading
				want func(string) bool
				why  string
			}{
				{ReadonlyWordIsTheDeclaration, tc.asDeclaration, tc.whyDeclaration},
				{ReadonlyWordIsAnAttribute, tc.asAttribute, tc.whyAttributeToo},
			} {
				out, _ := run(t, tc.src, func(r *Runner) {
					s := permissive()
					s.ReadonlyWord = reading.word
					// One vector for both readings, so the only thing that
					// moves between the two runs is the axis. The letters are
					// the declaration's under either, which is what makes the
					// attribute row's refusal evidence about the *route*
					// rather than about a narrower set.
					s.ReadonlyOptions = "aAfgilprtux"
					s.DeclareOptions = "aAfgilprtux"
					s.ReadonlyRecordsTheCompoundAttribute = Yes
					r.Semantics = &s
					// A name the shell holds in a scalar slot, which is what
					// the kind gate is keyed on. Written here rather than
					// borrowed from a dialect, so the row names the shape and
					// not a shell.
					r.MarkParameterKindFixed("SPECIAL", ScalarParameter)
				})
				if !reading.want(out) {
					t.Errorf("with ReadonlyWord=%v, `%s` wrote %q — %s",
						reading.word, tc.src, out, reading.why)
				}
			}
		})
	}
}

// The word carries the `r` letter into the declaration, which is what the
// *listing* reads — not the flag beside it.
//
// Its own test because the two are separable and one of them was wrong: the
// flag alone freezes the name correctly and still leaves a bare `readonly`
// writing bare names where it had always written `name=value`, since the
// filtered listing asks whether a letter was written under a minus rather
// than which attribute is being filtered on.
func TestReadonlyWordAsADeclarationWritesTheLetterForTheListing(t *testing.T) {
	out, _ := run(t, `v=1; readonly v; w=2; readonly`, func(r *Runner) {
		s := permissive()
		s.ReadonlyWord = ReadonlyWordIsTheDeclaration
		s.ReadonlyOptions = "aAfgilprtux"
		s.DeclareOptions = "aAfgilprtux"
		s.DeclareValueQuoting = ListingQuoteWhenNeededPlain
		r.Semantics = &s
	})
	if !strings.Contains(out, "v=1") {
		t.Errorf("a bare `readonly` wrote %q, want the frozen name with its value", out)
	}
	if strings.Contains(out, "w=2") {
		t.Errorf("a bare `readonly` wrote %q, want only the frozen names", out)
	}
}
