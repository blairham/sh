// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A `for` loop may take a brace group where `do … done` stands, in either
// spelling of the header.
func TestAForTakesABraceBody(t *testing.T) {
	takes, refuses := syntax.POSIX(), syntax.POSIX()
	takes.CStyleFor, takes.ArithCommand, takes.ForBraceBody = true, true, true
	takes.Select = true
	// The refusing vector keeps the C-style header and drops only the body,
	// so what it proves is about this flag and not about the loop.
	refuses.CStyleFor, refuses.ArithCommand = true, true

	for _, src := range []string{
		`for ((;;)) { echo hi; break; }`,
		`for ((i=0;i<2;i=i+1)) { echo $i; }`,
		// A terminator between the header and the brace is optional, exactly
		// as it is before `do`.
		`for ((i=0;i<2;i=i+1)); { echo $i; }`,
		"for ((i=0;i<2;i=i+1))\n{ echo $i; }",
		// A newline body, and a nested one.
		"for ((i=0;i<2;i=i+1)) {\necho $i\n}",
		`for ((i=0;i<2;i=i+1)) { for ((j=0;j<2;j=j+1)) { echo $i$j; }; }`,
		// The list form takes it too, but only after a separator: with
		// nothing between, `{` is another item of the list.
		`for i in a b; { echo $i; }`,
		"for i in a b\n{ echo $i; }",
		`for i; { echo $i; }`,
		// And so does the menu loop, whose header is a for-loop's.
		`select x in a; { echo $x; }`,
		// The other spelling still parses, and so does a redirection after
		// the closing brace.
		`for ((;;)) do echo hi; break; done`,
		`for ((i=0;i<2;i=i+1)) { echo $i; } > /dev/null`,
	} {
		if _, err := syntax.Parse(src, takes); err != nil {
			t.Errorf("%q should parse: %v", src, err)
		}
	}

	// Without the flag the loop still parses and the brace body does not.
	for _, src := range []string{
		`for ((;;)) { echo hi; break; }`,
		`for i in a b; { echo $i; }`,
	} {
		if _, err := syntax.Parse(src, refuses); err == nil {
			t.Errorf("%q should not parse without the flag", src)
		}
	}
	if _, err := syntax.Parse(`for ((;;)) do echo hi; break; done`, refuses); err != nil {
		t.Errorf("the loop itself should still parse without the flag: %v", err)
	}
}

// The production belongs to the two loops that share a header and to nothing
// else, which is what makes it a production rather than a general rule about
// bodies — and what makes it easy to miss.
func TestABraceBodyIsOnlyTheForLoops(t *testing.T) {
	d := syntax.POSIX()
	d.CStyleFor, d.ArithCommand, d.Select, d.ForBraceBody = true, true, true, true

	for _, src := range []string{
		// A separator is required in the list form, so these read `{` as an
		// item and then meet `}` where `do` belongs.
		`for i in a b { echo $i; }`,
		`select x in a { echo $x; }`,
		// And the other compound commands do not take one at all.
		`while true { echo hi; break; }`,
		`while true; { echo hi; break; }`,
		`until false; { echo hi; break; }`,
		`if true; { echo yes; }`,
	} {
		if _, err := syntax.Parse(src, d); err == nil {
			t.Errorf("%q should not parse", src)
		}
	}
}

// The body is an ordinary brace group and its list is the loop's, so what
// runs is what would have run between `do` and `done`.
func TestABraceBodyIsTheLoopsList(t *testing.T) {
	d := syntax.POSIX()
	d.CStyleFor, d.ArithCommand, d.ForBraceBody = true, true, true

	f, err := syntax.Parse(`for ((i=0;i<2;i=i+1)) { echo one; echo two; }`, d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	pipe, ok := f.Stmts[0].Expr.(*syntax.Pipeline)
	if !ok {
		t.Fatalf("got %T, want a pipeline", f.Stmts[0].Expr)
	}
	c, ok := pipe.Cmds[0].(*syntax.ForArithClause)
	if !ok {
		t.Fatalf("got %T, want a C-style for", pipe.Cmds[0])
	}
	if n := len(c.Body); n != 2 {
		t.Errorf("the body holds %d statements, want 2", n)
	}
	// And the header is unaffected by which spelling the body used.
	if c.CondText != "i<2" {
		t.Errorf("condition is %q, want %q", c.CondText, "i<2")
	}
}

// An unterminated brace body is unterminated input rather than a token in the
// wrong place — the same report the group already makes for itself.
func TestAnUnterminatedBraceBody(t *testing.T) {
	d := syntax.POSIX()
	d.CStyleFor, d.ArithCommand, d.ForBraceBody = true, true, true

	f, err := syntax.Parse(`for ((;;)) { echo hi;`, d)
	if err == nil {
		t.Fatalf("should not parse, got %#v", f)
	}
	var e *syntax.Error
	if !errors.As(err, &e) {
		t.Fatalf("got %T, want a syntax error", err)
	}
	if e.Kind != syntax.ErrUnterminated {
		t.Errorf("kind is %v, want the input reported as having run out", e.Kind)
	}
	if e.Construct != "{" {
		t.Errorf("construct is %q, want the brace named", e.Construct)
	}
}
