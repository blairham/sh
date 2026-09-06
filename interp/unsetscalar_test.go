// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// `unset "a[i]"` where the name holds a string rather than an array. Three
// answers across the panel, and all three fall out of what a subscripted name
// *means* there: one axis reads the subscript as a character, and where it
// reads an element instead a second decides what a subscript naming none does.

// scalarUnsetRun answers the base and the two readings, so each case reaches
// the question it is about rather than a refusal.
func scalarUnsetRun(t *testing.T, src string, set func(*Semantics)) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.UnsetTakesASubscript = Yes
		set(&sem)
		r.Semantics = &sem
	})
}

// characters and elements are the two readings, spelled once.
func characters(s *Semantics) {
	s.ScalarSubscriptIsACharacter = Yes
	s.ArrayBaseIsZero = No
}

func elementsFromZero(s *Semantics) {
	s.ScalarSubscriptIsACharacter = No
	s.ArrayBaseIsZero = Yes
}

func TestASubscriptOnAStringNamesACharacterAndUnsetTakesItOut(t *testing.T) {
	for _, tc := range []struct{ sub, want string }{
		{"1", "st=0 [ello]\n"},
		{"2", "st=0 [hllo]\n"},
		{"5", "st=0 [hell]\n"},
		{"-1", "st=0 [hell]\n"},
		{"-5", "st=0 [ello]\n"},
	} {
		out, st := scalarUnsetRun(t,
			`a=hello; unset "a[`+tc.sub+`]"; echo "st=$? [${a-UNSET}]"`, characters)
		if st != 0 || out != tc.want {
			t.Errorf("a[%s]: got %q status %d, want %q", tc.sub, out, st, tc.want)
		}
	}
}

func TestACharacterSubscriptOutOfReachChangesNothing(t *testing.T) {
	// Past the end names no character; so does a negative reaching back
	// before the first, and that one is *quiet* where the same reach with a
	// non-negative subscript is refused.
	for _, sub := range []string{"9", "-6", "-9"} {
		out, st := scalarUnsetRun(t,
			`a=hello; unset "a[`+sub+`]"; echo "st=$? [${a-UNSET}]"`, characters)
		if st != 0 || out != "st=0 [hello]\n" {
			t.Errorf("a[%s]: got %q status %d, want the string untouched at 0", sub, out, st)
		}
	}
}

func TestACharacterSubscriptBelowTheFirstIsRefused(t *testing.T) {
	out, st := scalarUnsetRun(t, `a=hello; unset "a[0]"; echo "st=$? [${a-UNSET}]"`, characters)
	if st != 0 || out != "sh: unset: [0]: bad array subscript\nst=1 [hello]\n" {
		t.Errorf("got %q status %d, want the refusal and the string kept", out, st)
	}
}

func TestASubscriptNamingTheOneElementAScalarIsTakesTheNameAway(t *testing.T) {
	// Not the value: the name, and its export attribute with it.
	out, st := scalarUnsetRun(t,
		`export a=hello; unset "a[0]"; echo "st=$? [${a-UNSET}]"; `+
			`/usr/bin/env | grep '^a=' || echo "(none)"`, elementsFromZero)
	if st != 0 || out != "st=0 [UNSET]\n(none)\n" {
		t.Errorf("got %q status %d, want the whole name gone", out, st)
	}
	// And the base decides which numeral names it.
	out, st = scalarUnsetRun(t, `a=hello; unset "a[1]"; echo "st=$? [${a-UNSET}]"`,
		func(s *Semantics) {
			s.ScalarSubscriptIsACharacter = No
			s.ArrayBaseIsZero = No
			s.UnsetSubscriptOnAScalarIsAnError = No
		})
	if st != 0 || out != "st=0 [UNSET]\n" {
		t.Errorf("base 1: got %q status %d, want the whole name gone through 1", out, st)
	}
}

