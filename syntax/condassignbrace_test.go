// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A `(` straight after `=` is an array literal where an assignment can stand
// and a pattern group where one cannot — #1585.
//
// The guard exists for `a=(b c)`, which every column reads as an array literal
// rather than as a word matching a pattern, and it was written without asking
// where the `=` was. Inside `[[ ]]` there is no assignment for it to win
// against, so the `=` there is an ordinary character of the pattern.
//
// Measured 2026-09-08: `[[ a=b == a=(b) ]]` and `[[ a=b == a=(b|c) ]]` are both
// 0 in zsh 5.9.2, where bash 5.3.15 answers `syntax error in conditional
// expression: unexpected token `('` for the pair. This shell gave bash's answer
// under every dialect, which is what stopped powerlevel10k parsing.
func TestAParenAfterEqualsIsAGroupInAConditionAndALiteralOutsideOne(t *testing.T) {
	d := Core()
	d.PatternAlternation = true

	// The condition: the group is a group, alternation and all.
	for _, src := range []string{
		`[[ a=b == a=(b) ]]`,
		`[[ a=b == a=(b|c) ]]`,
		`[[ $x == pre[$' \t']#=([^z]#)post ]]`,
	} {
		if _, err := Parse(src, d); err != nil {
			t.Errorf("Parse(%q) = %v, want it to parse", src, err)
		}
	}

	// The assignment, which is the case the guard was written for: still an
	// array literal, and still two elements rather than one pattern word.
	f, err := Parse(`a=(b c)`, d)
	if err != nil {
		t.Fatalf("Parse of the array literal = %v", err)
	}
	if got := len(f.Stmts); got != 1 {
		t.Fatalf("array literal parsed to %d statements, want 1", got)
	}
}

// A bare `{` inside a double-quoted expansion is an ordinary character, and
// only the `{` a `$` brought with it opens a level — #1586.
//
// Counting every brace made an escaped dollar open a level nothing could
// close: `\$` is consumed as an escape pair, which left its `{` to be read as
// a nested expansion, and the double quote around the whole word was then
// eaten looking for the `}` that would balance it. The failure is reported
// against the quote, which is why it reads as an unterminated string rather
// than as a brace that was miscounted.
//
// Measured 2026-09-08 and unanimous in both columns that have the construct:
// `a=x; echo "${a/x/\${y}"` is `${y` in zsh 5.9.2 and bash 5.3.15 alike, and
// `"${a/x/{y}"` is `{y` in both.
func TestABareBraceInAQuotedExpansionDoesNotNest(t *testing.T) {
	for _, src := range []string{
		`echo "${a/x/\${y}"`,
		`echo "${a/x/\${y\}}"`,
		`echo "${a/x/{y}"`,
		// The nesting that must survive: this one's brace came with a `$`.
		`echo "${a:-${b:-z}}"`,
		// And the p10k line the pair of them was found in.
		`x+="${(@)${(@o)p[(I)P_*]}:/(#m)*/\${${(q)MATCH}-$IFS\}}"`,
	} {
		if _, err := Parse(src, Core()); err != nil {
			t.Errorf("Parse(%q) = %v, want it to parse", src, err)
		}
	}
}

// Unquoted, the same brace *does* have to balance, because there it is a
// brace-expansion group and the group is what says how far the operand
// reaches.
//
// The quoting is the whole of the difference and it is measured:
// `printf "[%s]" ${u:-{a,q}.z}` is `[a.z][q.z]` in zsh — two fields, so the
// operand ran to `.z` — against the single field `[{a,q.z}]` for the same line
// in quotes. Without this row the fix for #1586 reads as "a bare brace never
// nests", which is a rule this shell measurably does not have.
func TestABareBraceOutsideQuotesStillBalances(t *testing.T) {
	// Unquoted and unbalanced: the operand is still looking for its `}`.
	if _, err := Parse(`echo ${u:-{a,q}`, Core()); err == nil {
		t.Error("Parse of an unbalanced unquoted group succeeded, want a failure")
	}
	// Unquoted and balanced: the group closes and then the expansion does.
	if _, err := Parse(`echo ${u:-{a,q}.z}`, Core()); err != nil {
		t.Errorf("Parse of the balanced unquoted group = %v, want it to parse", err)
	}
}
