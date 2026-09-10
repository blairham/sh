// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A subscript on the *left* of an assignment, where the name holds a string.
//
// ScalarSubscriptIsACharacter already decided the reading — `${s[2]}` is a
// character rather than an element — and the writing half asked nothing, so a
// string written through a subscript was converted into an array. The axis is
// answered on both sides here: one side splices characters and the other
// reaches an element.
//
// Every case picks a value and a subscript where the two readings give
// *visibly* different strings. A one-character string, or a subscript at the
// base, cannot tell "the character was replaced" from "the string became a
// one-element array" — which is exactly the bug — so each row asserts the
// value and the shape together. `n=` is the element count: 1 for a string
// however long, and more the moment a subscript has built an array.

func spliceSem(character Answer) Semantics {
	s := testSemantics()
	s.ScalarSubscriptIsACharacter = character
	// The rows below are written with the first element at 1 and with a
	// comma reading as a range, which is the combination the splicing shell
	// has. The base is a *parameter* of the splice rather than a backdrop —
	// see TestTheBaseDecidesWhichCharacterASubscriptNames.
	s.ArrayBaseIsZero = No
	s.SubscriptCommaIsARange = Yes
	// So that a negative subscript reaching past the first unit is answered
	// rather than refused on the element side, and the two sides stay
	// comparable.
	s.NegativeSubscriptPastTheStartInserts = Yes
	return s
}

// spliceShape is the value and the element count in one line, which is what
// tells the two readings apart.
const spliceShape = `; printf "[%s]" "${v[@]}"; echo " n=${#v[@]}"`

func runSplice(t *testing.T, sem Semantics, src string) (string, int) {
	t.Helper()
	out, st := runGrammar(t, src, nil, withSem(sem))
	return strings.TrimSpace(out), st
}

func TestASubscriptOnAStringReplacesTheSpanItNames(t *testing.T) {
	for _, tc := range []struct{ write, want string }{
		// The value is not one character wide, and neither is the span: the
		// string grows and shrinks with what goes in. A reading that
		// overwrote a position would answer `aXc` to all three of these.
		{`v[2]=X`, "[aXc] n=1"},
		{`v[2]=XY`, "[aXYc] n=1"},
		{`v[2]=`, "[ac] n=1"},
		// A pair is a span of characters, and the value replaces the whole of
		// it however many characters that is.
		{`v[2,3]=XY`, "[aXY] n=1"},
		{`v[2,3]=X`, "[aX] n=1"},
		{`v[2,3]=XYZW`, "[aXYZW] n=1"},
		{`v[1,-1]=X`, "[X] n=1"},
		{`v[2,-1]=X`, "[aX] n=1"},
		// Past the last character the value is appended and the gap is *not*
		// padded, which is the row an array does differently — and the row
		// that makes `name[${#name}+1]=x` an append rather than an append
		// with a hole in front of it.
		{`v[4]=X`, "[abcX] n=1"},
		{`v[10]=X`, "[abcX] n=1"},
		{`v[${#v}+1]=Q`, "[abcQ] n=1"},
		// A negative subscript counts back from the last character, and one
		// reaching past the first lands in front of everything without taking
		// anything out.
		{`v[-1]=X`, "[abX] n=1"},
		{`v[-2]=X`, "[aXc] n=1"},
		{`v[-4]=X`, "[Xabc] n=1"},
		{`v[-5]=X`, "[Xabc] n=1"},
		// An end before the start is an empty span *at* the start, so the
		// value goes in and nothing comes out.
		{`v[3,2]=X`, "[abXc] n=1"},
		{`v[1,0]=X`, "[Xabc] n=1"},
		// A start below the first character is the first.
		{`v[0,1]=X`, "[Xbc] n=1"},
		// `+=` reads the span it names and puts the value after it, back
		// where the span was. It reads no pair — see subscriptSpan — so the
		// comma there is the arithmetic operator and the span is its right
		// operand: `v[2,3]+=X` joins the character at 3.
		{`v[2]+=X`, "[abXc] n=1"},
		{`v[2,3]+=X`, "[abcX] n=1"},
		{`v[4]+=X`, "[abcX] n=1"},
		{`v[-1]+=X`, "[abcX] n=1"},
	} {
		out, st := runSplice(t, spliceSem(Yes), `v=abc; `+tc.write+spliceShape)
		if out != tc.want || st != 0 {
			t.Errorf("%s: got %q status %d, want %q", tc.write, out, st, tc.want)
		}
	}
}

