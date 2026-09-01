// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

func cond(c CondExpr) string {
	switch x := c.(type) {
	case nil:
		return "<nil>"
	case *CondUnary:
		return "(" + x.Op + " " + x.X.Literal() + ")"
	case *CondBinary:
		return "(" + x.X.Literal() + " " + x.Op + " " + x.Y.Literal() + ")"
	case *CondLogic:
		return "(" + cond(x.X) + " " + x.Op + " " + cond(x.Y) + ")"
	case *CondNot:
		return "(! " + cond(x.X) + ")"
	case *CondGroup:
		return "[" + cond(x.X) + "]"
	}
	return "?"
}

func parseCond(t *testing.T, src string) CondExpr {
	t.Helper()
	f, err := Parse(src, Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	tc, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*TestClause)
	if !ok {
		t.Fatalf("%q did not parse to a test clause", src)
	}
	return tc.Expr
}

func TestCondOperators(t *testing.T) {
	tests := []struct{ src, want string }{
		{`[[ a == b ]]`, `(a == b)`},
		{`[[ a = b ]]`, `(a = b)`},
		{`[[ a != b ]]`, `(a != b)`},
		{`[[ 10 -gt 9 ]]`, `(10 -gt 9)`},
		{`[[ a =~ ^b$ ]]`, `(a =~ ^b$)`},
		{`[[ -n x ]]`, `(-n x)`},
		{`[[ -f path ]]`, `(-f path)`},
		// A bare word is a test for non-emptiness.
		{`[[ x ]]`, `(-n x)`},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			if got := cond(parseCond(t, tc.src)); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestCondReinterpretsLessAndGreater(t *testing.T) {
	// The lexer produced these as redirection operators, deliberately: `[[`
	// is only special where a command may begin and the lexer does not know
	// where that is. The parser does, so it reinterprets them.
	//
	// They are *string* comparisons, which is the sharpest trap in the
	// construct — `[[ 10 > 9 ]]` is false.
	if got, want := cond(parseCond(t, `[[ a < b ]]`)), `(a < b)`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if got, want := cond(parseCond(t, `[[ 10 > 9 ]]`)), `(10 > 9)`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	// And no redirection was produced by any of it.
	f, _ := Parse(`[[ a < b ]]`, Core())
	tc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*TestClause)
	if len(tc.Redirs) != 0 {
		t.Errorf("a comparison became %d redirection(s)", len(tc.Redirs))
	}
}

func TestCondLogicAndGrouping(t *testing.T) {
	tests := []struct{ src, want string }{
		{`[[ -n a && -n b ]]`, `((-n a) && (-n b))`},
		{`[[ -n a || -n b ]]`, `((-n a) || (-n b))`},
		{`[[ ! -z x ]]`, `(! (-z x))`},
		// ( ) groups a condition rather than starting a subshell.
		{`[[ ( -n a || -n b ) && -n c ]]`, `([((-n a) || (-n b))] && (-n c))`},
		// && binds tighter than ||, as in C — unlike the command language,
		// where they share one level.
		{`[[ -n a || -n b && -n c ]]`, `((-n a) || ((-n b) && (-n c)))`},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			if got := cond(parseCond(t, tc.src)); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestCondRightOperandKeepsItsQuoting(t *testing.T) {
	// Unquoted the right operand is a pattern; quoted it is a literal. Only
	// the spans still know which, so the tree keeps a word rather than a
	// string.
	unq := parseCond(t, `[[ abc == a* ]]`).(*CondBinary)
	if unq.Y.Spans[0].Quoting != Unquoted {
		t.Error("an unquoted operand lost its unquotedness")
	}
	q := parseCond(t, `[[ abc == "a*" ]]`).(*CondBinary)
	if q.Y.Spans[0].Quoting == Unquoted {
		t.Error("a quoted operand lost its quoting, so a pattern cannot be told from a literal")
	}
}

func TestDoubleBracketIsOnlySpecialInCommandPosition(t *testing.T) {
	// `echo [[ a ]]` prints `[[ a ]]`, so this must stay a simple command.
	// It is the case a lexer mode would have broken, now checked from the
	// parser's side as well.
	f, err := Parse(`echo [[ a ]]`, Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd); !ok {
		t.Fatalf("`echo [[ a ]]` became %T", f.Stmts[0].Expr.(*Pipeline).Cmds[0])
	}
	if got, want := dump(f), `cmd[echo [[ a ]]]`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestDoubleBracketAbsentFromPosix(t *testing.T) {
	// Where the dialect lacks it the text still parses and means something
	// else — a command named `[[` with a redirection, which is what dash
	// does and why `[[ a < b ]]` opens the file b there.
	f, err := Parse(`[[ a < b ]]`, POSIX())
	if err != nil {
		t.Fatal(err)
	}
	sc, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if !ok {
		t.Fatalf("under posix this should be a simple command, got %T",
			f.Stmts[0].Expr.(*Pipeline).Cmds[0])
	}
	if len(sc.Redirs) != 1 {
		t.Fatalf("want the < to be a redirection under posix, got %d", len(sc.Redirs))
	}
	if got := sc.Redirs[0].Word.Literal(); got != "b" {
		t.Errorf("redirection target = %q, want b", got)
	}
}

func TestCondNeverPanics(t *testing.T) {
	for _, src := range []string{
		`[[`, `[[ ]]`, `[[ a ]]`, `[[ a ==`, `[[ ( ]]`, `[[ ! ]]`,
		`[[ && ]]`, `[[ a == ]]`, `[[ ( a || ) ]]`, `[[ -n ]]`,
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic on %q: %v", src, r)
				}
			}()
			_, _ = Parse(src, Core())
		}()
	}
}

// TestARegexOperandOwnsItsParentheses: after `=~` the operand is a regular
// expression, so its parentheses are the regex's and do not end the word.
// Where a word ends is settled by the lexer, so the lexer has to be told.
func TestARegexOperandOwnsItsParentheses(t *testing.T) {
	for _, src := range []string{
		`[[ abc =~ ^(a|x)bc$ ]]`,
		`[[ abc =~ (b) ]]`,
		`[[ abc =~ ^((a)(b))c$ ]]`,
		`[[ abc =~ (a)(b) ]]`,
	} {
		mustParse(t, src, Core(), "a group in a regex")
	}
	// Scoped to the operand: everywhere else a `(` is the shell's again.
	mustParse(t, `( echo subshell )`, Core(), "a subshell")
	mustParse(t, `[[ (a = a) ]]`, Core(), "grouping inside a condition")
	mustFail(t, `[[ abc == (b) ]]`, Core(), "a group after == is not a regex")
}

// TestABareAlternationInARegexIsADialectAnswer: two of the three shells with
// `[[ ]]` take a bare `|` as the regex's, and the third ends the word there.
func TestABareAlternationInARegexIsADialectAnswer(t *testing.T) {
	takes := Core()
	takes.RegexTakesAlternation = true
	mustParse(t, `[[ ab =~ a|b ]]`, takes, "a bare alternation where the dialect takes it")
	mustFail(t, `[[ ab =~ a|b ]]`, Core(), "a bare alternation where it does not")
	// Inside parentheses it belongs to the group either way, which is why the
	// flag is about the *bare* one.
	mustParse(t, `[[ ab =~ (a|b) ]]`, Core(), "an alternation inside a group")
}
