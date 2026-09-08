// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

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

// Process substitution is the one construct substitutionSpans leaves out, and
// the assertion is that the *word does not parse* rather than that running it
// exits non-zero.
//
// The status is not a discriminator and the first version of this asserted
// one. `eval '[[ x == (a<(echo x)b) ]]'; echo $?` is 1 whether the `<(` was
// read or refused — refused it is a parse failure, and read it is a condition
// that compares a subject against a path and does not match — so a mutant
// that put `startsProcSubst` into the shared list survived the whole suite.
// A marker file is no better: `<(touch m)` races the check, and zsh 5.9.2
// answers "not run" for a substitution it certainly performed.
//
// Where the two readings differ without racing anything is the parse, which
// is also where the decision is: zsh 5.9.2 refuses both surfaces —
// `process substitution … cannot be used here` and `number expected` — so
// reading one here would make two constructs work that the shell does not
// have.
func TestAProcessSubstitutionInsideAGroupIsNotOne(t *testing.T) {
	on := patGroup()
	on.ProcessSubstitution = true
	for _, src := range []string{
		`[[ $k == (a<(echo x)b) ]]`,
		`echo (a<(echo x)b)`,
		`echo (a>(echo x)b)`,
		`echo (<(echo x))`,
	} {
		if _, err := Parse(src, on); err == nil {
			t.Errorf("%s: parsed, where the `<(` has to stay the operator that ends the word", src)
		}
	}
	// The controls, which are what keep the rows above about the *group*: a
	// process substitution outside one is still a substitution, and the same
	// group without it still parses.
	for _, src := range []string{
		`echo <(echo x)`,
		`echo a<(echo x)b`,
		`echo (axb)`,
		`[[ $k == (a|b) ]]`,
	} {
		if _, err := Parse(src, on); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
}

// Printed source has to mean the same thing, which for a group holding an
// expansion means the expansion comes back as an expansion: printing `(L)`
// where `($L)` was written is a different pattern, and escaping the `$` is a
// different one again.
func TestAPatternGroupsExpansionPrintsBackAsItWasWritten(t *testing.T) {
	on := patGroup()
	for _, tc := range []struct{ src, group string }{
		{`echo ($L)`, `($L)`},
		// The braces come off where nothing needs them, which is what the
		// printer does with `${L}` anywhere else in a word — the group
		// changes nothing about it.
		{`echo (${L})`, `($L)`},
		{`echo (a${L}b)*`, `(a${L}b)*`},
		{`echo ($(id))`, `($(id))`},
		{`echo a($L|b)c`, `a($L|b)c`},
		{`[[ $k == ($L) ]]`, `($L)`},
	} {
		f, err := Parse(tc.src, on)
		if err != nil {
			t.Errorf("%s: %v", tc.src, err)
			continue
		}
		printed := Print(f)
		// The word itself, not only a stable round trip. Reprinting
		// `\(L\)` for `($L)` is stable and is a different pattern, and
		// escaping the `$` is a different one again — so the text is what
		// is asserted and the round trip below is the second half of it.
		if !strings.Contains(printed, tc.group) {
			t.Errorf("%s: printed %q, which does not hold %q", tc.src, printed, tc.group)
		}
		g, err := Parse(printed, on)
		if err != nil {
			t.Errorf("%s: printed as %q, which does not parse: %v", tc.src, printed, err)
			continue
		}
		if again := Print(g); again != printed {
			t.Errorf("%s: printed %q, reprinted %q", tc.src, printed, again)
		}
	}
}