// TestTheOtherReadingOfTheSameLineReachesAnElement is the axis answered No.
//
// The rows write at the *base*, which is the one subscript both readings can
// be compared at without #1570 in the way: a name holding a string loses that
// string on its way to becoming an array here, and the base is the slot the
// string would have occupied, so the element answer is the same before and
// after that is fixed. What the two readings do with the rest of the string is
// the whole difference — one keeps it and one does not.
func TestTheOtherReadingOfTheSameLineReachesAnElement(t *testing.T) {
	for _, tc := range []struct{ write, characters, elements string }{
		{`v[1]=X`, "[Xbc] n=1", "[X] n=1"},
		{`v[1]=XY`, "[XYbc] n=1", "[XY] n=1"},
		{`v[1]=X; v[2]=Y`, "[XYc] n=1", "[X][Y] n=2"},
	} {
		src := `v=abc; ` + tc.write + spliceShape
		out, st := runSplice(t, spliceSem(Yes), src)
		if out != tc.characters || st != 0 {
			t.Errorf("characters, %s: got %q status %d, want %q",
				tc.write, out, st, tc.characters)
		}
		out, st = runSplice(t, spliceSem(No), src)
		if out != tc.elements || st != 0 {
			t.Errorf("elements, %s: got %q status %d, want %q",
				tc.write, out, st, tc.elements)
		}
	}
}

// TestAStringSplicedAtASubscriptStaysAString is the assertion no single value
// can make. `v[1]=X` on a one-character string is `X` under *both* readings,
// so a suite that stopped at the value would pass with the bug in place; the
// second write is what shows which of the two happened.
func TestAStringSplicedAtASubscriptStaysAString(t *testing.T) {
	const src = `v=z; v[1]=X; v[2]=Y` + spliceShape
	out, st := runSplice(t, spliceSem(Yes), src)
	if want := "[XY] n=1"; out != want || st != 0 {
		t.Errorf("characters: got %q status %d, want %q", out, st, want)
	}
	out, st = runSplice(t, spliceSem(No), src)
	if want := "[X][Y] n=2"; out != want || st != 0 {
		t.Errorf("elements: got %q status %d, want %q", out, st, want)
	}
}

// TestOnlyANameAlreadyHoldingAStringSplices is the boundary, and it is
// measured rather than reasoned: a name holding the empty string splices, and
// a name nobody set becomes an array. The two lines differ only in whether
// there is a name there at all.
func TestOnlyANameAlreadyHoldingAStringSplices(t *testing.T) {
	// A sparse array with one element joins and counts exactly as a
	// one-character string does, so neither the joined list nor the count can
	// tell `v=; v[2]=X` from `unset v; v[2]=X`. The two subscripts can: the
	// string holds its one character at the *first* position and the array
	// holds its one element at the *second*.
	const probe = `; echo "[$v] one=${v[1]} two=${v[2]}"`
	for _, tc := range []struct{ start, want string }{
		{`v=abc;`, "[aXc] one=a two=X"},
		// Nothing to splice into, so the value stands alone rather than
		// arriving at character 2 of an empty string with padding in front.
		{`v=;`, "[X] one=X two="},
		// Unset: an array, and the value is at the subscript.
		{`v=abc; unset v;`, "[] one= two=X"},
		{``, "[] one= two=X"},
		// An array stays an array. The splice asks about a name holding a
		// *string* and this is not one.
		{`v=(p q r);`, "[p] one=p two=X"},
	} {
		out, st := runSplice(t, spliceSem(Yes), tc.start+` v[2]=X`+probe)
		if out != tc.want || st != 0 {
			t.Errorf("%q: got %q status %d, want %q", tc.start, out, st, tc.want)
		}
	}
}

