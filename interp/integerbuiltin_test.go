// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The `integer` builtin: the declaration under a second name with the integer
// attribute already decided. Tests name axes and letters, never shells.

// integerRun runs src with `integer` registered, the semantics the setter
// leaves, and the given diagnostics.
//
// Its own runner rather than declRun's because the builtin is registered
// through the extension seam — a dialect that does not have the word does not
// get it — so a helper that could not register would be testing `typeset`.
func integerRun(t *testing.T, src string, set func(*Semantics), dg Diagnostics) (string, string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.DeclaredNameWithoutValueIsEmpty = No
	sem.ValuelessDeclarationHidesTheOuterValue = Yes
	sem.DeclareOptions = "aAgHilpruUx"
	sem.IntegerOptions = "gHilprux"
	sem.IntegerAttributeTakesABase = Yes
	sem.IntegerPlusFormTakesAttributesOff = Yes
	sem.TypesetBadOptionFatal = No
	// `export -p` is how these tests observe the export attribute: the
	// runner has no PATH, so `env` is not reachable from inside one.
	sem.ExportListing = DeclareListingCommandWord
	sem.DeclareValueQuoting = ListingQuoteAlwaysEscaped
	if set != nil {
		set(&sem)
	}
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &dg,
		Dir: t.TempDir(), Name: "testsh",
	})
	r.Register("integer", IntegerBuiltin())
	r.SetDeclaring("integer")
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

