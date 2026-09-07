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
	// The majority reading of ten among the two shells that have a base is
	// that it is a base like any other, so a test not about that axis gets
	// it answered rather than meeting an unanswered question.
	sem.IntegerBaseTenIsNoBase = No
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

// An output base is read where the dialect's `-i` takes one, in both
// spellings, and the name renders in it. The attached one has to be caught
// before the letter loop reaches the `1` and calls it an unknown option — a
// true statement about a letter the script never wrote.
func TestAnOutputBaseIsReadInBothSpellings(t *testing.T) {
	for _, src := range []string{`integer -i 16 b=255`, `typeset -i16 b=255`} {
		out, errs, st := integerRun(t, src+"\necho \"b=[$b]\"", func(s *Semantics) {
			s.IntegerBaseDigits = "0123456789abcdefghijklmnopqrstuvwxyz"
		}, Diagnostics{})
		if out != "b=[16#ff]\n" || st != 0 || errs != "" {
			t.Errorf("%s = %q (stderr %q, status %d), want %q", src, out, errs, st, "b=[16#ff]\n")
		}
	}
}

// The alphabet is the whole of what a dialect can spell: its case and its
// length, which is the largest base. A base past the end is taken and
// rendered plain where the dialect has no complaint for it.
func TestTheOutputBaseAlphabetSaysTheCaseAndTheRange(t *testing.T) {
	for _, tc := range []struct{ digits, want string }{
		{"0123456789abcdefghijklmnopqrstuvwxyz", "b=[36#2s]\n"},
		{"0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ", "b=[36#2S]\n"},
		{"0123456789", "b=[100]\n"},
		{"", "b=[100]\n"},
	} {
		out, errs, st := integerRun(t, "typeset -i36 b=100\necho \"b=[$b]\"",
			func(s *Semantics) { s.IntegerBaseDigits = tc.digits }, Diagnostics{})
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("digits %q = %q (stderr %q, status %d), want %q",
				tc.digits, out, errs, st, tc.want)
		}
	}
}

// A base outside the alphabet is refused where the dialect has a complaint
// for it, and the name is left with nothing. With none it is taken in
// silence, which is the other shell — see IntegerBadBase.
func TestABaseOutsideTheAlphabetIsRefusedWhereThereIsAWording(t *testing.T) {
	out, errs, st := integerRun(t, "typeset -i64 b=100\necho \"b=[$b]\"",
		func(s *Semantics) { s.IntegerBaseDigits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ" },
		Diagnostics{IntegerBadBase: "%[1]s: invalid base: %[2]s"})
	if errs != "testsh: typeset: invalid base: 64\n" || out != "b=[]\n" || st != 0 {
		t.Errorf("got %q (stderr %q, status %d), want the base named and nothing stored", out, errs, st)
	}
}

// Base ten is valid everywhere and marks nothing, which is why the two
// questions cannot be one: joining them made the dialect that refuses a bad
// base refuse base ten as well.
func TestBaseTenIsTakenAndRendersPlain(t *testing.T) {
	out, errs, st := integerRun(t, "typeset -i10 b=255\necho \"b=[$b]\"",
		func(s *Semantics) { s.IntegerBaseDigits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ" },
		Diagnostics{IntegerBadBase: "invalid base: %[2]s"})
	if out != "b=[255]\n" || st != 0 || errs != "" {
		t.Errorf("got %q (stderr %q, status %d), want %q", out, errs, st, "b=[255]\n")
	}
}

// A negative value under a base is two answers: the bit pattern, or a sign in
// front of the magnitude.
func TestANegativeValueUnderAnOutputBase(t *testing.T) {
	for _, tc := range []struct {
		twos Answer
		want string
	}{
		{Yes, "b=[16#ffffffffffffff01]\n"},
		{No, "b=[-16#ff]\n"},
	} {
		out, errs, st := integerRun(t, "typeset -i16 b=-255\necho \"b=[$b]\"",
			func(s *Semantics) {
				s.IntegerBaseDigits = "0123456789abcdefghijklmnopqrstuvwxyz"
				s.IntegerBaseNegativeIsTwosComplement = tc.twos
			}, Diagnostics{})
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("%v = %q (stderr %q, status %d), want %q", tc.twos, out, errs, st, tc.want)
		}
	}
}

// The base learned from the value assigned, which one dialect does and the
// other does not, and which sticks to the name once learned.
func TestTheOutputBaseLearnedFromTheValue(t *testing.T) {
	for _, tc := range []struct {
		learns Answer
		want   string
	}{
		// Row 4 reads 14 rather than 16 because this test vector's
		// arithmetic takes a leading zero as octal, which is a different
		// question and one the panel splits on. What the row is here for is
		// that `016` teaches the name no *base* under either answer.
		{Yes, "1[16#10]\n2[16#5]\n3[8#143]\n4[14]\n5[16]\n"},
		{No, "1[16]\n2[5]\n3[99]\n4[14]\n5[16]\n"},
	} {
		out, errs, st := integerRun(t, `typeset -i a; a=0x10; echo "1[$a]"
typeset -i b; b=0x10; b=5; echo "2[$b]"
typeset -i c; c=8#7; c=99; echo "3[$c]"
typeset -i d; d=016; echo "4[$d]"
typeset -i e; e=$((0x10)); echo "5[$e]"`,
			func(s *Semantics) {
				s.IntegerBaseDigits = "0123456789abcdefghijklmnopqrstuvwxyz"
				s.IntegerBaseComesFromTheValueAssigned = tc.learns
			}, Diagnostics{})
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("%v = %q (stderr %q, status %d), want %q", tc.learns, out, errs, st, tc.want)
		}
	}
}

