// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A parenthesized element inside an array literal — `a=( (1 2) (3 4) )`, which
// is one shell's multi-dimensional array and a syntax error everywhere else.
//
// The element is a literal of its own rather than a word, which is why
// Assign.Elems holds an ArrayElem: a parallel list beside the words would have
// compiled everywhere and been silently wrong wherever a reader forgot it.

func nestedLiteralDialect() syntax.Dialect {
	d := syntax.Core()
	d.ArrayLiteral = true
	d.NestedArrayLiteral = true
	d.ArrayLiteralShapeFollowsTheFirstElement = true
	d.SubscriptSpansSeparators = true
	return d
}

func parsedAssign(t *testing.T, src string, d syntax.Dialect) *syntax.Assign {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	cmd, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd)
	if !ok || len(cmd.Assigns) == 0 {
		t.Fatalf("parse %q: no assignment", src)
	}
	return cmd.Assigns[0]
}

// shape renders an element list as `w` for a word and `(…)` for a nested
// literal, so a row says what the tree *is* rather than what it prints.
func shape(elems []*syntax.ArrayElem) string {
	var b strings.Builder
	for i, e := range elems {
		if i > 0 {
			b.WriteString(" ")
		}
		switch {
		case e.Word != nil && e.Nested != nil:
			b.WriteString(syntax.PrintWord(e.Word) + "(" + shape(e.Nested.Elems) + ")")
		case e.Word != nil:
			b.WriteString("w:" + syntax.PrintWord(e.Word))
		default:
			b.WriteString("(" + shape(e.Nested.Elems) + ")")
		}
	}
	return b.String()
}

func TestANestedLiteralIsAnElementOfItsOwn(t *testing.T) {
	d := nestedLiteralDialect()
	for _, tc := range []struct{ why, src, want string }{
		{"two of them", `a=( (1 2) (3 4) )`, `(w:1 w:2) (w:3 w:4)`},
		{"one, and an empty one", `a=( (1 2) () )`, `(w:1 w:2) ()`},
		{"words and literals interleave", `a=( x (1 2) y )`, `w:x (w:1 w:2) w:y`},
		{"it nests as far as it is written", `a=( ( (1 2) (3) ) (4) )`, `((w:1 w:2) (w:3)) (w:4)`},
		{"the close paren ends the element wherever it falls", `a=( (1 2)x )`, `(w:1 w:2) w:x`},
		{"under a key, written against the head", `a=( [0]=(1 2) )`, `[0]=(w:1 w:2)`},
		{"under a key, with a blank between", `a=( [0]= (1 2) )`, `[0]=(w:1 w:2)`},
		{"under a key, appending", `a=( [0]+= (1 2) )`, `[0]+=(w:1 w:2)`},
		{"and it is the head just read", `a=( [0]= [1]= (1 2) )`, `w:[0]= [1]=(w:1 w:2)`},
		// The controls: quoting makes the parentheses text, and a literal
		// with nothing nested in it is a list of words as it always was.
		{"quoted, it is a word", `a=( "(1 2)" )`, `w:"(1 2)"`},
		{"a plain literal", `a=( x y )`, `w:x w:y`},
		{"a subscripted one", `a=( [2]=c [0]=a )`, `w:[2]=c w:[0]=a`},
	} {
		t.Run(tc.why, func(t *testing.T) {
			if got := shape(parsedAssign(t, tc.src, d).Elems); got != tc.want {
				t.Errorf("%s came to %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// Where the dialect does not have it, the parenthesis is what it always was.
func TestANestedLiteralNeedsTheDialect(t *testing.T) {
	d := nestedLiteralDialect()
	d.NestedArrayLiteral = false
	if _, err := syntax.Parse("a=( (1 2) )\n", d); err == nil {
		t.Errorf("a=( (1 2) ) parsed with the grammar off")
	}
}

// In the **keyed** shape a parenthesis is a value and nothing else, which is
// measured: ksh93u+ 2012-08-01 answers “ `(' unexpected “ to both rows.
func TestANestedLiteralIsRefusedWhereItIsNotAValue(t *testing.T) {
	d := nestedLiteralDialect()
	for _, src := range []string{
		"a=( [0]=x (1 2) )\n",
		"a=( [0]= (1 2) (3 4) )\n",
	} {
		if _, err := syntax.Parse(src, d); err == nil {
			t.Errorf("%s parsed, where the shape refuses the parenthesis", src)
		}
	}
}

// Written back as it was read, so a formatter does not lose a dimension.
func TestANestedLiteralPrintsBack(t *testing.T) {
	d := nestedLiteralDialect()
	for _, tc := range []struct{ src, want string }{
		{"a=( (1 2) (3 4) )\n", "a=( (1 2) (3 4))"},
		{"a=( x (1 2) y )\n", "a=(x (1 2) y)"},
		{"a=( ( (1 2) (3) ) (4) )\n", "a=( ( (1 2) (3)) (4))"},
		{"a=( () )\n", "a=( ())"},
		{"a=( [0]=(1 2) )\n", "a=([0]=(1 2))"},
		{"a=( [0]= (1 2) )\n", "a=([0]=(1 2))"},
	} {
		f, err := syntax.Parse(tc.src, d)
		if err != nil {
			t.Errorf("parse %q: %v", tc.src, err)
			continue
		}
		got := syntax.Print(f)
		if got != tc.want {
			t.Errorf("%q printed back as %q, want %q", tc.src, got, tc.want)
			continue
		}
		// And what it printed parses to the same tree, which is the half a
		// golden string cannot check.
		if back, want := shape(parsedAssign(t, got, d).Elems),
			shape(parsedAssign(t, tc.src, d).Elems); back != want {
			t.Errorf("%q round-tripped to %q, want %q", tc.src, back, want)
		}
	}
}
