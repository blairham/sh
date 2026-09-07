// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.InheritedValueSurvivesADeclaredType: the third answer to whether
// an attribute reaches back, on the one input where the two shells that
// re-read part company. Tests name the axis and never a shell.

// withInheritedType is declRunEnv's setter for a dialect that spells the
// three type letters and leaves a valueless declaration bringing no name into
// being, which is the arrangement the divergent answer occurs in.
func withInheritedType(survives, rereads Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.DeclareOptions = "aAgilprux"
		s.LocalOptions = "aAilprux"
		s.DeclareListing = DeclareListingExportSpelled
		s.BareDeclarationListing = DeclareListingPlainAssignment
		s.DeclaredNameWithoutValueIsEmpty = No
		s.InheritedValueSurvivesADeclaredType = survives
		s.AttributeRereadsTheValueItFinds = rereads
		// A compound value is a question of its own and not this one; the
		// answer that keeps the elements is what lets these cases assert
		// that a table is left alone rather than discarded.
		s.CompoundAttribute = CompoundAttributeKeepsTheElements
	}
}

// The whole of the axis, read three ways at once: what the shell itself sees,
// whether the name is set at all, and what a child is told. The last is the
// one that cannot be argued about, and the middle one is what tells the two
// wrong answers apart — an emptied name would still be SET.
func TestAnInheritedValueMeetingADeclaredType(t *testing.T) {
	for _, tc := range []struct {
		name              string
		survives, rereads Answer
		want              string
		toldTheChild      bool
	}{
		{"kept and left alone", Yes, No, "[SET][bar]\n", true},
		{"kept and re-read", Yes, Yes, "[SET][0]\n", true},
		{"discarded outright", No, Yes, "[][]\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRunEnv(t, `typeset -i INHERITED
echo "[${INHERITED+SET}][$INHERITED]"
env`, withInheritedType(tc.survives, tc.rereads), Diagnostics{},
				append(testPATH(), "INHERITED=bar"))
			if !strings.HasPrefix(out, tc.want) || st != 0 || errs != "" {
				t.Fatalf("got %q (stderr %q, status %d), want it to start %q",
					out, errs, st, tc.want)
			}
			told := strings.Contains(out, "INHERITED=")
			if told != tc.toldTheChild {
				t.Errorf("out = %q, want the child told: %v", out, tc.toldTheChild)
			}
		})
	}
}

// The value the fold would leave alone, which is where this question parts
// company with the re-read: none of these three is a value any fold would
// alter, and the discarding answer discards all three. Asked behind the
// canonical-spelling predicate rather than ahead of it, every one of them
// would have gone unanswered.
func TestADeclaredTypeDiscardsAnInheritedValueAFoldWouldNotTouch(t *testing.T) {
	for _, tc := range []struct{ name, src, env string }{
		{"already upper", "typeset -u A\necho \"[${A+SET}][$A]\"", "A=UPPER"},
		{"already canonical", "typeset -i B\necho \"[${B+SET}][$B]\"", "B=7"},
		{"already lower", "typeset -l C\necho \"[${C+SET}][$C]\"", "C=lower"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRunEnv(t, tc.src, withInheritedType(No, Yes),
				Diagnostics{}, []string{tc.env})
			if out != "[][]\n" || st != 0 || errs != "" {
				t.Errorf("got %q (stderr %q, status %d), want %q",
					out, errs, st, "[][]\n")
			}
		})
	}
}

// A declaration with nothing to say about a value leaves an inherited name
// entirely alone, whatever the axis says: the question is never reached.
// Every shell in the panel agrees here, so a discarding answer that fired for
// these letters would be wrong in all of them.
func TestADeclarationWithNoTypeLeavesAnInheritedValueAlone(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"bare", "typeset INHERITED"},
		{"exported", "typeset -x INHERITED"},
		{"frozen", "typeset -r INHERITED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRunEnv(t, tc.src+"\necho \"[${INHERITED+SET}][$INHERITED]\"\nenv",
				withInheritedType(No, Yes), Diagnostics{},
				append(testPATH(), "INHERITED=bar"))
			if !strings.HasPrefix(out, "[SET][bar]\n") || st != 0 || errs != "" {
				t.Fatalf("got %q (stderr %q, status %d), want it to start [SET][bar]", out, errs, st)
			}
			if !strings.Contains(out, "INHERITED=bar") {
				t.Errorf("out = %q, want the child still told the value", out)
			}
		})
	}
}

// The bound, and it is a fact about *one command's* letters: `-x` or `-r`
// beside the type letter keeps the value, and the same two attributes split
// across two commands do not. A reading that consulted whether the name is
// exported when the type arrives gets the second case wrong — the name is
// exported there by the line before, and is discarded all the same.
func TestTheExportAndFrozenLettersKeepAnInheritedValueOnTheirOwnCommand(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"exported on the same command", "typeset -ix G", "[SET][7]\n"},
		{"frozen on the same command", "typeset -ir G", "[SET][7]\n"},
		{"exported by the command before", "typeset -x G\ntypeset -i G", "[][]\n"},
		{"named again by the command before", "typeset G\ntypeset -i G", "[][]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRunEnv(t, tc.src+"\necho \"[${G+SET}][$G]\"",
				withInheritedType(No, Yes), Diagnostics{}, []string{"G=3+4"})
			if out != tc.want || st != 0 || errs != "" {
				t.Errorf("got %q (stderr %q, status %d), want %q", out, errs, st, tc.want)
			}
		})
	}
}

// The control that says the question is about where the value *lives* rather
// than about the export attribute: the script assigns the name its own value
// first — the very same value — and the plain re-read answers instead.
func TestAValueTheScriptAssignedIsNotAnInheritedOne(t *testing.T) {
	out, errs, st := declRunEnv(t, `D=$D
typeset -i D
echo "[${D+SET}][$D]"
env`, withInheritedType(No, Yes), Diagnostics{}, append(testPATH(), "D=bar"))
	if !strings.HasPrefix(out, "[SET][0]\n") || st != 0 || errs != "" {
		t.Fatalf("got %q (stderr %q, status %d), want it to start [SET][0]", out, errs, st)
	}
	if !strings.Contains(out, "D=0") {
		t.Errorf("out = %q, want the child told the re-read value", out)
	}
}

// What the discarding answer leaves behind is a *fresh* name and not an
// emptied one: the attribute is intact, the listing says the name with its
// attribute and no value, and a later assignment does not put the export
// back. That is what makes it one answer rather than a special case.
func TestAnInheritedNameStartedOverIsAFreshOne(t *testing.T) {
	out, errs, st := declRunEnv(t, `typeset -u E
typeset -p E
E=mix
echo "[$E]"
env`, withInheritedType(No, Yes), Diagnostics{}, append(testPATH(), "E=jj"))
	want := "typeset -u E\n[MIX]\n"
	if !strings.HasPrefix(out, want) || st != 0 || errs != "" {
		t.Fatalf("got %q (stderr %q, status %d), want it to start %q", out, errs, st, want)
	}
	if strings.Contains(out, "E=") {
		t.Errorf("out = %q, want the child told nothing about the name", out)
	}
}

// An unanswered axis refuses rather than guessing, because the three answers
// leave three different names behind — and one of them tells a child nothing
// where another tells it a value.
func TestAnUnansweredInheritedTypeAxisIsRefused(t *testing.T) {
	out, errs, st := declRunEnv(t, `typeset -i INHERITED
echo "st=$?"
echo "[${INHERITED+SET}][$INHERITED]"`, withInheritedType(Unspecified, Yes),
		Diagnostics{}, []string{"INHERITED=bar"})
	if !strings.Contains(errs, "a value the shell was started with surviving a declared type") {
		t.Errorf("stderr = %q, want the axis named", errs)
	}
	// Refused rather than answered one way and reported: the declaration is
	// not made, and the value the shell was started with is untouched.
	want := "st=2\n[SET][bar]\n"
	if out != want || st != 0 {
		t.Errorf("out = %q (status %d), want %q with the value left standing", out, st, want)
	}
}

// A name holding a compound value and nothing else is not holding an
// inherited one, and must not be discarded for a value it never had.
//
// A keyed table is the case that matters: declaredNameHolds counts it as
// something the name is holding, and unlike an indexed array it keeps no
// scalar view for the "the script assigned it" check to find — so it arrives
// at the question with a table and no value anywhere. A mutant that dropped
// the inherited-value guard marked the name removed and wrote its export off,
// and the *listing* is what shows it: a removed name is listable only by a
// surviving attribute, so the table went and the letter was all that came
// back. `${m[k]}` cannot see it — an element read does not consult the
// removal mark — which is why the row asserts both.
func TestACompoundValueIsNotAnInheritedOne(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a keyed table",
			"typeset -A m\nm[k]=v\ntypeset -i m\necho \"[${m[k]}]\"\ntypeset -p m",
			"[v]\ntypeset -Ai m=( [k]=\"v\" )\n",
		},
		{
			"an indexed array",
			"a=(x y)\ntypeset -i a\necho \"[${a[1]}]\"\ntypeset -p a",
			"[y]\ntypeset -ai a=( \"x\" \"y\" )\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRunEnv(t, tc.src, withInheritedType(No, Yes),
				Diagnostics{}, testPATH())
			if out != tc.want || st != 0 || errs != "" {
				t.Errorf("got %q (stderr %q, status %d), want %q", out, errs, st, tc.want)
			}
		})
	}
}

// An unanswered axis refuses the whole declaration and not only the operand
// it was asked about, which is what the rest of this family does: reporting
// and then declaring the names after it is the silent wrong answer with a
// diagnostic stapled to it.
func TestAnUnansweredInheritedTypeAxisRefusesTheNamesAfterItToo(t *testing.T) {
	out, errs, st := declRunEnv(t, `typeset -i INHERITED OTHER=5
echo "st=$?"
echo "other=[${OTHER-UNSET}]"`, withInheritedType(Unspecified, Yes),
		Diagnostics{}, []string{"INHERITED=bar"})
	want := "st=2\nother=[UNSET]\n"
	if out != want || st != 0 || errs == "" {
		t.Errorf("out = %q (stderr %q, status %d), want %q — the declaration is not made at all",
			out, errs, st, want)
	}
}
