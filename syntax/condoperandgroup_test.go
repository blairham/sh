// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A group at the front of a condition's third word (#3040).
//
// `[[ 9 -gt ( 1 + 2 ) ]]` and `[[ -prefix 1 (f|ht)tp:// ]]` are the two
// shapes, and four of the completion functions one shell ships are written
// with them. The `(` belongs to the word, which is a *lexical* question:
// `(` is in the operator table and a token beginning with one never reaches
// the word scanner, so the parser has to say which position this is.
//
// Measured 2026-09-15, each probe in a script file of its own:
//
//	| probe                   | zsh 5.9.2 | bash 5.3 | bash-as-sh | bash 3.2 | ksh93u+ | dash | ash |
//	| `[[ 9 -gt ( 1 ) ]]`     | true      | refused  | refused    | refused  | refused | refused | refused |
//	| `[[ 2 -gt ( 1 + 2 ) ]]` | false     | refused  | refused    | refused  | refused | refused | refused |
//	| `[[ -pfx 1 (a\|b)c ]]`  | parsed    | refused  | refused    | refused  | refused | refused | refused |
//
// One column against six, so this is a dialect's grammar. The second row is
// what says the group reaches the *evaluator* rather than being dropped: the
// same probe at 2 and at 9 answers differently, so the words inside were
// read as `3`.
//
// **The position is the whole of the rule**, measured in the same run:
//
//	[[ ( 1 -gt 0 ) ]]      the condition's own grouping paren — the first word
//	[[ -n ( a ) ]]         parse error near `(` — the second
//	[[ -prefix ( a ) ]]    parse error near `(` — the second again
//	[[ -n x ( a ) ]]       parsed — the third, whatever the operator took
//	[[ -prefix 1 ( a ) ]]  parsed — the third
//	[[ -n x y ( a ) ]]     parse error near `(` — the fourth
//	[[ -prefix 1 2 ( a ) ]] parse error near `(` — the fourth
//
// A `!` or a connective starts the count over: `[[ ! -pfx 1 ( a ) ]]` and
// `[[ x == y || -pfx 1 ( a ) ]]` both parse.
//
// The rows below assert the *word* the operand came out as, not that the
// line parsed. A reading that took the `(` and stopped the word at the first
// blank would parse `[[ 9 -gt ( 1 ) ]]` and hand the evaluator a lone
// parenthesis.

func condOperandGroupGrammar(d *Dialect) {
	d.ConditionOperandMayOpenWithAGroup = true
	// A bare group is what the parentheses are, and a condition is where
	// they stand.
	d.PatternAlternation = true
	// One row is a named condition with two arguments, which this dialect
	// accepts and refuses when it runs.
	d.ConditionIsResolvedWhenItRuns = true
	// And the completion-context tests, so the rows written with `-prefix`
	// are refused for the position of the `(` rather than for the operator
	// not being one.
	d.CompletionConditions = true
}

// condRightOperand is the source text of the right-hand word of the first
// binary condition in src.
func condRightOperand(t *testing.T, src string, d Dialect) string {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	c, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*TestClause)
	if !ok {
		t.Fatalf("%s: first command is %T, want a test clause", src, f.Stmts[0].Expr.(*Pipeline).Cmds[0])
	}
	b, ok := c.Expr.(*CondBinary)
	if !ok {
		t.Fatalf("%s: the condition is %T, want a binary one", src, c.Expr)
	}
	return src[b.Y.Pos().Offset:b.Y.End().Offset]
}

func TestAConditionsThirdWordMayOpenWithAGroup(t *testing.T) {
	t.Parallel()
	d := Core()
	condOperandGroupGrammar(&d)
	for _, tc := range []struct{ name, src, want string }{
		{"a group with blanks in it", "[[ 9 -gt ( 1 ) ]]", "( 1 )"},
		{"an expression inside it", "[[ 9 -gt ( 1 + 2 ) ]]", "( 1 + 2 )"},
		{"no blanks inside it", "[[ 9 -gt (1 + 2) ]]", "(1 + 2)"},
		{"nested groups", "[[ 9 -gt (( 1 + 2 )) ]]", "(( 1 + 2 ))"},
		{"text behind the group", "[[ $x = (f|ht)tp ]]", "(f|ht)tp"},
		{
			// The control: an operand with no paren in it is untouched.
			"no group at all",
			"[[ 9 -gt 3 ]]",
			"3",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := condRightOperand(t, tc.src, d); got != tc.want {
				t.Errorf("%s: right operand %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestAConditionGroupOperandIsTheThirdWordOnly is the half that keeps the
// reading positional. The shell this is measured from refuses a `(` at the
// second word of a condition and at the fourth, and reads one at the first as
// the condition's own grouping paren.
func TestAConditionGroupOperandIsTheThirdWordOnly(t *testing.T) {
	t.Parallel()
	d := Core()
	condOperandGroupGrammar(&d)
	for _, src := range []string{
		// The second word.
		"[[ -n ( a ) ]]",
		"[[ -prefix ( a ) ]]",
		// And the fourth, which is where the reading has to stop again.
		"[[ -n x y ( a ) ]]",
		"[[ -prefix 1 2 ( a ) ]]",
	} {
		if _, err := Parse(src, d); err == nil {
			t.Errorf("%s parsed, and the shell this is measured from refuses it", src)
		}
	}
	// And the third word is taken whatever the operator did with it: a
	// one-operand test that already has its operand still reads a group
	// where the surplus word stands. `[[ -n x ( a ) ]]` is `unknown
	// condition: -n` at 2 on the shell this is measured from, which is a
	// refusal when it runs and so a line that parsed.
	for _, src := range []string{"[[ -n x ( a ) ]]", "[[ -prefix 1 ( a ) ]]"} {
		if _, err := Parse(src, d); err != nil {
			t.Errorf("%s: %v — the shell this is measured from parses it", src, err)
		}
	}
	// And the first word is the grouping paren, which is a different node
	// entirely and was already right.
	f, err := Parse("[[ ( 1 -gt 0 ) ]]", d)
	if err != nil {
		t.Fatalf("[[ ( 1 -gt 0 ) ]]: %v", err)
	}
	c := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*TestClause)
	if _, ok := c.Expr.(*CondGroup); !ok {
		t.Errorf("[[ ( 1 -gt 0 ) ]]: the condition is %T, want a group", c.Expr)
	}
}

// TestAConditionGroupOperandIsOneDialectsAndNotEveryShells is the panel's
// other side: six columns refuse the line, so the core grammar must too.
func TestAConditionGroupOperandIsOneDialectsAndNotEveryShells(t *testing.T) {
	t.Parallel()
	d := Core()
	// The group grammar without the positional flag, so the row that fails
	// is the flag's and not the absence of bare groups.
	d.PatternAlternation = true
	if _, err := Parse("[[ 9 -gt ( 1 ) ]]", d); err == nil {
		t.Error("`[[ 9 -gt ( 1 ) ]]` parsed without the flag that reads a group at a condition's third word")
	}
}
