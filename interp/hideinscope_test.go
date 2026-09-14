// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// The substrate behind `typeset -h` and `typeset +h`: a letter the dialect
// hands the declaration builtins through Semantics.DeclareOptions and
// LocalOptions, recorded against the name, and consulted wherever a scope has
// displaced that name. Tests name the letter and never a shell — see
// interp/hideinscope.go for what it does and where it was measured.

// The letter is observable through **one of the shell's own ties and nothing
// else**. A `local` of one half of a tie a *script* made is an ordinary,
// untied local whether the letter is there or not — see interp/tielocal.go —
// so every row here arranges the tie the way a dialect does, with Runner.Tie,
// and `typeset -T` appears below only where the script's own letter is the
// subject.

// withHidingAndTies is declRun's setter for a dialect that spells both
// letters: the tie is what the hide letter is observable through, so a
// dialect with one and not the other could not be asked the question.
// It answers two more axes than the letters, because the shape the letter
// lives in needs them: a valueless declaration sets the name rather than
// hiding it — which is what makes an emptied half of a tie observable at all
// — and `typeset` declares a local without a keyword-defined function, which
// is the only spelling `typeset -T` has. Both are also used by
// interp/tielocal_test.go.
func withHidingAndTies(s *Semantics) {
	s.DeclareOptions = "aAghilprTuUx"
	s.LocalOptions = "aAhilprTuUx"
	s.DeclareTypeLetter = DeclareTypeLetterTiesAScalarAndAnArray
	s.ArraysAreSparse = No
	s.ArrayBaseIsZero = No
	s.DeclaredNameWithoutValueIsEmpty = Yes
	s.TypesetLocalNeedsKeywordFunction = No
}