// The base belongs to the *name*: a second declaration with a different base
// re-renders what the name is already holding, and the plus form takes the
// base off with the attribute while leaving the text that is there alone.
func TestTheOutputBaseBelongsToTheName(t *testing.T) {
	out, errs, st := integerRun(t, `typeset -i b=5
typeset -i16 b
echo "1[$b]"
typeset -i8 b
echo "2[$b]"
typeset +i b
echo "3[$b]"
b=3
echo "4[$b]"`, func(s *Semantics) {
		s.IntegerBaseDigits = "0123456789abcdefghijklmnopqrstuvwxyz"
		// Both shells with an output base answer this one yes; the
		// re-render below is not it, and answering it keeps the two apart.
		s.AttributeRereadsTheValueItFinds = Yes
	}, Diagnostics{})
	want := "1[16#5]\n2[8#5]\n3[8#5]\n4[3]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("got %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The rendered text is what is *stored*, not a way of printing what is: it is
// what a length counts, what a copy copies, and what arithmetic parses back.
func TestABasedValueIsTheValue(t *testing.T) {
	out, errs, st := integerRun(t, `typeset -i16 h=255
echo "len=${#h} copy=$(echo "$h") arith=$(( h + 1 )) strip=${h#16}"`,
		func(s *Semantics) {
			s.IntegerBaseDigits = "0123456789abcdefghijklmnopqrstuvwxyz"
		}, Diagnostics{})
	want := "len=5 copy=16#ff arith=256 strip=#ff\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("got %q (stderr %q, status %d), want %q", out, errs, st, want)
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

// Whether ten is a base of its own or the letter's default, which decides two
// things at once: what a written ten records, and what a *bare* letter does to
// a base the name already has.
func TestWhetherBaseTenIsNoBaseAtAll(t *testing.T) {
	for _, tc := range []struct {
		ten  Answer
		want string
	}{
		// Yes: the letter always names a base and ten is what it names when
		// nothing is written, so the bare letter takes the base off and the
		// listing has no base word.
		{Yes, "1[255]\n2[255]\ntypeset -i d='255'\n"},
		// No: ten is a state, recorded and listed back, and the bare letter
		// leaves the base where it was.
		{No, "1[16#ff]\n2[255]\ntypeset -i 10 d=255\n"},
	} {
		out, errs, st := integerRun(t, `typeset -i16 a=255
typeset -i a
echo "1[$a]"
typeset -i16 b=255
typeset -i10 b
echo "2[$b]"
typeset -i10 d=255
typeset -p d`, func(s *Semantics) {
			s.IntegerBaseDigits = "0123456789abcdefghijklmnopqrstuvwxyz"
			s.IntegerBaseTenIsNoBase = tc.ten
			s.DeclareListing = DeclareListingBareAssignments
			// The re-render a second declaration performs is not a re-read,
			// and answering this keeps the two apart — see
			// TestTheOutputBaseBelongsToTheName.
			s.AttributeRereadsTheValueItFinds = Yes
		}, Diagnostics{})
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("%v = %q (stderr %q, status %d), want %q", tc.ten, out, errs, st, tc.want)
		}
	}
}

// The control the axis needs: another letter over a based name is not this
// question, and neither answer may touch the base. Without it, "the bare
// letter clears the base" could be implemented as "any second declaration
// clears it" and every row above would still pass.
func TestAnotherLetterLeavesTheOutputBaseAlone(t *testing.T) {
	for _, ten := range []Answer{Yes, No} {
		out, errs, st := integerRun(t, `typeset -i16 c=255
typeset -x c
echo "1[$c]"`, func(s *Semantics) {
			s.IntegerBaseDigits = "0123456789abcdefghijklmnopqrstuvwxyz"
			s.IntegerBaseTenIsNoBase = ten
			// Declaring over a standing value is a question of its own and
			// not this one; answering it keeps the two apart.
			s.AttributeRereadsTheValueItFinds = Yes
		}, Diagnostics{})
		if out != "1[16#ff]\n" || st != 0 || errs != "" {
			t.Errorf("%v = %q (stderr %q, status %d), want %q", ten, out, errs, st, "1[16#ff]\n")
		}
	}
}

// An unanswered axis refuses by name rather than picking a shell, and it is
// asked only where it decides something — which is why the ordinary `typeset
// -i n` above it says nothing at all.
func TestBaseTenAsNoBaseUnansweredRefusesByName(t *testing.T) {
	out, errs, _ := integerRun(t, `typeset -i n=5
echo "plain=[$n]"
typeset -i16 h=255
typeset -i h
echo "st=$? h=[$h]"`, func(s *Semantics) {
		s.IntegerBaseDigits = "0123456789abcdefghijklmnopqrstuvwxyz"
		s.IntegerBaseTenIsNoBase = Unspecified
		s.AttributeRereadsTheValueItFinds = Yes
	}, Diagnostics{})
	if !strings.Contains(errs, "base ten as the absence of an output base") {
		t.Errorf("stderr %q, want the axis named", errs)
	}
	// The first line is the whole point of asking so late: a declaration
	// with no base to lose never meets the question and answers as it did.
	if !strings.HasPrefix(out, "plain=[5]\n") {
		t.Errorf("got %q, want the declaration with no base answered anyway", out)
	}
	if !strings.Contains(out, "st=2 ") {
		t.Errorf("got %q, want the refused declaration's own status", out)
	}
}

// An unanswered `-i` reading a base refuses by name and consumes nothing: the
// base word is not a base in a dialect that has not said the letter takes
// one, so the declaration goes no further. Both shapes matter — a base with a
// name after it, and a base that is the last word — because the second is
// where a consumed word would leave a declaration with no operands and let it
// list instead of refusing.
func TestAnUnansweredOutputBaseAxisConsumesNothing(t *testing.T) {
	// The third is the shape that told the two readings apart: a word after
	// the base is a *name*, so an engine that consumed the base anyway asks
	// the question a second time and says so twice for one declaration.
	for _, src := range []string{"typeset -i 16 b=255", "typeset -i 16", "typeset -i 16 16"} {
		out, errs, st := integerRun(t, src, func(s *Semantics) {
			s.IntegerAttributeTakesABase = Unspecified
		}, Diagnostics{})
		if !strings.Contains(errs, "`-i` reading an output base") {
			t.Errorf("%s: stderr %q, want the axis named", src, errs)
		}
		if out != "" || st != 2 {
			t.Errorf("%s = %q (status %d), want nothing listed and a refusal", src, out, st)
		}
		if n := strings.Count(errs, "`-i` reading an output base"); n != 1 {
			t.Errorf("%s named the axis %d times, want once", src, n)
		}
	}
}

// A sign in front of a radix prefix does not hide it: the base is learned
// from `-0x10` the same as from `0x10`, and the sign then renders the way a
// negative under a base renders anywhere. Measured in the shell that learns.
func TestASignedLiteralStillTeachesTheBase(t *testing.T) {
	out, errs, st := integerRun(t, `typeset -i b; b=-0x10; echo "1[$b]"
typeset -i c; c=-8#7; echo "2[$c]"
typeset -i d; d=+0x10; echo "3[$d]"`, func(s *Semantics) {
		s.IntegerBaseDigits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		s.IntegerBaseComesFromTheValueAssigned = Yes
		s.IntegerBaseNegativeIsTwosComplement = No
	}, Diagnostics{})
	want := "1[-16#10]\n2[-8#7]\n3[16#10]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("got %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A base learned from the value is recorded like one named on the letter, and
// ten is one of them: it is a base the dialect *takes*, so the learning path
// keeps it even though nothing is written in it. Only the listing tells the
// two readings apart — the value is a plain `5` either way.
func TestALearnedBaseOfTenIsStillRecorded(t *testing.T) {
	for _, tc := range []struct {
		ten  Answer
		want string
	}{
		{No, "5\ntypeset -i 10 b=5\n"},
		// Where ten is the letter's default it records nothing, learned or
		// written: the same answer the letter gives.
		{Yes, "5\ntypeset -i b='5'\n"},
	} {
		out, errs, st := integerRun(t, "typeset -i b\nb=10#5\necho \"$b\"\ntypeset -p b",
			func(s *Semantics) {
				s.IntegerBaseDigits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
				s.IntegerBaseComesFromTheValueAssigned = Yes
				s.IntegerBaseTenIsNoBase = tc.ten
				s.DeclareListing = DeclareListingBareAssignments
			}, Diagnostics{})
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("%v = %q (stderr %q, status %d), want %q", tc.ten, out, errs, st, tc.want)
		}
	}
}

// A value the name is already holding is re-rendered when a base arrives, and
// a negative one keeps its sign through that: the number does not change,
// only the characters it is written in.
func TestANegativeValueSurvivesARerender(t *testing.T) {
	for _, tc := range []struct {
		twos Answer
		want string
	}{
		{No, "1[-16#5]\n"},
		{Yes, "1[16#fffffffffffffffb]\n"},
	} {
		out, errs, st := integerRun(t, "typeset -i a=-5\ntypeset -i16 a\necho \"1[$a]\"",
			func(s *Semantics) {
				s.IntegerBaseDigits = "0123456789abcdefghijklmnopqrstuvwxyz"
				s.IntegerBaseNegativeIsTwosComplement = tc.twos
				s.AttributeRereadsTheValueItFinds = Yes
			}, Diagnostics{})
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("%v = %q (stderr %q, status %d), want %q", tc.twos, out, errs, st, tc.want)
		}
	}
}

