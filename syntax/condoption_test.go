// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// `[[ -o name ]]` is one unary condition, and it is the core's.
//
// Core() rather than a flag, which is the claim the whole change turns on:
// every shell in the panel that has `[[ ]]` has the option test inside it,
// measured on bash 5.3, bash 3.2, bash-as-`sh`, ksh93 and zsh 5.9, and dash is
// absent from the row only because it has no `[[ ]]`. So there is no grammar
// for a dialect to switch — the disagreement is about which *names* exist,
// which is a question the parser never asks.
func TestTheOptionTestIsAUnaryCondition(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"a bare name", `[[ -o errexit ]]`},
		{"a quoted name, which is the prompt theme's spelling", `[[ -o 'aliases' ]]`},
		{"a name with an underscore", `[[ -o err_exit ]]`},
		{"a name out of a parameter", `[[ -o $v ]]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := onlyCommand(t, tc.src, syntax.Core())
			clause, ok := cmd.(*syntax.TestClause)
			if !ok {
				t.Fatalf("parse %q: got %T, want a *syntax.TestClause", tc.src, cmd)
			}
			un, ok := clause.Expr.(*syntax.CondUnary)
			if !ok {
				t.Fatalf("parse %q: condition is %T, want a *syntax.CondUnary", tc.src, clause.Expr)
			}
			if un.Op != "-o" {
				t.Errorf("parse %q: operator = %q, want %q", tc.src, un.Op, "-o")
			}
		})
	}
}

// The operand is an ordinary word, so the quotes are the word's and not the
// name's.
//
// The row that made this worth its own test is the prompt theme's, which
// writes `[[ ! -o 'aliases' ]]`: a parser that kept the apostrophes in the
// operand would look up a name spelled with them and never find it, and the
// failure would be a silent false rather than anything anybody could see.
func TestTheOptionTestsOperandKeepsItsQuoting(t *testing.T) {
	cmd := onlyCommand(t, `[[ ! -o 'aliases' ]]`, syntax.Core())
	clause, ok := cmd.(*syntax.TestClause)
	if !ok {
		t.Fatalf("got %T, want a *syntax.TestClause", cmd)
	}
	not, ok := clause.Expr.(*syntax.CondNot)
	if !ok {
		t.Fatalf("condition is %T, want a *syntax.CondNot", clause.Expr)
	}
	un, ok := not.X.(*syntax.CondUnary)
	if !ok {
		t.Fatalf("negated condition is %T, want a *syntax.CondUnary", not.X)
	}
	if !un.X.IsQuoted() {
		t.Error("the operand does not report itself quoted, so the name would be looked up with its apostrophes")
	}
	if got, want := un.X.Literal(), "aliases"; got != want {
		t.Errorf("operand = %q, want %q", got, want)
	}
}

// It combines the way every other condition does: negated, grouped, and joined
// by `&&` and `||` — which is the shape the plugin loader on this machine
// writes, and the one neither of the other two third-party files exercises.
func TestTheOptionTestCombinesLikeAnyOtherCondition(t *testing.T) {
	const src = `[[ ! -o functionargzero || ${options[posixargzero]} = on || ${ZI[ZERO]} != */* ]]`
	cmd := onlyCommand(t, src, syntax.Core())
	clause, ok := cmd.(*syntax.TestClause)
	if !ok {
		t.Fatalf("parse %q: got %T, want a *syntax.TestClause", src, cmd)
	}
	// `||` is left-associative, so the outer node's left side holds the first
	// two and the option test is under two `||` and a `!`.
	outer, ok := clause.Expr.(*syntax.CondLogic)
	if !ok || outer.Op != "||" {
		t.Fatalf("condition is %T, want the outer `||`", clause.Expr)
	}
	inner, ok := outer.X.(*syntax.CondLogic)
	if !ok || inner.Op != "||" {
		t.Fatalf("left side is %T, want the inner `||`", outer.X)
	}
	not, ok := inner.X.(*syntax.CondNot)
	if !ok {
		t.Fatalf("leftmost condition is %T, want a *syntax.CondNot", inner.X)
	}
	if un, ok := not.X.(*syntax.CondUnary); !ok || un.Op != "-o" {
		t.Fatalf("negated condition is %T, want the `-o` test", not.X)
	}
}

// A missing operand is refused rather than defaulted.
//
// Measured: bash and ksh93 make it a parse error and zsh takes the `-o` for a
// condition name it does not know, so no shell in the panel lets `[[ -o ]]`
// through. Refusing it here is what keeps the operator one that takes an
// operand — a parser that let the `]]` *be* the operand would answer a
// question about an option named `]]`.
func TestTheOptionTestNeedsAnOperand(t *testing.T) {
	if _, err := syntax.Parse(`[[ -o ]]`, syntax.Core()); err == nil {
		t.Fatal("parsed `[[ -o ]]`, want a refusal")
	}
}

// The printer writes it back, which is what says the node carries everything
// the source did.
func TestTheOptionTestPrintsBack(t *testing.T) {
	for _, src := range []string{
		`[[ -o errexit ]]`,
		`[[ ! -o errexit ]]`,
		`[[ -o errexit && -o nounset ]]`,
	} {
		cmd := onlyCommand(t, src, syntax.Core())
		if got := syntax.PrintCommand(cmd); got != src {
			t.Errorf("printed %q, want %q", got, src)
		}
	}
}
