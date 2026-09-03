// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// declaring is a dialect with array literals and the five utilities that take
// one as an operand.
func declaring() syntax.Dialect {
	d := syntax.Core()
	d.DeclarationUtilities = map[string]bool{
		"declare": true, "typeset": true, "local": true,
		"export": true, "readonly": true,
	}
	return d
}

// An array assignment written after a declaration utility is an operand of it,
// not a prefix to it — and it used to be a syntax error, which is how
// `local -a x=()` in three installed bats-core files failed to parse.
func TestAnArrayAssignmentMayBeAnOperand(t *testing.T) {
	for _, c := range []struct {
		name    string
		src     string
		assigns int
		args    int
	}{
		{"local with a value", "local a=(x y)", 1, 1},
		{"an empty array", "local -a a=()", 1, 2},
		{"two of them", "local a=(x) b=(y)", 2, 1},
		{"an option between", "declare -a a=(x) -i b=(y)", 2, 3},
		{"every utility takes one", "readonly a=(x)", 1, 1},
		// A scalar stays an ordinary word: it has expansion rules of its own
		// and this changes nothing about them.
		{"a scalar is still a word", "local a=1", 0, 2},
		// And a bare `name=` is a word too, not an empty array.
		{"a bare name= is a word", "local a=", 0, 2},
		{"a prefix is still a prefix", "a=1 local b=(x)", 2, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := syntax.NewParser(c.src, declaring())
			f := p.Parse()
			if err := p.Err(); err != nil {
				t.Fatalf("parse %q: %v", c.src, err)
			}
			cmd := onlySimple(t, f)
			if len(cmd.Assigns) != c.assigns {
				t.Errorf("%d assignments, want %d", len(cmd.Assigns), c.assigns)
			}
			if len(cmd.Args) != c.args {
				t.Errorf("%d words, want %d", len(cmd.Args), c.args)
			}
		})
	}
}

// The name is what tells an operand from a syntax error: `echo a=(x)` is not a
// declaration and does not parse, in bash and ksh93 alike.
func TestOnlyADeclarationUtilityTakesOne(t *testing.T) {
	for _, src := range []string{"echo a=(x)", "true a=(x)", "nosuchcmd a=(x)"} {
		p := syntax.NewParser(src, declaring())
		p.Parse()
		if p.Err() == nil {
			t.Errorf("%q parsed, want a syntax error", src)
		}
	}
	// And a dialect with no such utilities refuses even `local`, which is
	// what ksh93 does with a name it does not have.
	d := syntax.Core()
	d.DeclarationUtilities = nil
	p := syntax.NewParser("local a=(x)", d)
	p.Parse()
	if p.Err() == nil {
		t.Error("parsed with no declaration utilities, want a syntax error")
	}
}

// A prefix assignment and an operand are different things wearing one syntax,
// and the tree says which is which.
func TestAnOperandIsMarkedApartFromAPrefix(t *testing.T) {
	p := syntax.NewParser("a=1 local b=(x)", declaring())
	f := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatal(err)
	}
	cmd := onlySimple(t, f)
	if len(cmd.Assigns) != 2 {
		t.Fatalf("%d assignments, want 2", len(cmd.Assigns))
	}
	if cmd.Assigns[0].Operand {
		t.Error("the prefix is marked as an operand")
	}
	if !cmd.Assigns[1].Operand {
		t.Error("the operand is not marked as one")
	}
}

func onlySimple(t *testing.T, f *syntax.File) *syntax.SimpleCmd {
	t.Helper()
	if len(f.Stmts) != 1 {
		t.Fatalf("%d statements, want 1", len(f.Stmts))
	}
	pipe, ok := f.Stmts[0].Expr.(*syntax.Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		t.Fatalf("got %T, want a one-command pipeline", f.Stmts[0].Expr)
	}
	cmd, ok := pipe.Cmds[0].(*syntax.SimpleCmd)
	if !ok {
		t.Fatalf("got %T, want a simple command", pipe.Cmds[0])
	}
	return cmd
}