// An unanswered sign axis refuses by name and renders the number plainly
// rather than picking one shell's spelling of a negative.
func TestAnUnansweredNegativeSignAxisRefusesByName(t *testing.T) {
	out, errs, _ := integerRun(t, "typeset -i16 h=-255\necho \"1[$h]\"",
		func(s *Semantics) {
			s.IntegerBaseDigits = "0123456789abcdefghijklmnopqrstuvwxyz"
			s.IntegerBaseNegativeIsTwosComplement = Unspecified
		}, Diagnostics{})
	if !strings.Contains(errs, "a negative integer rendered in its output base") {
		t.Errorf("stderr %q, want the axis named", errs)
	}
	if out != "1[]\n" && out != "1[-255]\n" {
		t.Errorf("got %q, want the refusal to leave no spelling behind", out)
	}
	if strings.Contains(out, "#") {
		t.Errorf("got %q, want no base written for an unanswered sign", out)
	}
	// The re-render is the path where the refusal has to be honored on its
	// own: an assignment is refused above by the guard that follows it, and
	// this one writes the name itself.
	out, errs, _ = integerRun(t, "typeset -i a=-5\ntypeset -i16 a\necho \"2[$a]\"",
		func(s *Semantics) {
			s.IntegerBaseDigits = "0123456789abcdefghijklmnopqrstuvwxyz"
			s.IntegerBaseNegativeIsTwosComplement = Unspecified
			s.AttributeRereadsTheValueItFinds = Yes
		}, Diagnostics{})
	if !strings.Contains(errs, "a negative integer rendered in its output base") {
		t.Errorf("re-render: stderr %q, want the axis named", errs)
	}
	if out != "2[-5]\n" {
		t.Errorf("re-render gave %q, want the number left as it was", out)
	}
}
