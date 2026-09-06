// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// Most nodes carry their own Stop and a test that reads it is testing the
// parser. Three take their extent from a *child* instead — a function
// declaration and an anonymous function from their body, a `time` clause from
// its pipeline — and those three are the ones where End is a decision rather
// than a field, so they are the ones worth asserting.
//
// They were not asserted at all until #911, which is how #911 survived: the
// fix there gives [syntax.FuncDecl] an answer for the body it was refused, and
// a mutant replacing the whole method with `return c.Start` — throwing the
// extent of every *well-formed* declaration away — passed the suite. So did
// the same mutant on [syntax.AnonFunc], and so did inverting the nil check
// [syntax.TimeClause] has had all along. One line of one node had a test and
// the family had none.
//
// Both halves of each decision are here, because a guard is two answers and a
// test of one of them cannot tell a working guard from an inverted one.
func TestAnExtentTakenFromAChild(t *testing.T) {
	d := syntax.Core()
	d.AnonymousFunction = true
	d.TimeKeyword = true

	for _, tc := range []struct {
		src   string
		pos   syntax.Pos
		end   syntax.Pos
		about string
	}{
		{`f() { echo hi; }`, syntax.Pos{Offset: 0, Line: 1, Col: 1}, syntax.Pos{Offset: 16, Line: 1, Col: 17}, "a declaration ends where its body does"},
		{`() { echo hi; }`, syntax.Pos{Offset: 0, Line: 1, Col: 1}, syntax.Pos{Offset: 15, Line: 1, Col: 16}, "an anonymous function ends where its body does"},
		{`time echo hi`, syntax.Pos{Offset: 0, Line: 1, Col: 1}, syntax.Pos{Offset: 12, Line: 1, Col: 13}, "a time clause ends where its pipeline does"},
		{`time`, syntax.Pos{Offset: 0, Line: 1, Col: 1}, syntax.Pos{Offset: 4, Line: 1, Col: 5}, "a time clause reporting on nothing ends at the word"},
	} {
		f, err := syntax.Parse(tc.src, d)
		if err != nil {
			t.Errorf("%q: %v", tc.src, err)
			continue
		}
		if len(f.Stmts) != 1 {
			t.Errorf("%q: %d statements, want 1", tc.src, len(f.Stmts))
			continue
		}
		n := node(t, f.Stmts[0])
		if got := n.Pos(); got != tc.pos {
			t.Errorf("%q (%s): Pos() = %+v, want %+v", tc.src, tc.about, got, tc.pos)
		}
		if got := n.End(); got != tc.end {
			t.Errorf("%q (%s): End() = %+v, want %+v", tc.src, tc.about, got, tc.end)
		}
	}
}

// node is the one node under a statement, whichever of the three shapes it is.
// A `time` clause stands where the and-or does and the other two are a command
// inside a pipeline of one, so the unwrapping differs and the assertion does
// not.
func node(t *testing.T, st *syntax.Stmt) interface {
	Pos() syntax.Pos
	End() syntax.Pos
} {
	t.Helper()
	switch x := st.Expr.(type) {
	case *syntax.TimeClause:
		return x
	case *syntax.Pipeline:
		if len(x.Cmds) != 1 {
			t.Fatalf("pipeline of %d, want 1", len(x.Cmds))
		}
		return x.Cmds[0]
	default:
		t.Fatalf("statement is %T", st.Expr)
		return nil
	}
}
