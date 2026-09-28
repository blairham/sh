// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A declaration's **array-literal operand** is printed where it was written,
// not after every other word (#5096).
//
// The parser puts it in `SimpleCmd.Assigns` and the words around it in
// `SimpleCmd.Args`, so a printer that walks the two lists in turn writes a
// permutation of the command. `let a=(5+3)x y` came back as
// `let x y a=(5+3)`, which is a different program — and it went unnoticed
// because the *runtime* was permuting the same list the same way, so the two
// wrongs agreed with each other. This is the third surface of one record; the
// other two are in interp/operandwrittenorder.go.
//
// The case that caught it — `let a=(5+3)x y`, from the corpus — needs a
// dialect that gives `let` an array literal, so it is graded where it was
// found: interp's TestPrintedSourceStillMeansTheSameThing runs the printed
// source and compares what it does, and that is the row that failed the
// moment the runtime stopped permuting.
func TestAnArrayOperandPrintsWhereItWasWritten(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{"one operand before a word", "typeset a=(x y) b\n", "typeset a=(x y) b"},
		{"and after one", "typeset b a=(x y)\n", "typeset b a=(x y)"},
		{"between two", "typeset b a=(1) c\n", "typeset b a=(1) c"},
		{"two operands among words", "typeset x a=(1) y b=(2) z\n", "typeset x a=(1) y b=(2) z"},
		{"two operands together", "typeset a=(1) b=(2)\n", "typeset a=(1) b=(2)"},
		{"the other order", "typeset b=(2) a=(1)\n", "typeset b=(2) a=(1)"},
		// A prefix assignment is not an operand and keeps its place in front
		// of the command word, which is the control that says this change is
		// about the operands and not about assignments in general.
		{"a prefix stays in front", "v=1 typeset a=(2) b\n", "v=1 typeset a=(2) b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := syntax.Print(f); got != tc.want {
				t.Errorf("printed %q, want %q", got, tc.want)
			}
		})
	}
}

// A compound variable's body is printed by the same kind of loop and is
// deliberately left alone: its items cannot hold a bare word at all, so there
// is nothing to interleave. `typeset -C c=( x=1 w y=2 )` is `"w" unexpected`
// to this parser, measured 2026-09-28, and a shape the grammar refuses is not
// a shape the printer has to order.
func TestACompoundBodyTakesNoBareWordToOrderAgainst(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.CompoundVariableDeclarators = map[string]bool{"typeset": true}
	// The control: with the declarators in place, a body of assignments is
	// read. Without this row the two below would pass against a dialect that
	// has no compound variables at all, where the refusal is about something
	// else entirely.
	if _, err := syntax.Parse("typeset -C c=( x=1 y=2 )\n", d); err != nil {
		t.Fatalf("a body of assignments does not parse: %v", err)
	}
	for _, src := range []string{
		"typeset -C c=( x=1 w y=2 )\n",
		"typeset -C c=( w x=1 )\n",
	} {
		if _, err := syntax.Parse(src, d); err == nil {
			t.Errorf("%q parsed, want the bare word refused", src)
		}
	}
}
