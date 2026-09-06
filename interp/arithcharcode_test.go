// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// runCharCode runs src under a grammar that has the character-code operator.
func runCharCode(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArithCharacterCode = true
	}, nil)
}

// The operator yields a character *code* and never a count, which is the whole
// reason it is worth having a test that says so: `$((#a))` looks like a length
// and the length is `$(( $#a ))`.
func TestTheCharacterCodeOperatorIsNotALength(t *testing.T) {
	out, st := runCharCode(t, `a=(1 2); printf "[%s][%s]" "$((#a))" "${#a[@]}"`)
	if out != "[49][2]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[49][2]")
	}
}

// What each operand reads, and what has no answer at all.
func TestTheCharacterCodeOperatorReadsItsOperand(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a value's first character", `b=zebra; printf "%s" $((#b))`, "122"},
		{"a name never set", `printf "%s" $((#nothing))`, "0"},
		{"a name holding nothing", `b=; printf "%s" $((#b))`, "0"},
		{"no operand at all", `printf "%s" $(( # ))`, "0"},
		{"a subscript finds nothing", `a=(xy z); printf "%s %s" $((#a)) $((#a[2]))`, "120 0"},
		{"the value, not the name it holds", `b=zebra; c=b; printf "%s" $((#c))`, "98"},
		{"a positional parameter", `set -- hello; printf "%s" $((#1))`, "104"},
		{"a character written out", `printf "%s %s" $((##a)) $((##A))`, "97 65"},
		{"an escape the quoting decodes", `printf "%s %s" $((##\n)) $((##\x41))`, "10 65"},
		{"a backslash under one hash is literal", `printf "%s %s" $((#\n)) $((#\A))`, "110 65"},
		{"a character and not a byte", `b=é; printf "%s %s" $((#b)) $((##é))`, "233 233"},
		{"it composes like any value", `b=zebra; printf "%s %s" $((#b + 1)) $(( -#b ))`, "123 -122"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runCharCode(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The radix literal is untouched with the flag on: its `#` follows digits and
// is read by the number, where this one stands where an operand belongs.
func TestTheRadixLiteralIsUnaffected(t *testing.T) {
	out, st := runCharCode(t, `printf "%s %s %s" $((16#ff)) $((2#101)) $((8#17))`)
	if out != "255 5 15" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "255 5 15")
	}
}
