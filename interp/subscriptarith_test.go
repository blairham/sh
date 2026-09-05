// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// A subscript is an arithmetic expression wherever one is written, and not
// only where one is assigned through.
//
// Reading, taking a length, assigning through `:=` and unsetting each took a
// numeral and nothing else, so every other spelling silently came back empty —
// while `a[1+1]=v` had already learned to store at 2. Two spellings of one
// subscript naming two different elements is the sharpest form of it: the
// script writes with an expression, reads back with the same expression, and
// finds nothing.
func TestASubscriptIsAnExpressionWhereverItIsWritten(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// Reading one.
		{`a=(x y z); echo "[${a[1+1]}]"`, "[z]"},
		{`a=(x y z); i=1; echo "[${a[i+1]}]"`, "[z]"},
		{`a=(x y z); i=2; echo "[${a[i]}]"`, "[z]"},
		{`a=(x y z); i=1; echo "[${a[$i+1]}]"`, "[z]"},
		// An unset name is 0, which is what the evaluator says of any bare
		// name — not an error, and not a subscript that quietly finds nothing.
		{`a=(x y z); echo "[${a[k]}]"`, "[x]"},
		// The operators reach the element the expression names.
		{`a=(xx yy zzz); echo "[${#a[1+1]}]"`, "[3]"},
		{`a=(x y z); echo "[${a[1+1]:-d}]"`, "[z]"},
		{`a=(x y z); echo "[${a[1+1]+set}]"`, "[set]"},
		{`a=(hello there); echo "[${a[0+1]#th}]"`, "[ere]"},
		// Assigning through one stores where the expression says.
		{`a=(x y); echo "[${a[1+1]:=Q}]"; echo "n=${#a[@]}"`, "[Q]\nn=3"},
		// The whole-array subscripts are not expressions and must not be
		// evaluated as one.
		{`a=(x y z); echo "[${a[@]}]"`, "[x y z]"},
		{`a=(x y z); echo "[${a[*]}]"`, "[x y z]"},
	} {
		out, st := runArray(t, c.src)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
		if st != 0 {
			t.Errorf("%s: status = %d, want 0", c.src, st)
		}
	}
}

// The same reading reaches `unset`, which had been rejecting an expression for
// not being a numeral and then deleting a whole variable that was never there
// — so the array came back untouched with status 0.
func TestUnsettingAnElementByExpression(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(x y z); unset "a[1+1]"; echo "keys=${!a[@]}"`, "keys=0 1"},
		{`a=(x y z); i=1; unset "a[i+1]"; echo "keys=${!a[@]}"`, "keys=0 1"},
		{`a=(x y z); unset "a[k]"; echo "keys=${!a[@]}"`, "keys=1 2"},
		// The operand arrives unexpanded — it was in single quotes — and an
		// expression is substituted into before it is read.
		{`a=(x y z); i=1; unset 'a[$i]'; echo "keys=${!a[@]}"`, "keys=0 2"},
		// A key is still a key where the attribute says so, and never an
		// expression.
		{`typeset -A m; m[1+1]=v; unset "m[1+1]"; echo "n=${#m[@]}"`, "n=0"},
	} {
		out, st := runArray(t, c.src)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
		if st != 0 {
			t.Errorf("%s: status = %d, want 0", c.src, st)
		}
	}
}

// A substring's offset and length are expressions for the same reason, and
// taking the numeral alone gave an offset of 0 — a real substring of the right
// length, which is why nothing noticed.
func TestASubstringOffsetIsAnExpression(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`x=abcdef; echo "[${x:1+1:2}]"`, "[cd]"},
		{`x=abcdef; i=1; echo "[${x:i+1:2}]"`, "[cd]"},
		{`x=abcdef; echo "[${x:0:1+2}]"`, "[abc]"},
		{`a=(p q r s); echo "[${a[@]:1+1:2}]"`, "[r s]"},
	} {
		out, st := runArray(t, c.src)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
		if st != 0 {
			t.Errorf("%s: status = %d, want 0", c.src, st)
		}
	}
}

// A subscript written where the same expression was stored finds what was
// stored, which is the whole point of one reading rather than four.
func TestWritingAndReadingAgreeOnASubscript(t *testing.T) {
	const src = `a[1+1]=v; echo "[${a[1+1]}][${a[2]}]"`
	out, _ := runArray(t, src)
	if got := strings.TrimSpace(out); got != "[v][v]" {
		t.Errorf("%s = %q, want the element found by both spellings", src, got)
	}
}
