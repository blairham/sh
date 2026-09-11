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
//
// The measurement lives here rather than in the corpus, and that is forced
// rather than chosen: the corpus is read by one grammar wide enough for every
// case in it, and this construct needs PatternAlternation, which that grammar
// does not have. Turning it on there made three `pat/` cases — the `(#m)` and
// `(#b)` flag rows — parse, and those are in the corpus precisely because the
// reference shells reject them, so widening to admit this row would have
// falsified theirs. A test can set the one flag it means; corpusDialect cannot.
// **And with `=(cmd)` in the grammar too**, which is the combination the one
// shell that has either actually runs. `=(` opens a temp-file process
// substitution there (#1878), and a rule for it written as "an `=` in front
// of a `(`" would take every row below for one — so both flags are set on the
// second pass and every assertion is made twice. Without that pass this test
// would pass on a grammar no shell has.
func TestAParenAfterEqualsIsAGroupInAConditionAndALiteralOutsideOne(t *testing.T) {
	for _, withFileSubst := range []bool{false, true} {
		d := Core()
		d.PatternAlternation = true
		d.ProcessSubstitutionToFile = withFileSubst

		// The condition: the group is a group, alternation and all.
		for _, src := range []string{
			`[[ a=b == a=(b) ]]`,
			`[[ a=b == a=(b|c) ]]`,
			`[[ $x == pre[$' \t']#=([^z]#)post ]]`,
			// The p10k line itself, as it is written in
			// `internal/p10k.zsh`: the `=` is the twelfth character of the
			// word, so the `(` behind it is the pattern's capture group and
			// not a substitution.
			`[[ $'\n'$cfg$'\n' == (#b)*$'\n'prompt[$' \t']#=([^$'\n']#)$'\n'* ]]`,
		} {
			if _, err := Parse(src, d); err != nil {
				t.Errorf("ProcessSubstitutionToFile=%v: Parse(%q) = %v, want it to parse",
					withFileSubst, src, err)
			}
		}

		// The assignment, which is the case the guard was written for: still
		// an array literal, and still two elements rather than one pattern
		// word.
		f, err := Parse(`a=(b c)`, d)
		if err != nil {
			t.Fatalf("ProcessSubstitutionToFile=%v: Parse of the array literal = %v", withFileSubst, err)
		}
		if got := len(f.Stmts); got != 1 {
			t.Fatalf("ProcessSubstitutionToFile=%v: array literal parsed to %d statements, want 1",
				withFileSubst, got)
		}
		a := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd).Assigns[0]
		if !a.IsArray || len(a.Elems) != 2 {
			t.Errorf("ProcessSubstitutionToFile=%v: `a=(b c)` is array=%v with %d elements, want an array of 2",
				withFileSubst, a.IsArray, len(a.Elems))
		}
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

// Unquoted, whether the same brace has to balance is a dialect question —
// #1587, the other half of #1586.
//
// The panel splits two against three, so the flag carries it rather than the
// scanner. Measured 2026-09-10 with `unset u; printf "[%s]" ${u:-{a,q}.z}`:
// zsh 5.9.2 and ksh93 answer `[a.z][q.z]`, two fields, so the operand ran to
// `.z` and the group was then expanded; bash 5.3.15, bash 3.2.57 and dash
// answer the single field `[{a,q.z}]`, the operand stopping at the first `}`
// and `.z}` arriving as literal text.
//
// Both halves are here because either alone reads as a rule this shell does
// not have: with the flag on and no off row it is "a bare brace always
// balances", and with it off and no on row it is "a bare brace never nests".
func TestBareBraceNestingInAnUnquotedExpansionFollowsTheFlag(t *testing.T) {
	on, off := Core(), Core()
	on.BareBraceNestsInExpansion = true

	// With the flag, the operand is still looking for its `}` and the input
	// runs out; without it the expansion closed at the first one and the
	// `.z` that follows is the rest of an ordinary word.
	if _, err := Parse(`echo ${u:-{a,q}`, on); err == nil {
		t.Error("Parse of an unbalanced group with the flag succeeded, want a failure")
	}
	if _, err := Parse(`echo ${u:-{a,q}`, off); err != nil {
		t.Errorf("Parse of the same text without the flag = %v, want it to parse", err)
	}
	if _, err := Parse(`echo ${u:-{a,q}.z}`, on); err != nil {
		t.Errorf("Parse of the balanced group with the flag = %v, want it to parse", err)
	}

	// Where the expansion *ends* is the whole of the difference, and the
	// spans say so without running anything: one word either way, and one
	// span with the flag against two without it.
	for _, tc := range []struct {
		d     Dialect
		spans int
		value string
	}{
		{on, 1, "u:-{a,q}.z"},
		{off, 2, "u:-{a,q"},
	} {
		w := firstArgument(t, `echo ${u:-{a,q}.z}`, tc.d)
		if len(w.Spans) != tc.spans {
			t.Fatalf("BareBraceNestsInExpansion=%v: %d spans, want %d", tc.d.BareBraceNestsInExpansion, len(w.Spans), tc.spans)
		}
		if got := w.Spans[0].Value; got != tc.value {
			t.Errorf("BareBraceNestsInExpansion=%v: expansion is %q, want %q", tc.d.BareBraceNestsInExpansion, got, tc.value)
		}
	}
}

// In double quotes the flag is not consulted at all: every column in the
// panel stops at the first `}` there, which is what #1586 settled.
func TestBareBraceNestingIsNotConsultedInsideQuotes(t *testing.T) {
	on := Core()
	on.BareBraceNestsInExpansion = true
	for _, d := range []Dialect{Core(), on} {
		w := firstArgument(t, `echo "${a/x/{y}"`, d)
		if len(w.Spans) != 1 || w.Spans[0].Value != "a/x/{y" {
			t.Errorf("BareBraceNestsInExpansion=%v: %+v, want the one span `a/x/{y`", d.BareBraceNestsInExpansion, w.Spans)
		}
	}
}

// firstArgument parses one command and hands back the word after its name.
func firstArgument(t *testing.T, src string, d Dialect) *Word {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("Parse(%q) = %v", src, err)
	}
	pipe, ok := f.Stmts[0].Expr.(*Pipeline)
	if !ok {
		t.Fatalf("Parse(%q) did not give a pipeline", src)
	}
	c, ok := pipe.Cmds[0].(*SimpleCmd)
	if !ok || len(c.Args) < 2 {
		t.Fatalf("Parse(%q) did not give a command with an argument", src)
	}
	return c.Args[1]
}