// The whole of what the second name is for: a declaration that carries the
// integer attribute without the letter being written, so a later assignment
// is an expression. A registration that merely aliased `typeset` would answer
// the second line `5+2`.
func TestIntegerDeclaresTheAttributeWithoutTheLetter(t *testing.T) {
	out, errs, st := integerRun(t, `integer n=3
echo "decl=[$n]"
n=5+2
echo "later=[$n]"
integer m=1+2
echo "onTheLine=[$m]"`, nil, Diagnostics{})
	want := "decl=[3]\nlater=[7]\nonTheLine=[3]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("integer n=3 = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// Text that is not a number is not an error under the attribute, and the
// second name has to reach the same reading: `abc` is an expression made of
// an unset name, so the value is zero and nothing is said.
func TestIntegerTakesTextAsAnExpression(t *testing.T) {
	out, errs, st := integerRun(t, `integer q=abc
echo "q=[$q]"`, nil, Diagnostics{})
	want := "q=[0]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("integer q=abc = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The other letters come with the name and mean what they mean on the
// declaration: `-r` freezes after assigning its own value, and `-x` reaches
// the environment.
func TestIntegerCarriesTheDeclarationsOtherLetters(t *testing.T) {
	out, errs, st := integerRun(t, `integer -r r=5
echo "r=[$r]"
integer -x e=2
export -p`, nil, Diagnostics{})
	if st != 0 || errs != "" || !strings.HasPrefix(out, "r=[5]\n") ||
		!strings.Contains(out, "export e='2'") {
		t.Errorf("integer -r/-x = %q (stderr %q, status %d), want r=[5] and e exported",
			out, errs, st)
	}
}

// Where a plus word takes attributes off, `+i` reaches the attribute the name
// itself asked for and a later assignment is text again.
func TestAPlusFormThatRemovesReachesTheNamesOwnAttribute(t *testing.T) {
	out, errs, st := integerRun(t, `integer n=5
integer +i n
n=3+4
echo "n=[$n]"`, nil, Diagnostics{})
	want := "n=[3+4]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("integer +i = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// And where it does not, the same three lines leave the attribute standing —
// the axis, read from both sides. A shell that took the removal anyway
// answers `3+4` here.
func TestAPlusFormThatRemovesNothingLeavesTheAttribute(t *testing.T) {
	out, errs, st := integerRun(t, `integer n=5
integer +i n
n=3+4
echo "n=[$n]"`, func(s *Semantics) {
		s.IntegerPlusFormTakesAttributesOff = No
	}, Diagnostics{})
	want := "n=[7]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("integer +i where nothing is removed = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The axis is about the plus *word* and not about the `i` in it: where
// nothing is removed, `integer +x` leaves the export standing too. This is
// the row that makes it one field rather than one per letter.
func TestAPlusFormThatRemovesNothingLeavesTheExport(t *testing.T) {
	out, errs, st := integerRun(t, `integer -x e=2
integer +x e
export -p`, func(s *Semantics) {
		s.IntegerPlusFormTakesAttributesOff = No
	}, Diagnostics{})
	if st != 0 || errs != "" || !strings.Contains(out, "export e='2'") {
		t.Errorf("integer +x where nothing is removed = %q (stderr %q, status %d), "+
			"want e still exported", out, errs, st)
	}
}

// A plus word that removes takes the export off and leaves the *integer*
// attribute alone, because `i` was not the letter written. The name asked for
// that one and only its own spelling may cancel it.
func TestAPlusFormOnAnotherLetterKeepsTheIntegerAttribute(t *testing.T) {
	out, errs, st := integerRun(t, `integer -x e=2
integer +x e
e=5+2
echo "e=[$e]"
export -p`, nil, Diagnostics{})
	if st != 0 || errs != "" || !strings.HasPrefix(out, "e=[7]\n") ||
		strings.Contains(out, "export e=") {
		t.Errorf("integer +x = %q (stderr %q, status %d), want e=[7] and e unexported",
			out, errs, st)
	}
}

// The unanswered axis refuses rather than picking a shell.
func TestAPlusFormOnIntegerRefusesWithoutTheAxis(t *testing.T) {
	_, errs, st := integerRun(t, `integer n=5
integer +i n`, func(s *Semantics) {
		s.IntegerPlusFormTakesAttributesOff = Unspecified
	}, Diagnostics{})
	want := "testsh: a plus word on `integer` taking an attribute off the name: " +
		"the shells disagree here and no dialect was chosen\n"
	if errs != want || st != 2 {
		t.Errorf("integer +i with no axis = %q (status %d), want %q", errs, st, want)
	}
}

// A letter out of the set is refused as unknown, in the dialect's words —
// which is the answer for `-a` and `-A`, letters the declaration takes and
// this second name does not.
func TestALetterOutsideTheIntegerSetIsUnknown(t *testing.T) {
	_, errs, st := integerRun(t, `integer -A m`, nil, Diagnostics{
		BuiltinBadOption:       "%[1]s: bad option: %[2]s",
		BuiltinBadOptionStatus: 1,
	})
	want := "testsh: integer: bad option: -A\n"
	if errs != want || st != 1 {
		t.Errorf("integer -A = %q (status %d), want %q", errs, st, want)
	}
}

// A letter the dialect's `integer` really has and this shell does not is
// named as missing rather than as unknown, and under the builtin's own name.
func TestALetterTheDialectsIntegerHasIsCalledMissing(t *testing.T) {
	_, errs, st := integerRun(t, `integer -Z m`, nil, Diagnostics{
		UnimplementedOptionLetters: map[string]string{"integer": "LRZht"},
	})
	want := "testsh: integer: -Z is not implemented yet\n"
	if errs != want || st != 2 {
		t.Errorf("integer -Z = %q (status %d), want %q", errs, st, want)
	}
}

// The refusal can name another builtin, which is what one of the two shells
// with the word does: its `integer` is `typeset` and says so.
func TestTheIntegerRefusalCanNameTheDeclaration(t *testing.T) {
	_, errs, st := integerRun(t, `integer -Z m`, nil, Diagnostics{
		UnimplementedOptionLetters: map[string]string{"integer": "Z"},
		BuiltinComplaintName:       map[string]string{"integer": "typeset"},
	})
	want := "testsh: typeset: -Z is not implemented yet\n"
	if errs != want || st != 2 {
		t.Errorf("integer -Z under a second complaint name = %q (status %d), want %q",
			errs, st, want)
	}
}

// A bad letter ends the script where the dialect counts the declaration among
// its special builtins, the same way a bad `typeset` option does.
func TestABadIntegerLetterEndsTheScriptWhereTheDialectSaysSo(t *testing.T) {
	out, _, st := integerRun(t, `integer -q m
echo after`, func(s *Semantics) {
		s.TypesetBadOptionFatal = Yes
	}, Diagnostics{})
	if out != "" || st != 2 {
		t.Errorf("integer -q under a fatal dialect = %q (status %d), want no output at 2", out, st)
	}
}

// No names at all is a filtered listing in both shells that have the word and
// is not built, so it refuses by name. The danger it is guarding against is
// the bare-declaration listing: falling through to that would answer with the
// whole variable table, which is a *wrong* answer rather than a missing one.
func TestIntegerWithNoNamesRefusesByName(t *testing.T) {
	out, errs, st := integerRun(t, `v=1
integer
echo after`, nil, Diagnostics{})
	if out != "after\n" || st != 0 {
		t.Errorf("bare integer = %q (status %d), want only the line after it", out, st)
	}
	want := "testsh: integer: a listing is not implemented yet\n"
	if errs != want {
		t.Errorf("bare integer said %q, want %q", errs, want)
	}
}

// The same for letters with no names, which is the other spelling of a
// filtered listing.
func TestIntegerWithLettersAndNoNamesRefusesByName(t *testing.T) {
	_, errs, st := integerRun(t, `integer -r`, nil, Diagnostics{})
	want := "testsh: integer: a listing is not implemented yet\n"
	if errs != want || st != 2 {
		t.Errorf("integer -r with no names = %q (status %d), want %q", errs, st, want)
	}
}

// An output base is refused by name where the dialect's `-i` takes one. It is
// the silent shape that makes this worth a refusal: read and dropped, `-i 16`
// leaves 255 standing where the shells that have the base print `16#ff`, and
// the `16` becomes a name of its own.
func TestAnOutputBaseRefusesByName(t *testing.T) {
	out, errs, st := integerRun(t, `echo "b=[$b]"
integer -i 16 b=255`, nil, Diagnostics{})
	want := "testsh: integer: an output base is not implemented yet\n"
	if errs != want || st != 2 {
		t.Errorf("integer -i 16 = %q (status %d), want %q", errs, st, want)
	}
	if out != "b=[]\n" {
		t.Errorf("integer -i 16 stored %q, want the name left alone", out)
	}
}

// And in the attached spelling, which has to be caught before the letter loop
// reaches the `1` and calls it an unknown option — a true statement about a
// letter the script never wrote.
func TestAnAttachedOutputBaseRefusesByName(t *testing.T) {
	_, errs, st := integerRun(t, `typeset -i16 b=255`, nil, Diagnostics{})
	want := "testsh: typeset: an output base is not implemented yet\n"
	if errs != want || st != 2 {
		t.Errorf("typeset -i16 = %q (status %d), want %q", errs, st, want)
	}
}

// Where the dialect's `-i` takes no base, nothing is refused and the words
// mean what they meant: `-i16` is somebody else's bad option and a bare `16`
// is an operand. The axis is asked only when a base is written, so an
// ordinary `-i` never meets it.
func TestNoBaseIsRefusedWhereTheLetterTakesNone(t *testing.T) {
	out, errs, st := integerRun(t, `typeset -i n=5
n=1+2
echo "n=[$n]"`, func(s *Semantics) {
		s.IntegerAttributeTakesABase = Unspecified
	}, Diagnostics{})
	want := "n=[3]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("typeset -i with no base = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// A base written where the axis is unanswered refuses as an unanswered axis
// rather than guessing that the letter takes none.
func TestABaseWithoutTheAxisRefuses(t *testing.T) {
	_, errs, st := integerRun(t, `typeset -i 16 b=255`, func(s *Semantics) {
		s.IntegerAttributeTakesABase = Unspecified
	}, Diagnostics{})
	want := "testsh: `-i` reading an output base: the shells disagree here and no dialect " +
		"was chosen\n"
	if errs != want || st != 2 {
		t.Errorf("a base with no axis = %q (status %d), want %q", errs, st, want)
	}
}

// A declaring word: an unquoted operand is not field-split, so the whole of
// `1:2` reaches the assignment as one value and the arithmetic complains
// about *it*. A word that split would have assigned `1` and left `2` as a
// second operand, which is silent and wrong.
func TestIntegerIsADeclaringWord(t *testing.T) {
	_, errs, st := integerRun(t, `IFS=:
v='1:2'
integer n=$v`, nil, Diagnostics{})
	want := "testsh: 1:2: operator expected\n"
	if errs != want || st != 2 {
		t.Errorf("integer with an unquoted operand = %q (status %d), want %q", errs, st, want)
	}
}
