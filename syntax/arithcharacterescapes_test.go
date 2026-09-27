// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// How far the one character after a character-code operator reaches when it
// is written as a `\C` or `\M` escape.
//
// The field is named here and the shells that set it are not. What each
// escape *stands for* is decoded elsewhere; all the parser settles is where
// the operand ends, and that is what decides whether the operand is complete
// at all — a span cut short leaves bytes standing where an operator belongs.
func TestTheCharacterCodeOperandSpansTheCaretEscapes(t *testing.T) {
	t.Parallel()
	masked := charCode(true)
	masked.ArithCharacterEscapes = syntax.ArithCharacterEscapesMaskedCaretMeta
	folded := charCode(true)
	folded.ArithCharacterEscapes = syntax.ArithCharacterEscapesFoldedCaret
	plain := charCode(true)
	for _, tc := range []struct {
		name string
		src  string
		dial syntax.Dialect
		char string
	}{
		// Masked: the dash is optional and the argument may be an escape.
		{"masked, with the dash", `echo $((##\C-a))`, masked, `\C-a`},
		{"masked, without the dash", `echo $((##\Ca))`, masked, `\Ca`},
		{"masked meta", `echo $((##\M-a))`, masked, `\M-a`},
		{"masked, an escape as the argument", `echo $((##\M-\C-a))`, masked, `\M-\C-a`},
		{"masked, nothing after it", `echo $((##\C))`, masked, `\C`},
		{"masked, the dash is the argument", `echo $((##\C-))`, masked, `\C-`},

		// Folded: one argument, no dash in the spelling, and `\M-` whole.
		{"folded takes the dash as the argument", `echo $((##\C-+0))`, folded, `\C-`},
		{"folded without a dash", `echo $((##\Ca))`, folded, `\Ca`},
		{"folded meta is the two characters", `echo $((##\M-+0))`, folded, `\M-`},
		{"folded meta without a dash is not an escape", `echo $((##\M+0))`, folded, `\M`},

		// Neither: the backslash claims one character and no more, so the
		// `-a` behind it is read as a subtraction of a parameter.
		{"neither, C", `echo $((##\C-a))`, plain, `\C`},
		{"neither, M", `echo $((##\M-a))`, plain, `\M`},

		// The shared escapes are not this field's business.
		{"masked leaves hexadecimal alone", `echo $((##\x41))`, masked, `\x41`},
		{"folded leaves octal alone", `echo $((##\101))`, folded, `\101`},
		{"masked leaves an unclaimed letter alone", `echo $((##\q))`, masked, `\q`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x, ok := firstCharCode(arithOf(t, tc.src, tc.dial))
			if !ok {
				t.Fatalf("parse %q: no character-code node", tc.src)
			}
			if x.Char != tc.char {
				t.Errorf("parse %q: char %q, want %q", tc.src, x.Char, tc.char)
			}
		})
	}
}

// firstCharCode is the leftmost character-code node of an expression, which a
// row whose operand is followed by an operator needs: the span is the point,
// and what is left standing behind it is read as arithmetic of its own.
func firstCharCode(e syntax.ArithExpr) (*syntax.ArithCharCode, bool) {
	switch x := e.(type) {
	case *syntax.ArithCharCode:
		return x, true
	case *syntax.ArithBinary:
		if n, ok := firstCharCode(x.X); ok {
			return n, true
		}
		return firstCharCode(x.Y)
	case *syntax.ArithUnary:
		return firstCharCode(x.X)
	}
	return nil, false
}

// The other half of a span: where the escape ends before the operand does,
// the character left behind stands where an operator belongs and the
// expression is refused. That is the same refusal ksh93 gives `$(( '\C-a' ))`,
// and it is what says the span is load-bearing rather than cosmetic.
func TestACaretEscapeCutShortLeavesACharacterStanding(t *testing.T) {
	t.Parallel()
	folded := charCode(true)
	folded.ArithCharacterEscapes = syntax.ArithCharacterEscapesFoldedCaret
	for _, tc := range []struct {
		name string
		src  string
		dial syntax.Dialect
	}{
		{"folded, a dash and a letter", `##\C-a`, folded},
		{"folded meta with a letter behind it", `##\M-x`, folded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := arithErr(tc.src, tc.dial); err == nil {
				t.Errorf("read %q: no error, want one", tc.src)
			}
		})
	}
}
