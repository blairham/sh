// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// Dialect.ConditionArityIsCheckedWhenItRuns, both answers.
//
// The flag decides whether a *known* conditional operator standing with the
// wrong number of operands is a parse failure or a tree the interpreter
// refuses. Every row is written twice for that reason: the point is not what
// the sentence says, it is whether there is a sentence here at all.
func TestAConditionsArityMayBeLeftForTheInterpreter(t *testing.T) {
	t.Parallel()
	arity := Core()
	arity.ConditionArityIsCheckedWhenItRuns = true
	for _, tc := range []struct {
		name, src string
		// op is the operator the accepted tree names, and words how many
		// stood with it. Empty op means the line is a parse failure under
		// both answers.
		op    string
		words int
	}{
		{"a surplus operand", `[[ -n x y ]]`, "-n", 2},
		{"two surplus operands", `[[ -n x -z "" ]]`, "-n", 3},
		{"no operand at all", `[[ -n ]]`, "-n", 0},
		{"another operator", `[[ -o ]]`, "-o", 0},
		{"three surplus operands", `[[ -n x y z ]]`, "-n", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// With the flag on, the condition parses and the tree says which
			// operator had the wrong arity.
			f, err := Parse(tc.src, arity)
			if err != nil {
				t.Fatalf("with the flag on, parse %q: %v", tc.src, err)
			}
			got := testClauseOf(t, f)
			x, isArity := got.(*CondArity)
			if !isArity {
				t.Fatalf("with the flag on, %q parsed to %T, want *CondArity", tc.src, got)
			}
			if x.Op != tc.op {
				t.Errorf("the tree names %q, want %q", x.Op, tc.op)
			}
			if len(x.Words) != tc.words {
				t.Errorf("the tree kept %d words, want %d", len(x.Words), tc.words)
			}
			// And with it off, the same text is refused while reading, which
			// is what every other column in the panel does.
			if _, err := Parse(tc.src, Core()); err == nil {
				t.Errorf("with the flag off, %q parsed — it is a syntax error everywhere else", tc.src)
			}
		})
	}
}

// The rows the flag does **not** reach, which are what keep it from being
// "any word after the operand".
func TestAConditionsArityStopsWhereItWasMeasuredTo(t *testing.T) {
	t.Parallel()
	d := Core()
	d.ConditionArityIsCheckedWhenItRuns = true
	for _, tc := range []struct {
		name, src string
		refused   bool
	}{
		// An operator-shaped operand starts a reading of its own, so the
		// word after it is unexpected rather than surplus. Measured on the
		// shell this flag is for: `[[ -n -z x ]]` is a parse error naming
		// `x`, where `[[ -n x y ]]` is the run-time refusal.
		{"an operator-shaped operand leaves the next word unexpected", `[[ -n -z x ]]`, true},
		// And with nothing after it, that operand is an ordinary word.
		{"an operator-shaped operand on its own is an operand", `[[ -n -n ]]`, false},
		// A word that is no operator at all is a bare-word test, whatever
		// stands after it — the flag is about a *known* operator's arity.
		{"a word that is not an operator", `[[ -bogus ]]`, false},
		{"the ordinary arity", `[[ -n x ]]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.src, d)
			if tc.refused != (err != nil) {
				t.Errorf("parse %q: err = %v, want refused = %v", tc.src, err, tc.refused)
			}
		})
	}
}

// testClauseOf digs the condition out of a one-statement file.
func testClauseOf(t *testing.T, f *File) CondExpr {
	t.Helper()
	if len(f.Stmts) != 1 {
		t.Fatalf("%d statements, want 1", len(f.Stmts))
	}
	tc, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*TestClause)
	if !ok {
		t.Fatalf("%q did not parse to a test clause", f.Stmts[0].Expr)
	}
	return tc.Expr
}
