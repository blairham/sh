// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a declaration's attributes do to the name a scope stands in front of.
// The rows are measured in interp/localattributes.go and are unanimous across
// the panel, so the tests name letters and axes and never a shell.
//
// Each one asserts *both* ends. A fix that only cleared the attribute going
// in would pass the first half and leave the caller's name stripped; one that
// only put it back would pass the second half and leave the local typed.

// withAttributeLetters is the declaration surface these rows need: every
// attribute letter the runner tracks, and `typeset` declaring a local in an
// ordinary function — the axis one shell in the panel answers the other way,
// which is about *where* a scope is and not about what the letters do.
func withAttributeLetters(s *Semantics) {
	s.DeclareOptions = "aAfFgilprtuUxHE"
	s.LocalOptions = "aAfFilprtuUxHE"
	s.TypesetLocalNeedsKeywordFunction = No
}

// The reported shape, in its smallest form: a numeric letter on a local, and
// the caller's plain name still plain afterwards.
func TestANumericLocalDoesNotTypeTheCallersName(t *testing.T) {
	src := `v=plain
f() { typeset -i v; v=1+1; echo "in=[$v]"; }
f
v=3+4
echo "after=[$v]"`
	out, errs, st := declRun(t, src, withAttributeLetters, Diagnostics{})
	const want = "in=[2]\nafter=[3+4]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The same rule under the other word, which is a separate route through the
// runner and not a spelling of the first: `local` has its own declaration
// loop, and a fix applied to one of them leaves the other letting the
// attribute out. Both ends again, and the letter typing the local is what
// says the declaration still works.
func TestALocalsAttributesAreTheLocalsToo(t *testing.T) {
	src := `w=plain
f() { local -i w; w=1+1; echo "in=[$w]"; }
f
w=3+4
echo "after=[$w]"`
	out, errs, st := declRun(t, src, withAttributeLetters, Diagnostics{})
	const want = "in=[2]\nafter=[3+4]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// And the other direction: the cell a scope was just taken for carries none
// of the attributes the outer name carried, so a value written into it is
// stored as it was written.
func TestAFreshLocalInheritsNoneOfTheOuterAttributes(t *testing.T) {
	src := `typeset -i n=5
f() { typeset n; n=3+4; echo "in=[$n]"; }
f
echo "after=[$n]"`
	out, errs, st := declRun(t, src, withAttributeLetters, Diagnostics{})
	const want = "in=[3+4]\nafter=[5]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// And the third route: an operand that names an *element*. The attributes are
// the base array's — that is what the declaration is about — so they belong to
// the local the same way, and the letter reaching the caller's array is read
// two assignments later. Its own declaration loop again, so its own row.
func TestAnElementDeclarationsAttributesAreTheLocalsToo(t *testing.T) {
	src := `a=(x y)
f() { typeset -i a[0]=3+4; echo "in=[${a[0]}]"; }
f
echo "after=[${a[*]}]"
a=(1+1 2)
echo "later=[${a[0]}]"`
	// The two axes a subscripted operand runs into, on the answers that let
	// it reach this question at all — the dialect that refuses the operand
	// never gets here, and the one with no scope to take has nothing to put
	// back. See Semantics.SubscriptedOperandTakesALocalDeclaration.
	out, errs, st := declRun(t, src, func(sem *Semantics) {
		withAttributeLetters(sem)
		sem.TypesetTakesASubscript = Yes
		sem.SubscriptedOperandTakesALocalDeclaration = Yes
		sem.SubscriptedOperandTakesTheIntegerAttribute = Yes
		sem.CompoundElementsGoThroughTheAttribute = Yes
	}, Diagnostics{})
	const want = "in=[7]\nafter=[x y]\nlater=[1+1]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The case letters, which are the same rule read through a fold rather than
// through arithmetic — and the pair that says the *value* was safe too: a
// leaked `-l` folded what the caller was holding on the way back out.
func TestTheCaseLettersDoNotOutliveTheDeclaration(t *testing.T) {
	src := `up=ABC
f() { typeset -l up; up=MIXED; echo "in=[$up]"; }
f
echo "after=[$up]"
up=StillMixed
echo "later=[$up]"`
	out, errs, st := declRun(t, src, withAttributeLetters, Diagnostics{})
	const want = "in=[mixed]\nafter=[ABC]\nlater=[StillMixed]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The integer *base*, which travels with the numeric letter and is a second
// table: a local declared `-i` with no base written is base ten, whatever base
// the name it shadows renders in, and the caller renders in its own again
// afterwards.
func TestTheIntegerBaseDoesNotOutliveTheDeclaration(t *testing.T) {
	src := `typeset -i16 h=255
f() { typeset -i h; h=255; echo "in=[$h]"; }
f
echo "after=[$h]"`
	// The base is one shell's reading of the letter and not everyone's — see
	// Semantics.IntegerAttributeTakesABase — so the row that is about the
	// *scope* has to say which reading it is asking under.
	out, errs, st := declRun(t, src, func(sem *Semantics) {
		withAttributeLetters(sem)
		sem.IntegerAttributeTakesABase = Yes
	}, Diagnostics{})
	// The inner half is the discriminating one: base ten is what a local `-i`
	// with no base written renders in, and the leaked base makes it `16#FF`.
	const want = "in=[255]\nafter=[255]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The unique letter, whose leak is the quietest of the family: nothing is
// reported and an array simply loses an element some lines later.
func TestTheUniqueLetterDoesNotOutliveTheDeclaration(t *testing.T) {
	src := `f() { typeset -U u; }
f
u=(a b a)
echo "after n=${#u[@]} [${u[*]}]"`
	out, errs, st := declRun(t, src, withAttributeLetters, Diagnostics{})
	const want = "after n=3 [a b a]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The composition the bug was found through, which is the one that matters
// and the one no single-letter row reaches: one function's numeric letter,
// a *later* function's array of the same name, and a path stored in it.
//
// With the attribute leaking, the second declaration made an integer array
// and the store evaluated a path as an expression — `bad math expression:
// operand expected at …` from a line that never wrote an arithmetic context
// (#1673). The assertion is the path coming back whole, because that is what
// a script reads; the diagnostic is the second half of it, since a leak that
// merely truncated would print nothing.
func TestANumericLetterFromAnEarlierCallDoesNotTypeALaterLocalArray(t *testing.T) {
	src := `first() { typeset -i list; }
first
second() { typeset -a list; list=( /some/dir/file.zsh ); echo "in=[${list[*]}]"; }
second`
	out, errs, st := declRun(t, src, withAttributeLetters, Diagnostics{})
	const want = "in=[/some/dir/file.zsh]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A name the *call* typed and that nothing outside it ever named: the
// attribute goes away with the call rather than being left behind on a name
// the caller then meets for the first time. This is the half savedReadonly's
// `false` entry exists for, asked of the rest of the letters.
func TestAnAttributeOnANameTheCallerNeverHadGoesAwayWithTheCall(t *testing.T) {
	src := `f() { typeset -i fresh=1; }
f
fresh=3+4
echo "after=[$fresh]"`
	out, errs, st := declRun(t, src, withAttributeLetters, Diagnostics{})
	const want = "after=[3+4]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The control that keeps the clearing where it belongs: `-g` takes no scope,
// so its attributes are the global's and outlive the call by design.
func TestAGlobalDeclarationsAttributeIsNotClearedByTheCall(t *testing.T) {
	src := `f() { typeset -gi g; }
f
g=3+4
echo "after=[$g]"`
	out, errs, st := declRun(t, src, withAttributeLetters, Diagnostics{})
	const want = "after=[7]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// And the control that keeps it out of the top level entirely: a declaration
// outside a function has no scope to put anything back, so its own letters
// stand and a second declaration of the same name does not wipe them.
func TestATopLevelDeclarationKeepsItsAttributes(t *testing.T) {
	src := `typeset -i t=1
typeset t
t=3+4
echo "after=[$t]"`
	out, errs, st := declRun(t, src, withAttributeLetters, Diagnostics{})
	const want = "after=[7]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}
