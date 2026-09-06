// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// `#` in arithmetic is a character code, end to end through this dialect.
//
// Measured 2026-09-05 on zsh 5.9.2, which is the whole panel for this: bash
// 5.3.15, bash 3.2.57, bash as `sh`, ksh93 and dash all call `$((#b))` an
// arithmetic syntax error naming the operand they could not read.
//
// The first row is the reason the operator is worth recording at all. It looks
// like a length and is not one — `a=(1 2); $((#a))` is 49, the code of the
// `1`, where the count is `$(( $#a ))`, which #846 made spellable.
func TestTheCharacterCodeOperator(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an array is not a count", `a=(1 2); echo $((#a)) $(( $#a ))`, "49 2\n"},
		{"a string's first character", `b=zebra; echo $((#b))`, "122\n"},
		{"a name never set", `echo $((#nothing))`, "0\n"},
		{"a name holding nothing", `b=; echo $((#b))`, "0\n"},
		{"no operand at all", `echo $(( # ))`, "0\n"},
		{"the value and not the name it holds", `b=zebra; c=b; echo $((#c))`, "98\n"},
		{"a positional parameter", `set -- hello; echo $((#1))`, "104\n"},
		{"a positional never given", `echo $((#1))`, "0\n"},
		{"a subscript finds nothing", `a=(xy z); echo $((#a)) $((#a[1])) $((#a[2]))`, "120 0 0\n"},
		{"it is a value like any other", `b=zebra; echo $((#b + 1)) $((2 * #b)) $(( -#b ))`, "123 244 -122\n"},
		{"inside parentheses", `b=zebra; echo $(( ( #b ) ))`, "122\n"},
		{"as an arithmetic command", `b=zebra; (( x = #b )); echo $x`, "122\n"},
		{"a multi-byte value", `b=é; echo $((#b))`, "233\n"},
		{"the integer attribute is still text", `typeset -i n=65; echo $((#n))`, "54\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// `##` takes the character written out rather than a parameter, and it decodes
// the escapes `$'…'` decodes — where a single `#` before a backslash takes the
// next character as itself. Measured: `$((##\n))` is 10 and `$((#\n))` is 110.
func TestTheCharacterCodeOperatorOnACharacter(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a letter", `echo $((##a)) $((##A)) $((##0))`, "97 65 48\n"},
		{"a space", `echo $((## ))`, "32\n"},
		{"a multi-byte character", `echo $((##é))`, "233\n"},
		{"the named escapes", `echo $((##\n)) $((##\t)) $((##\e))`, "10 9 27\n"},
		{"a backslash", `echo $((##\\))`, "92\n"},
		{"an escape nothing claims", `echo $((##\q))`, "113\n"},
		{"hexadecimal", `echo $((##\x41))`, "65\n"},
		{"octal", `echo $((##\101)) $((##\047))`, "65 39\n"},
		{"a unicode escape", `echo $((##\u0041))`, "65\n"},
		{"a wide unicode escape", `echo $((##\U00000041))`, "65\n"},
		{"one hash before a backslash is literal", `echo $((#\A)) $((#\n)) $((#\ ))`, "65 110 32\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// Nothing after the operator is refused, in words that are neither of the two
// the rest of arithmetic uses — and one character and no more is taken, so
// anything past it is left over and refused as text where an operator belonged.
func TestTheCharacterCodeOperatorRefusals(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo $((##))`, "bad math expression: character missing after ##"},
		{`echo $((##ab))`, "bad math expression: operator expected at `b'"},
		{`echo $((##\x41x))`, "bad math expression: operator expected at `x'"},
		// A single `#` has no escapes, so `\x` is the letter x and the `41`
		// is left over — where the doubled spelling reads the whole of it.
		{`echo $((#\x41))`, "bad math expression: operator expected at `41'"},
		{`echo $((#\101))`, "bad math expression: operator expected at `01'"},
		// Four hex digits for the narrow escape and eight for the wide one,
		// so a fifth is left over here and a ninth would be there.
		{`echo $((##\u00410))`, "bad math expression: operator expected at `0'"},
		{`echo $((# b))`, "bad math expression: operator expected at `b'"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if err == nil {
			t.Errorf("%s parsed, want a refusal", tc.src)
			continue
		}
		if got := zsh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%s said %q, want %q", tc.src, got, tc.want)
		}
	}
}

// The `base#digits` literal is not this operator and keeps working, which is
// the one place the two spellings could have collided.
func TestTheRadixLiteralStillReads(t *testing.T) {
	out, st := answersRun(t, `echo $((16#ff)) $((2#101)) $((8#17))`)
	if want := "255 5 15\n"; out != want || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, want)
	}
}
