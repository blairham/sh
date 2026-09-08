// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// An expansion inside a pattern group is a span of its own kind, not bytes of
// the group's literal text (#1331).
//
// The group scanner grew out of scanWord and kept a *copy* of its case list
// with the quoting forms in it and none of the expansions, so a `$L` inside a
// group stayed the two characters it was written as and reached the matcher
// as a pattern nobody wrote. There is one list now and both scanners read it.
//
// The span kind is what the assertion is on rather than the parse succeeding:
// the old reading parsed everything here — that is what made it silent.
func TestAnExpansionInsideAPatternGroupIsASpan(t *testing.T) {
	on := patGroup()
	on.DollarSingleQuote = true
	on.DollarBracketArith = true
	on.ParamTildeFlag = true
	for _, tc := range []struct {
		src  string
		kind SpanKind
		val  string // the span's Value, where the kind alone is not enough
	}{
		{`echo ($L)`, ParamExp, "L"},
		{`echo (${L})`, ParamExp, "L"},
		{`echo (${~L})`, ParamExp, "L"},
		{`echo (a${L}b)`, ParamExp, "L"},
		{`echo ($(id))`, CommandSubst, "id"},
		{"echo (`id`)", CommandSubst, "id"},
		{`echo ($((1+1)))`, ArithSubst, "1+1"},
		{`echo ($[1+1])`, ArithSubst, "1+1"},
		// Nested one group deep, which is the shape the backreference
		// construct this was found in has.
		{`echo ((--|)($L)(*))`, ParamExp, "L"},
		// Mid-word, where the group does not open the word.
		{`echo a($L)b`, ParamExp, "L"},
		// And in a condition, the route the bug was filed from.
		{`[[ $k == ($L) ]]`, ParamExp, "L"},
	} {
		f, err := Parse(tc.src, on)
		if err != nil {
			t.Errorf("%s: %v", tc.src, err)
			continue
		}
		var spans []Span
		switch c := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(type) {
		case *SimpleCmd:
			spans = c.Args[1].Spans
		default:
			// The condition: reach the right-hand operand through the tree
			// rather than by position, so a grammar change is a compile
			// error and not a silent skip.
			spans = conditionRightOperand(t, tc.src, f)
		}
		var found bool
		for _, s := range spans {
			if s.Kind == tc.kind && spanText(s) == tc.val {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no %v span holding %q — the group was taken as text: %#v",
				tc.src, tc.kind, tc.val, spans)
		}
	}
}

// spanText is the text a span carries, wherever its kind keeps it.
func spanText(s Span) string {
	if s.Kind == ParamExp && s.Param != nil {
		return s.Param.Name
	}
	return s.Value
}

// conditionRightOperand digs the pattern operand out of a `[[ x == y ]]`.
func conditionRightOperand(t *testing.T, src string, f *File) []Span {
	t.Helper()
	tc, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*TestClause)
	if !ok {
		t.Fatalf("%s: not a test clause", src)
	}
	bin, ok := tc.Expr.(*CondBinary)
	if !ok {
		t.Fatalf("%s: not a binary condition", src)
	}
	return bin.Y.Spans
}

// A group's `(` and `)` still belong to the group, and a substitution's do
// not: the scanner that takes a substitution balances its own parentheses, so
// the group closes at the last `)` in the word rather than at the first one
// the command happens to contain.
func TestASubstitutionsParenthesesAreNotTheGroups(t *testing.T) {
	on := patGroup()
	for _, tc := range []struct{ src, text string }{
		{`echo ($(echo ')')x)`, "(x)"},
		{`echo (${x:-)}y)`, "(y)"},
		{"echo (`echo ')'`z)", "(z)"},
	} {
		f, err := Parse(tc.src, on)
		if err != nil {
			t.Errorf("%s: %v", tc.src, err)
			continue
		}
		cmd := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
		// The literal text the group contributed, with the substitution's
		// own span left out: it is the parentheses that are being counted.
		var lit string
		for _, s := range cmd.Args[1].Spans {
			if s.Kind == Literal {
				lit += s.Value
			}
		}
		if lit != tc.text {
			t.Errorf("%s: the group's literal text is %q, want %q", tc.src, lit, tc.text)
		}
	}
}

// Printed source has to mean the same thing, which for a group holding an
// expansion means the expansion comes back as an expansion: printing `(L)`
// where `($L)` was written is a different pattern, and escaping the `$` is a
// different one again.
func TestAPatternGroupsExpansionPrintsBackAsItWasWritten(t *testing.T) {
	on := patGroup()
	for _, src := range []string{
		`echo ($L)`,
		`echo (${L})`,
		`echo (a${L}b)*`,
		`echo ($(id))`,
		`echo a($L|b)c`,
		`[[ $k == ($L) ]]`,
	} {
		f, err := Parse(src, on)
		if err != nil {
			t.Errorf("%s: %v", src, err)
			continue
		}
		printed := Print(f)
		g, err := Parse(printed, on)
		if err != nil {
			t.Errorf("%s: printed as %q, which does not parse: %v", src, printed, err)
			continue
		}
		if again := Print(g); again != printed {
			t.Errorf("%s: printed %q, reprinted %q", src, printed, again)
		}
	}
}