// TestTheBaseDecidesWhichCharacterASubscriptNames: the splice counts from the
// dialect's base and from nowhere else, so the option that moves an array's
// first element moves a string's first character with it — and takes the
// refusal with it, because which subscript is *below* the first is one
// question. See ArrayBaseIsZero.
func TestTheBaseDecidesWhichCharacterASubscriptNames(t *testing.T) {
	for _, tc := range []struct {
		write, fromOne, fromZero string
		refusedFromOne           bool
	}{
		{`v[0]=X`, "", "[Xbc] n=1", true},
		{`v[1]=X`, "[Xbc] n=1", "[aXc] n=1", false},
		{`v[2]=X`, "[aXc] n=1", "[abX] n=1", false},
		{`v[3]=X`, "[abX] n=1", "[abcX] n=1", false},
		{`v[1,2]=XY`, "[XYc] n=1", "[aXY] n=1", false},
		// A negative subscript counts back from the last character under
		// either base: that half does not move.
		{`v[-1]=X`, "[abX] n=1", "[abX] n=1", false},
	} {
		src := `v=abc; ` + tc.write + spliceShape
		sem := spliceSem(Yes)
		out, st := runSplice(t, sem, src)
		switch {
		case tc.refusedFromOne:
			if st == 0 {
				t.Errorf("base 1, %s: status 0 with %q, want a refusal", tc.write, out)
			}
		case out != tc.fromOne || st != 0:
			t.Errorf("base 1, %s: got %q status %d, want %q", tc.write, out, st, tc.fromOne)
		}
		sem.ArrayBaseIsZero = Yes
		out, st = runSplice(t, sem, src)
		if out != tc.fromZero || st != 0 {
			t.Errorf("base 0, %s: got %q status %d, want %q", tc.write, out, st, tc.fromZero)
		}
	}
}

// TestASubscriptBelowTheFirstCharacterIsRefused is the array's refusal reached
// through a string: the script ends there rather than carrying on with the
// character not written.
func TestASubscriptBelowTheFirstCharacterIsRefused(t *testing.T) {
	for _, write := range []string{`v[0]=X`, `v[0,0]=X`} {
		out, st := runSplice(t, spliceSem(Yes), `v=abc; `+write+`; echo reached`)
		if st == 0 {
			t.Errorf("%s: status = 0, want a refusal", write)
		}
		if strings.Contains(out, "reached") {
			t.Errorf("%s: the script carried on: %q", write, out)
		}
	}
}

// TestAStringIsSplicedThroughEveryWayOfWritingAnElement: the rule is at the
// store, so the spellings that reach it cannot disagree. Stated at the
// assignment statement alone, the others would go on converting the string.
func TestAStringIsSplicedThroughEveryWayOfWritingAnElement(t *testing.T) {
	for _, tc := range []struct{ write, want string }{
		{`v[2]=X`, "[aXc] n=1"},
		{`(( v[2] = 88 ))`, "[a88c] n=1"},
	} {
		out, st := runSplice(t, spliceSem(Yes), `v=abc; `+tc.write+spliceShape)
		if out != tc.want || st != 0 {
			t.Errorf("%s: got %q status %d, want %q", tc.write, out, st, tc.want)
		}
	}
}

// TestReadStoresThroughASubscriptedOperand. `read` took its operand as a whole
// name and stored it with setVar, so `read 'v[2]'` created a parameter *called*
// `v[2]` and left `v` alone — nothing assigned, nothing said, status 0. The
// element half of that is no dialect's question: every shell measured fills the
// element for the array spelling.
func TestReadStoresThroughASubscriptedOperand(t *testing.T) {
	for _, tc := range []struct{ start, operand, want string }{
		{`v=xy;`, `v[2]`, "[xQ] n=1"},
		{`v=xy;`, `v[${#v}+1]`, "[xyQ] n=1"},
		{`v=xy;`, `v[2,3]`, "[xQ] n=1"},
		// An array's element, which is what the whole panel does.
		{`v=(p q r);`, `v[2]`, "[p][Q][r] n=3"},
	} {
		src := tc.start + " read '" + tc.operand + "' <<IN\nQ\nIN\n" +
			strings.TrimPrefix(spliceShape, "; ")
		out, st := runSplice(t, spliceSem(Yes), src)
		if out != tc.want || st != 0 {
			t.Errorf("read %s: got %q status %d, want %q", tc.operand, out, st, tc.want)
		}
	}
}
