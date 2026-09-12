// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// An alias body is a piece of program and the grammar has to read it as one.
// Two questions in the grammar are answered from the *input's* offsets, and a
// spliced token has none of its own: every one carries the position of the
// word it replaced, so both answers came out wrong for a body that was
// perfectly ordinary shell.

// aliasBody parses src with one alias defined, which is the only way to reach
// a spliced token from here.
func aliasBody(t *testing.T, name, body, src string) (*syntax.File, error) {
	t.Helper()
	p := syntax.NewParser(src, syntax.Core())
	p.Aliases = table(name, body)
	f := p.Parse()
	return f, p.Err()
}

// `a=(x y)` is an array and `a= (x y)` is not, and the parenthesis touching
// the `=` is the whole of the difference. Written in an alias body the two
// tokens carry the same position, so an offset comparison says they do not
// touch and the assignment was refused outright.
func TestAnAliasBodyMayBeACompoundAssignment(t *testing.T) {
	f, err := aliasBody(t, "f", "a=(x y)", "f")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cmd := onlyCommandOf(t, f)
	simple, ok := cmd.(*syntax.SimpleCmd)
	if !ok || len(simple.Assigns) != 1 {
		t.Fatalf("parsed to %T with %d assignments, want one simple command with one", cmd, len(simple.Assigns))
	}
	a := simple.Assigns[0]
	if !a.IsArray {
		t.Fatalf("the assignment is not an array: the `(` was not read as the literal's")
	}
	if len(a.Elems) != 2 {
		t.Errorf("got %d elements, want 2", len(a.Elems))
	}
}

// And the separated spelling has to stay separated: the body says where its
// own blanks are, so a body written with one must not gain an array.
//
// The discriminating half of the pair. Answering "they touch" for every
// spliced token passes the case above and fails this one, which is what makes
// the two together a test of the body's layout rather than of the splice.
func TestABlankInAnAliasBodyStillPartsTheAssignmentFromTheParen(t *testing.T) {
	if _, err := aliasBody(t, "f", "a= (x y)", "f"); err == nil {
		t.Fatal("`a= (x y)` in an alias body was accepted: the body's blank was lost")
	}
	// And the same text read straight from the input, so the row above is
	// about the splice and not about the construct.
	if _, err := syntax.Parse("a= (x y)", syntax.Core()); err == nil {
		t.Fatal("`a= (x y)` read from the input was accepted")
	}
}

// The loop variable is compared against the input text to see whether it was
// written plainly, and for a spliced token that text is the alias's own name.
// So every loop in an alias body was refused for having a name it did not
// have.
func TestAnAliasBodyMayBeALoop(t *testing.T) {
	f, err := aliasBody(t, "f", "for i in 1 2; do echo hi; done", "f")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cmd := onlyCommandOf(t, f)
	loop, ok := cmd.(*syntax.ForClause)
	if !ok {
		t.Fatalf("parsed to %T, want a for loop", cmd)
	}
	if len(loop.Names) != 1 || loop.Names[0] != "i" {
		t.Errorf("loop variables %q, want [i]", loop.Names)
	}
}

// onlyCommandOf is onlyCommand's half that takes a file, for the cases above
// that have to parse with aliases rather than from a source string.
func onlyCommandOf(t *testing.T, f *syntax.File) syntax.Command {
	t.Helper()
	if len(f.Stmts) != 1 {
		t.Fatalf("got %d statements, want 1", len(f.Stmts))
	}
	pipe, ok := f.Stmts[0].Expr.(*syntax.Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		t.Fatalf("not one command")
	}
	return pipe.Cmds[0]
}