func TestASubscriptNamingNoElementOfAScalarIsAnAxis(t *testing.T) {
	const src = `a=hello; unset "a[2]"; echo "st=$? [${a-UNSET}]"`
	out, st := scalarUnsetRun(t, src, func(s *Semantics) {
		elementsFromZero(s)
		s.UnsetSubscriptOnAScalarIsAnError = Yes
	})
	if st != 0 || out != "sh: unset: a: not an array variable\nst=1 [hello]\n" {
		t.Errorf("refusing: got %q status %d, want the complaint at 1", out, st)
	}
	out, st = scalarUnsetRun(t, src, func(s *Semantics) {
		elementsFromZero(s)
		s.UnsetSubscriptOnAScalarIsAnError = No
	})
	if st != 0 || out != "st=0 [hello]\n" {
		t.Errorf("quiet: got %q status %d, want nothing said and nothing done", out, st)
	}
}

func TestTheRefusalIsAboutTheNameAndNotTheSubscript(t *testing.T) {
	// An array with a gap takes the same subscript without a word under
	// either answer, so what the refusing shell objects to is the name not
	// being an array — which is what its wording says.
	for _, answer := range []Answer{Yes, No} {
		out, st := scalarUnsetRun(t,
			`a=(x y z); unset "a[9]"; echo "st=$? n=${#a[@]}"`,
			func(s *Semantics) {
				elementsFromZero(s)
				s.UnsetSubscriptOnAScalarIsAnError = answer
			})
		if st != 0 || out != "st=0 n=3\n" {
			t.Errorf("answer %v: got %q status %d, want the array left alone at 0", answer, out, st)
		}
	}
}

func TestASubscriptOnANameHoldingNothingIsQuietUnderEveryReading(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*Semantics)
	}{
		{"characters", characters},
		{"elements", func(s *Semantics) {
			elementsFromZero(s)
			s.UnsetSubscriptOnAScalarIsAnError = Yes
		}},
	} {
		out, st := scalarUnsetRun(t,
			`unset b; unset "b[0]"; echo "st=$?"; unset "b[1]"; echo "st=$?"`, tc.set)
		if st != 0 || out != "st=0\nst=0\n" {
			t.Errorf("%s: got %q status %d, want both quiet at 0", tc.name, out, st)
		}
	}
}

func TestAnEmptyStringStillHasTheOneElementAScalarIs(t *testing.T) {
	// No character to name, so the character reading leaves the name empty;
	// the element reading still finds the element and takes the name away.
	out, st := scalarUnsetRun(t, `a=; unset "a[1]"; echo "st=$? [${a-UNSET}]"`, characters)
	if st != 0 || out != "st=0 []\n" {
		t.Errorf("characters: got %q status %d, want the empty name kept", out, st)
	}
	out, st = scalarUnsetRun(t, `a=; unset "a[0]"; echo "st=$? [${a-UNSET}]"`, elementsFromZero)
	if st != 0 || out != "st=0 [UNSET]\n" {
		t.Errorf("elements: got %q status %d, want the name taken away", out, st)
	}
}

func TestACharacterSubscriptCountsCharactersAndNotBytes(t *testing.T) {
	// The locale's, which is what interp/multibyte.go owns for every other
	// reading of a string by position.
	out, st := runGrammar(t, `a=héllo; unset "a[2]"; echo "[$a]"`, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.UnsetTakesASubscript = Yes
		sem.MultibyteEncodingIsHonored = Yes
		characters(&sem)
		r.Semantics = &sem
		r.Env = append(r.Env, "LC_ALL=en_US.UTF-8")
	})
	if st != 0 || out != "[hllo]\n" {
		t.Errorf("got %q status %d, want the character removed rather than a byte", out, st)
	}
}

func TestAnEmptyArrayIsStillAnArray(t *testing.T) {
	// The scalar path is for a name the array table does not hold at all, not
	// for one it holds empty: an array with no elements has no element for a
	// subscript to name, and the *name* is not a string for a subscript to
	// name a character of either. Read through the listing, because the
	// difference between an empty array and no name at all is invisible to
	// an expansion.
	for _, sub := range []string{"0", "1", "9"} {
		out, st := scalarUnsetRun(t,
			`a=(); unset "a[`+sub+`]"; echo "st=$?"; typeset -p a`,
			func(s *Semantics) {
				elementsFromZero(s)
				s.UnsetSubscriptOnAScalarIsAnError = Yes
			})
		if st != 0 || out != "st=0\ndeclare -a a=()\n" {
			t.Errorf("a[%s]: got %q status %d, want the empty array still declared", sub, out, st)
		}
	}
}
