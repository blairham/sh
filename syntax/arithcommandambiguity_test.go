// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// `((` at the start of a command is ambiguous, and the two readings are a
// whole program apart: an arithmetic command runs in the shell that wrote it,
// a subshell whose first command is a subshell runs two groupings.
//
// Every shell that has the construct resolves it the same way — the
// arithmetic reading holds only if the expression's own nesting is closed by
// two **adjacent** `)`, and is given up otherwise. Measured 2026-09-15 from a
// script file, and unanimous in bash 5.3.20, bash 3.2.57, bash-as-`sh`, zsh
// 5.9.2 and ksh93u+ 2012-08-01 (#3052).
//
// Asserted on the node kind rather than on an error, because a refusal is not
// the failure this had: `((echo a); echo b)` was an *arithmetic* command over
// the expression `echo a); echo b`, and the diagnostic quoted the script's own
// text back as one. What is wrong is which construct was built.
func TestTheArithmeticReadingOfTwoParensIsGivenUpWhereItDoesNotClose(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		src   string
		arith bool   // the reading held
		expr  string // and this is the expression it holds
	}{
		// The reading holds: the two closers are adjacent, at the depth the
		// expression's own parentheses left.
		{"an expression", "((1+1))", true, "1+1"},
		{"an assignment", "((a=5))", true, "a=5"},
		{"with blanks inside", "(( a = 1 ))", true, " a = 1 "},
		{"an expression that opens with a paren", "(( (1+2)*3 ))", true, " (1+2)*3 "},
		{"parentheses and nothing else", "(( ((2)) ))", true, " ((2)) "},
		{"no blanks around a parenthesised one", "(((a=5)))", true, "(a=5)"},
		{"one deeper again", "((((a=5))))", true, "((a=5))"},
		{"an empty expression", "(())", true, ""},
		// A bad expression is still an arithmetic command: it closed, so
		// what is wrong with it is the evaluator's to say at run time. This
		// is the row that says the fix did not simply widen the fallback
		// until nothing was arithmetic any more.
		{"a bad expression that closed", "((echo a))", true, "echo a"},

		// The reading is given up: a `)` arrives where the expression's own
		// nesting is already closed and the next character is not a `)`.
		{"a command list behind the first group", "((echo a); echo b)", false, ""},
		{"two commands in the inner group", "((echo a; echo b); echo c)", false, ""},
		{"a blank between the closers", "((echo a) )", false, ""},
		{"a blank after the openers too", "(( echo a ); echo b)", false, ""},
		{"an and-or between the groups", "((echo a)&&(echo b))", false, ""},
		{"a pipeline between them", "((echo a)|(cat))", false, ""},
		{"a separator and nothing after it", "((echo a);)", false, ""},
		{"three groups", "(((echo a); echo b); echo c)", false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := onlyCommand(t, tc.src, syntax.Core())
			if !tc.arith {
				sub, ok := cmd.(*syntax.Subshell)
				if !ok {
					t.Fatalf("%q is %T, want a subshell", tc.src, cmd)
				}
				if len(sub.List) == 0 {
					t.Errorf("%q: the subshell holds nothing", tc.src)
				}
				return
			}
			arith, ok := cmd.(*syntax.ArithCmdClause)
			if !ok {
				t.Fatalf("%q is %T, want an arithmetic command", tc.src, cmd)
			}
			if arith.Expr != tc.expr {
				t.Errorf("%q holds the expression %q, want %q", tc.src, arith.Expr, tc.expr)
			}
		})
	}
}

// Where the line stands changes nothing: the ambiguity is the lexer's and it
// is resolved from the text alone, so the same characters go the same way in
// every position a command may begin.
func TestTheGivenUpReadingHoldsWhereverACommandMayBegin(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"((echo a); echo b) | cat",
		"{ ((echo a); echo b); }",
		"f() { ((echo a); echo b); }",
		"if true; then ((echo a); echo b); fi",
		"while false; do ((echo a); echo b); done",
		"((echo a); echo b) > /dev/null",
	} {
		mustParseHere(t, src, syntax.Core(), "two groupings, wherever they are written")
	}
}

// The reading is given up one character at a time, not two: the `(` becomes an
// ordinary grouping paren and the *next* token is read from the second one,
// which may open an arithmetic command of its own.
//
// `(((a=5)); echo $a)` is what says so. bash 5.3.20, bash 3.2.57 and zsh
// 5.9.2 all print 5 inside and leave the outer shell's copy alone; ksh93u+
// alone prints 0, having split the `((` into two subshell opens and never
// looked again. Four of five references take the reading here, so this
// follows them.
func TestAGivenUpReadingIsReReadFromTheSecondParen(t *testing.T) {
	t.Parallel()
	cmd := onlyCommand(t, "(((a=5)); echo $a)", syntax.Core())
	sub, ok := cmd.(*syntax.Subshell)
	if !ok {
		t.Fatalf("not a subshell but %T", cmd)
	}
	if len(sub.List) != 2 {
		t.Fatalf("the subshell holds %d statements, want 2", len(sub.List))
	}
	pipe, ok := sub.List[0].Expr.(*syntax.Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		t.Fatalf("the first statement is not one command")
	}
	arith, ok := pipe.Cmds[0].(*syntax.ArithCmdClause)
	if !ok {
		t.Fatalf("the first command is %T, want an arithmetic command", pipe.Cmds[0])
	}
	if arith.Expr != "a=5" {
		t.Errorf("the expression is %q, want %q", arith.Expr, "a=5")
	}
}

// Giving the reading up is not the same as running out of input. Every shell
// on the panel refuses `((1+1` and `((echo a` alike, so an expression that
// ends with the file stays a refusal — and at a prompt, still a request for
// the rest of it rather than a subshell nobody opened.
func TestAnExpressionThatRunsOutIsStillRefused(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"((1+1",
		"((echo a",
		"((echo a); echo b",
	} {
		mustFailHere(t, src, syntax.Core(), "input that ended inside the construct")
	}
}

// Where no command may begin the question never arises, and the positions
// that already suspended the arithmetic reading still do: a `case` arm's
// paren, a condition's grouping parens and an expansion's operand.
func TestThePositionsThatSuspendTheReadingStillDo(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.PatternAlternation = true
	d.GlobQualifiers = true
	for _, src := range []string{
		"case x in ((a|b)) :;; esac",
		"[[ ((1 == 1)) ]]",
		"u=; echo ${u:-((a))}",
	} {
		mustParseHere(t, src, d, "two parens where no command may begin")
	}
}