// The whole of what `-h` does: the local is an ordinary parameter spelled
// like the special one, so the array half keeps what the caller put there,
// and the caller's scalar comes back untouched on return.
func TestTheHideLetterDetachesALocalFromItsTie(t *testing.T) {
	out, errs, st := declRunTied(t, `S=one:two
f() { local -h S=zzz; echo "in=[$S][${s[@]}]"; }
f
echo "after=[$S][${s[@]}]"`, withHidingAndTies, Diagnostics{}, [2]string{"S", "s"})
	want := "in=[zzz][one two]\nafter=[one:two][one two]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("local -h over a tied name = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The control the row above is only meaningful against: the same declaration
// without the letter is *not* detached, and writing the scalar moves the
// array with it. A `-h` that parsed and did nothing passes the first test by
// answering this one's output.
func TestALocalWithoutTheHideLetterKeepsItsTie(t *testing.T) {
	out, errs, st := declRunTied(t, `S=one:two
f() { local S=zzz; echo "in=[$S][${s[@]}]"; }
f`, withHidingAndTies, Diagnostics{}, [2]string{"S", "s"})
	want := "in=[zzz][zzz]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("local over a tied name = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The letter is an attribute of the *name*: a declaration carrying no `h` of
// its own inherits whatever the name it shadows was given, however far away
// that was written.
func TestTheHideAttributeIsInheritedByALaterLocal(t *testing.T) {
	out, errs, st := declRunTied(t, `S=one:two
typeset -h S
f() { local S=zzz; echo "in=[${s[@]}]"; }
f`, withHidingAndTies, Diagnostics{}, [2]string{"S", "s"})
	want := "in=[one two]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a plain local under an inherited hide = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// And the plus form takes the inherited attribute off, which is the whole
// reason `+h` is a spelling a script writes rather than a letter it could
// have left out. This is the row that fails if `+h` is a no-op: the script is
// the one above with three characters added, and the answer is the other one.
func TestThePlusHideLetterTakesTheInheritedAttributeOff(t *testing.T) {
	out, errs, st := declRunTied(t, `S=one:two
typeset -h S
f() { local +h S=zzz; echo "in=[${s[@]}]"; }
f`, withHidingAndTies, Diagnostics{}, [2]string{"S", "s"})
	want := "in=[zzz]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("local +h under an inherited hide = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The attribute detaches a *shadow* and nothing else. With no local standing
// in the way there is no second cell to be ordinary, and the name goes on
// being the special one — so a declaration at the top level changes no answer
// until a function shadows the name.
func TestTheHideAttributeDetachesNothingWithoutALocal(t *testing.T) {
	out, errs, st := declRunTied(t, `typeset -h S
S=one:two
echo "top=[${s[@]}]"`, withHidingAndTies, Diagnostics{}, [2]string{"S", "s"})
	want := "top=[one two]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("the hide letter at the top level = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// `unset` does **not** take the letter off, which is the row this file said
// the other way round until the tie survived an unset and made it measurable
// (#1631). The name goes away, comes back on the next assignment still tied,
// and is still detached from that tie inside a local — only the plus form
// puts it back.
func TestUnsetDoesNotTakeTheHideAttributeOff(t *testing.T) {
	out, errs, st := declRunTied(t, `typeset -h S
unset S
S=one:two
f() { local S=zzz; echo "in=[${s[*]}]"; }
f
typeset +h S
g() { local S=www; echo "then=[${s[*]}]"; }
g`, withHidingAndTies, Diagnostics{}, [2]string{"S", "s"})
	want := "in=[one two]\nthen=[www]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("the hide letter across an unset = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// An attribute a call added goes away with the call, the way a freeze does:
// the caller's name is the special one again the moment the function returns.
func TestTheHideAttributeAddedByACallGoesAwayWithIt(t *testing.T) {
	out, errs, st := declRunTied(t, `f() { local -h S=zzz; }
f
S=three:four
echo "after=[${s[@]}]"`, withHidingAndTies, Diagnostics{}, [2]string{"S", "s"})
	want := "after=[three four]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("the hide letter after the call returns = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// And one a call *removed* with `+h` comes back for the same reason, which is
// the other direction of the same save. A restore that only put back a `true`
// would pass the test above and fail this one.
func TestTheHideAttributeRemovedByACallComesBack(t *testing.T) {
	out, errs, st := declRunTied(t, `typeset -h S
f() { local +h S=zzz; }
f
S=one:two
g() { local S=qqq; echo "in=[${s[@]}]"; }
g`, withHidingAndTies, Diagnostics{}, [2]string{"S", "s"})
	want := "in=[one two]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a later local after `+h` in a returned call = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The last `h` written decides, whichever sign it carried, which is the rule
// the other two-sign letters here keep.
func TestTheLastHideLetterWrittenDecides(t *testing.T) {
	out, errs, st := declRunTied(t, `S=one:two
T=one:two
f() { local -h +h S=zzz; echo "plusLast=[${s[@]}]"; }
f
g() { local +h -h T=qqq; echo "minusLast=[${t[@]}]"; }
g`, withHidingAndTies, Diagnostics{}, [2]string{"S", "s"}, [2]string{"T", "t"})
	want := "plusLast=[zzz]\nminusLast=[one two]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("both signs of the hide letter = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// A letter outside the dialect's set is still refused by name, under both
// signs: the plus form is what a real script writes and it must not be the
// one spelling that slips past a dialect which has not been given the letter.
func TestTheHideLetterIsTheDialectsToGive(t *testing.T) {
	for _, sign := range []string{"-", "+"} {
		_, errs, st := declRun(t, "typeset "+sign+"h v=1", nil,
			Diagnostics{UnimplementedOptionLetters: map[string]string{"typeset": "h"}})
		want := "testsh: typeset: " + sign + "h is not implemented yet\n"
		if errs != want || st != 2 {
			t.Errorf("typeset %sh without the letter = %q (status %d), want %q with 2",
				sign, errs, st, want)
		}
	}
}

// The other thing the letter detaches: a parameter the shell **produces**.
//
// A tie is what the letter is observable through for a name a *script* can
// make, and a produced parameter is the second seam it reaches — and the one
// it did not reach for as long as the attribute merely tied and untied
// (#2586). Every row below registers a producer the way a dialect does, with
// SetDynamic, and the assertion is whether the local holds what the
// declaration wrote or what the producer says.

// withHidingInScope is withHidingAndTies without the tie letters, for the
// rows whose subject is a producer rather than a pair of names. Named for the
// letter it spells, because hidevalue_test.go's withHiding spells the *other*
// hiding letter and the two are not the same attribute (#2042).
func withHidingInScope(s *Semantics) {
	s.DeclareOptions = "aAghilprux"
	s.LocalOptions = "aAhilprux"
	s.DeclaredNameWithoutValueIsEmpty = Yes
	s.TypesetLocalNeedsKeywordFunction = No
}

// producedAndHidden registers a produced scalar and gives it the attribute,
// which is what a dialect's module registration does — the shell's own
// parameters carry it from the moment the module is loaded rather than from
// anything a script writes.
func producedAndHidden(name, value string, hide, frozen bool) func(*Runner) {
	return func(r *Runner) {
		r.SetDynamic(name, func(*Runner) string { return value })
		if frozen {
			r.MarkReadonly(name)
		}
		if hide {
			r.MarkHideInScope(name)
		}
	}
}

// The whole of what the attribute does to a producer: the shadow is an
// ordinary parameter, so it holds what the declaration wrote and the name is
// the shell's own again on return.
func TestAHiddenShadowOfAProducedNameIsAnOrdinaryParameter(t *testing.T) {
	out, errs, st := declRunWith(t, `f() { local P=5; echo "in=[$P]"; }
f
echo "after=[$P]"`, withHidingInScope, Diagnostics{}, nil, producedAndHidden("P", "produced", true, false))
	want := "in=[5]\nafter=[produced]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a hidden shadow of a produced name = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The control the row above is only meaningful against, and the one that says
// this is the *attribute* rather than "a local of a produced name is always
// ordinary": the same producer without it is read straight through the
// shadow, and the value the declaration wrote is never seen.
func TestAShadowOfAProducedNameWithoutTheAttributeIsTheProducedView(t *testing.T) {
	out, errs, st := declRunWith(t, `f() { local P=5; echo "in=[$P]"; }
f`, withHidingInScope, Diagnostics{}, nil, producedAndHidden("P", "produced", false, false))
	want := "in=[produced]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a plain shadow of a produced name = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// A valueless declaration of a hidden producer is the empty cell the dialect
// says a valueless declaration is, and not the produced value — which is the
// row a fix aimed at the *assignment* would fail.
func TestAValuelessHiddenShadowOfAProducedNameIsEmpty(t *testing.T) {
	out, errs, st := declRunWith(t, `f() { local P; echo "in=[$P]"; }
f`, withHidingInScope, Diagnostics{}, nil, producedAndHidden("P", "produced", true, false))
	want := "in=[]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a valueless hidden shadow = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The letter written on the declaration reaches a producer the same way the
// registration does, for a name that carries nothing of its own.
func TestTheHideLetterOnADeclarationDetachesAProducer(t *testing.T) {
	out, errs, st := declRunWith(t, `f() { local -h P=5; echo "in=[$P]"; }
f
echo "after=[$P]"`, withHidingInScope, Diagnostics{}, nil, producedAndHidden("P", "produced", false, false))
	want := "in=[5]\nafter=[produced]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("`local -h` over a produced name = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// And `+h` puts the producer back, which is more than the absence of `-h`:
// the name carries the attribute already, so this declaration is the only
// thing asking for the second view — and a `+h` that merely parsed would
// leave the local holding the 5 it wrote.
func TestThePlusHideLetterPutsTheProducerBack(t *testing.T) {
	out, errs, st := declRunWith(t, `f() { local +h P=5; echo "in=[$P]"; }
f`, withHidingInScope, Diagnostics{}, nil, producedAndHidden("P", "produced", true, false))
	want := "in=[produced]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("`local +h` over a hidden producer = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The attribute suspends a producer only where a local stands, which is the
// same control the tie half has: a name carrying it at the top level is
// still produced there.
func TestTheHideAttributeSuspendsNoProducerWithoutALocal(t *testing.T) {
	out, errs, st := declRunWith(t, `echo "top=[$P]"`, withHidingInScope, Diagnostics{}, nil,
		producedAndHidden("P", "produced", true, false))
	want := "top=[produced]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a hidden producer at the top level = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The freeze and the producer come off together, which is what makes the
// letter usable at all over one of the shell's own frozen names: without the
// second half the declaration is accepted and then discarded, and the local
// reads the produced value at status 0.
func TestAHiddenShadowOfAFrozenProducerHoldsWhatItWasGiven(t *testing.T) {
	frozen := func(s *Semantics) {
		withHidingInScope(s)
		s.DeclarationMayShadowAReadonly = Yes
	}
	out, errs, st := declRunWith(t, `f() { local -h P=5; echo "in=[$P]"; }
f
echo "after=[$P]"`, frozen, Diagnostics{}, nil, producedAndHidden("P", "produced", false, true))
	want := "in=[5]\nafter=[produced]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("`local -h` over a frozen producer = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// A nested call inside a hidden shadow sees the ordinary parameter, because
// the producer is out of the tables for as long as the shadow stands rather
// than for the one expansion the declaration is on.
func TestANestedCallInsideAHiddenShadowSeesTheOrdinaryParameter(t *testing.T) {
	out, errs, st := declRunWith(t, `g() { echo "g=[$P]"; }
f() { local P=5; g; }
f
g`, withHidingInScope, Diagnostics{}, nil, producedAndHidden("P", "produced", true, false))
	want := "g=[5]\ng=[produced]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a nested call inside a hidden shadow = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}
