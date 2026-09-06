// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// charCode is the core plus the character-code operator. The flag is named
// here and the shell that sets it is not.
func charCode(on bool) syntax.Dialect {
	d := syntax.Core()
	d.ArithCharacterCode = on
	return d
}

// arithOf returns the expression `$(( … ))` parsed to, which is the operand of
// the command.
func arithOf(t *testing.T, src string, d syntax.Dialect) syntax.ArithExpr {
	t.Helper()
	spans := spansOf(t, src, d)
	if len(spans) != 1 || spans[0].Kind != syntax.ArithSubst {
		t.Fatalf("parse %q: operand is not one arithmetic expansion, got %d spans", src, len(spans))
	}
	return spans[0].Arith
}

// The two spellings and what each reads: a name, or one character.
func TestTheCharacterCodeOperatorReadsItsOperand(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		op   string
		nm   string // the parameter, "" when the operand is a character
		char string // the character as written, "" when the operand is a name
		sub  bool
	}{
		{"a name", "echo $((#b))", "#", "b", "", false},
		{"a name with digits", "echo $((#a1))", "#", "a1", "", false},
		{"an underscore", "echo $((#_x))", "#", "_x", "", false},
		{"a positional parameter", "echo $((#1))", "#", "1", "", false},
		{"two digits of one", "echo $((#16))", "#", "16", "", false},
		{"a name with a subscript", "echo $((#a[1]))", "#", "a", "", true},
		{"nothing at all", "echo $(( # ))", "#", "", "", false},
		{"a character", "echo $((##a))", "##", "", "a", false},
		{"a space", "echo $((## ))", "##", "", " ", false},
		{"a hash", "echo $((###))", "##", "", "#", false},
		{"an escape", `echo $((##\n))`, "##", "", `\n`, false},
		{"a hex escape", `echo $((##\x41))`, "##", "", `\x41`, false},
		{"an octal escape", `echo $((##\101))`, "##", "", `\101`, false},
		{"a long unicode escape", `echo $((##\U00000041))`, "##", "", `\U00000041`, false},
		{"a multi-byte character", "echo $((##é))", "##", "", "é", false},
		{"one hash before a backslash", `echo $((#\A))`, "#", "", `\A`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x, ok := arithOf(t, tc.src, charCode(true)).(*syntax.ArithCharCode)
			if !ok {
				t.Fatalf("parse %q: not a character-code node", tc.src)
			}
			if x.Op != tc.op || x.Name != tc.nm || x.Char != tc.char || x.Subscripted != tc.sub {
				t.Errorf("parse %q: op %q name %q char %q sub %v, want %q %q %q %v",
					tc.src, x.Op, x.Name, x.Char, x.Subscripted, tc.op, tc.nm, tc.char, tc.sub)
			}
		})
	}
}

// One character and exactly one: the rest is left for the caller to refuse as
// an operator it cannot read, which is what the shell does.
func TestTheCharacterCodeOperatorTakesOneCharacter(t *testing.T) {
	for _, src := range []string{
		"echo $((##ab))", "echo $((###a))", `echo $((##\x41x))`, `echo $((##\0101))`,
	} {
		if _, err := syntax.Parse(src, charCode(true)); err == nil {
			t.Errorf("parse %q: accepted, want the leftover refused", src)
		}
	}
}

// Nothing after the operator is its own failure, worded by neither of the two
// the rest of arithmetic has.
func TestTheCharacterCodeOperatorWithNothingAfterIt(t *testing.T) {
	for _, src := range []string{"echo $((##))"} {
		_, err := syntax.Parse(src, charCode(true))
		var se *syntax.Error
		if !errors.As(err, &se) {
			t.Fatalf("parse %q: %v, want a syntax error", src, err)
		}
		if se.Kind != syntax.ErrArithCharacterMissing {
			t.Errorf("parse %q: kind %v, want ErrArithCharacterMissing", src, se.Kind)
		}
	}
}

// Without the flag the `#` is not an operator at all, and the refusal is the
// ordinary missing-operand one every other unreadable operand gets — which is
// what the rest of the panel says about the same text.
func TestWithoutTheFlagTheHashIsNoOperator(t *testing.T) {
	for _, src := range []string{"echo $((#b))", "echo $((##a))", "echo $((##))"} {
		_, err := syntax.Parse(src, charCode(false))
		var se *syntax.Error
		if !errors.As(err, &se) {
			t.Fatalf("parse %q without the flag: %v, want a syntax error", src, err)
		}
		if se.Kind != syntax.ErrArithOperand {
			t.Errorf("parse %q without the flag: kind %v, want ErrArithOperand", src, se.Kind)
		}
	}
}

// The `base#digits` literal is untouched in both directions: its `#` follows
// digits and is read by the number, where this one stands where an operand
// belongs.
func TestTheRadixLiteralIsNotTheCharacterCodeOperator(t *testing.T) {
	for _, on := range []bool{true, false} {
		x, ok := arithOf(t, "echo $((16#ff))", charCode(on)).(*syntax.ArithNum)
		if !ok {
			t.Fatalf("with the flag %v: not a number", on)
		}
		if x.Text != "16#ff" {
			t.Errorf("with the flag %v: text %q, want %q", on, x.Text, "16#ff")
		}
	}
}
