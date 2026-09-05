// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// Two parentheses that touch are the arithmetic command wherever a command
// may begin, and two grouping parentheses inside `[[ ]]`, where none may.
//
// Measured rather than assumed: bash 3.2.57, bash 5.3.15, bash-as-sh, ksh93
// and zsh 5.9.2 all take `[[ ((1 -eq 1)) ]]` and give it the same status as
// the spaced `[[ ( (1 -eq 1) ) ]]`. dash has no `[[ ]]` and abstains. The
// intersection is unanimous, so this is core rather than a dialect flag
// (#859).
func TestCondTouchingParensAreGroups(t *testing.T) {
	tests := []struct{ src, want string }{
		{`[[ ((1 -eq 1)) ]]`, `[[(1 -eq 1)]]`},
		{`[[ ( (1 -eq 1) ) ]]`, `[[(1 -eq 1)]]`},
		{`[[ (((1 -eq 1))) ]]`, `[[[(1 -eq 1)]]]`},
		{`[[ (( -z x )) ]]`, `[[(-z x)]]`},
		{`[[ ((-z x)) ]]`, `[[(-z x)]]`},
		{`[[ 1 -eq 1 && ((2 -eq 2)) ]]`, `((1 -eq 1) && [[(2 -eq 2)]])`},
		{`[[ 1 -eq 2 || ((2 -eq 2)) ]]`, `((1 -eq 2) || [[(2 -eq 2)]])`},
		{`[[ ! ((1 -eq 2)) ]]`, `(! [[(1 -eq 2)]])`},
		{
			`[[ (1 -eq 1) || ((2 -eq 2) && (3 -eq 3)) ]]`,
			`([(1 -eq 1)] || [([(2 -eq 2)] && [(3 -eq 3)])])`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			if got := cond(parseCond(t, tc.src)); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// The spaced and unspaced forms are the *same* tree, which is the property
// the bug broke: a space made the condition parse and its absence did not.
func TestCondSpacingGroupingParensChangesNothing(t *testing.T) {
	spaced := cond(parseCond(t, `[[ ( (1 -eq 1) ) ]]`))
	touching := cond(parseCond(t, `[[ ((1 -eq 1)) ]]`))
	if spaced != touching {
		t.Errorf("`[[ ( (1 -eq 1) ) ]]` is %s and `[[ ((1 -eq 1)) ]]` is %s; they are the same condition", spaced, touching)
	}
}

// The suspension is the condition's and nobody else's. One character past
// `]]` the arithmetic command is back, and an expression of its own that
// opens with a parenthesis was never in doubt.
func TestArithCommandOutsideAConditionIsUnaffected(t *testing.T) {
	tests := []struct{ src, want string }{
		{`(( (1+2)*3 ))`, ` (1+2)*3 `},
		{`(( x++ ))`, ` x++ `},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			f, err := Parse(tc.src, Core())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			c, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*ArithCmdClause)
			if !ok {
				t.Fatalf("%q parsed to %T, want an arithmetic command", tc.src, f.Stmts[0].Expr.(*Pipeline).Cmds[0])
			}
			if c.Expr != tc.want {
				t.Errorf("expression is %q, want %q", c.Expr, tc.want)
			}
		})
	}
}

// The flag is cleared when the condition ends rather than leaking into what
// follows it — the regression a lexer mode invites.
func TestArithCommandAfterATestClause(t *testing.T) {
	f, err := Parse(`[[ 1 -eq 1 ]] && (( x++ ))`, Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	list, ok := f.Stmts[0].Expr.(*BinaryExpr)
	if !ok {
		t.Fatalf("parsed to %T, want an and-or list", f.Stmts[0].Expr)
	}
	right := list.Y.(*Pipeline).Cmds[0]
	if _, ok := right.(*ArithCmdClause); !ok {
		t.Fatalf("right-hand side is %T, want an arithmetic command", right)
	}
}

// And a subshell that opens with a subshell still needs its space, which is
// the rule the condition is the exception to.
func TestTouchingParensAtCommandPositionAreArithmetic(t *testing.T) {
	// Read as arithmetic, `echo hi` is not an expression — which is the
	// proof that the two parentheses were one token. bash, ksh93 and zsh all
	// report the same thing, a beat later, when the command runs.
	_, err := Parse(`((echo hi))`, Core())
	if err == nil {
		t.Fatal("`((echo hi))` parsed; it is an arithmetic command over `echo hi`")
	}
	if got, want := err.Error(), `1:1: arithmetic expression: echo hi`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	f, err := Parse(`( (echo hi) )`, Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	outer, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*Subshell)
	if !ok {
		t.Fatalf("`( (echo hi) )` parsed to %T, want a subshell", f.Stmts[0].Expr.(*Pipeline).Cmds[0])
	}
	inner := outer.List[0].Expr.(*Pipeline).Cmds[0]
	if _, ok := inner.(*Subshell); !ok {
		t.Fatalf("the subshell holds %T, want another subshell", inner)
	}
}

// An unbalanced group is still an error, and it is reported where it happened
// rather than swallowed by the parentheses now being separate tokens. The
// whole rendered line is asserted, location included: a diagnostic that moves
// to the wrong position is the failure this catches.
func TestCondUnbalancedTouchingParens(t *testing.T) {
	tests := []struct{ src, want string }{
		{`[[ ((1 -eq 1) ]]`, `1:15: expected ) in a condition`},
		{`[[ (()) ]]`, `1:6: expected a condition after (`},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			_, err := Parse(tc.src, Core())
			if err == nil {
				t.Fatalf("parse %q: no error", tc.src)
			}
			if got := err.Error(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
