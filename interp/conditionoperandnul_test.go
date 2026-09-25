// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// nulIsAByte answers the one axis every row here depends on: a NUL written
// inside `$'…'` is a byte of the text rather than the end of the span or
// nothing at all. Without it the core refuses the spelling, which is the
// right refusal and not what these tests are about.
func nulIsAByte(r *Runner) {
	s := *r.Semantics
	s.DollarSingleNul = DollarSingleNulIsAByte
	r.Semantics = &s
}

// valuesBracketIsPartOfTheKey adds the axis the control below needs: a
// subscript inside a condition read as arithmetic takes the brackets the
// *script* wrote as its own, so the ones a value carries are characters.
func valuesBracketIsPartOfTheKey(r *Runner) {
	nulIsAByte(r)
	s := *r.Semantics
	s.ConditionArithmeticReadsTheWrittenSubscript = Yes
	r.Semantics = &s
}

// A NUL a value carries has to survive a condition's operand, and the reason
// it did not is that syntax.ArithValueMark *is* the NUL byte.
//
// The mark says which bytes of a subscript came out of a value, and its own
// comment states the invariant the encoding rests on: a mark in front of a
// mark is a NUL that was data, so a bare one can only be the mark. A
// condition's left operand is expanded marked and unmarked again by its
// readers, and the marking skipped every span outside brackets the *script*
// wrote — so a data NUL arrived bare and the unmarking took it and the byte
// behind it with it.
//
// The shape that measurement leaves is the whole tell and it is what these
// tests pin: a NUL survived at the **end** of a value and nowhere else,
// because at the end there is no byte behind it to be eaten.
//
// Tests name axes and constructs, never shells.

// nulValueRows are the values a condition has to compare with itself.
//
// The last two are the controls in the two directions a null result needs:
// a value with no NUL in it at all, which would agree whatever the encoding
// did, and a value that is *nothing but* a NUL, which agreed before this was
// fixed and so cannot be the evidence.
var nulValueRows = []struct{ name, spelling string }{
	{"a NUL alone", `$'\0'`},
	{"a NUL at the end", `$'a\0'`},
	{"a NUL at the start", `$'\0b'`},
	{"a NUL in the middle", `$'a\0b'`},
	{"two NULs in the middle", `$'a\0\0b'`},
	{"no NUL at all", `abc`},
}

func TestAConditionsOperandKeepsANULTheValueCarried(t *testing.T) {
	for _, tc := range nulValueRows {
		t.Run(tc.name, func(t *testing.T) {
			src := `v=` + tc.spelling + `; [[ $v == "$v" ]] && printf same || printf differs`
			if got, st := run(t, src, nulIsAByte); got != "same" || st != 0 {
				t.Errorf("%q (status %d), want %q at 0", got, st, "same")
			}
		})
	}
}

// The discriminating half, and the one a self-comparison cannot give: both
// sides of `[[ $v == "$v" ]]` come through operands, so an encoding that
// dropped the NUL on *both* would agree with itself. These compare the value
// against text that says how many characters it has.
//
// The length is asserted beside them from a route that does not come through
// a condition at all, so a row that reads "three characters" is not reading
// its own answer back.
func TestAConditionsOperandDoesNotShortenTheValueItCompares(t *testing.T) {
	const decl = `v=$'a\0b'; `
	for _, tc := range []struct{ name, cond, want string }{
		// What the shortening looked like: the two surviving bytes.
		{"the two bytes either side of it", `[[ $v == ab ]]`, "differs"},
		{"a pattern of two characters", `[[ $v == ?? ]]`, "differs"},
		// And what the value really is.
		{"a pattern of three characters", `[[ $v == ??? ]]`, "same"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := decl + tc.cond + ` && printf same || printf differs`
			if got, st := run(t, src, nulIsAByte); got != tc.want || st != 0 {
				t.Errorf("%q (status %d), want %q at 0", got, st, tc.want)
			}
		})
	}
	if got, st := run(t, decl+`printf '%s' "${#v}"`, nulIsAByte); got != "3" || st != 0 {
		t.Errorf("length off a route with no condition in it: %q (status %d), want %q at 0", got, st, "3")
	}
}

// The control that says the marking still does the job it was put there for:
// a `]` that arrived in a value inside brackets the script wrote is a
// character of the key and does not close the subscript.
//
// It is here rather than left to its own file because this change moves the
// marking, and a fix that marked everything unconditionally — or that stopped
// marking — would pass every row above and break this one.
func TestAValuesBracketInsideAWrittenSubscriptIsStillPartOfTheKey(t *testing.T) {
	const src = `typeset -A m; k='x]'; m[$k]=5; m[x]=9; [[ m[$k] -eq 5 ]] && printf key || printf other`
	if got, st := run(t, src, valuesBracketIsPartOfTheKey); got != "key" || st != 0 {
		t.Errorf("%q (status %d), want %q at 0", got, st, "key")
	}
}
