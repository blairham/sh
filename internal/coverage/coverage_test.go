// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package coverage_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/coverage"
	"github.com/blairham/sh/syntax"
)

// mentions parses src under the core grammar and returns what it reached.
func mentions(t *testing.T, src string, builtins ...string) map[coverage.Element]int {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	into := map[coverage.Element]int{}
	if err := coverage.Mentions(f, func(n string) bool {
		return slices.Contains(builtins, n)
	}, into); err != nil {
		t.Fatalf("mentions: %v", err)
	}
	return into
}

func has(m map[coverage.Element]int, kind, name string) bool {
	return m[coverage.Element{Kind: kind, Name: name}] > 0
}

// TestTheSurfaceIsReadFromTheTree. Each enumeration has a failure mode that
// looks like success — an empty result reads as "everything is covered" — so
// each one is checked for a member it cannot lose without the grammar losing
// it too.
func TestTheSurfaceIsReadFromTheTree(t *testing.T) {
	nodes, err := coverage.NodeTypes()
	if err != nil {
		t.Fatalf("NodeTypes: %v", err)
	}
	for _, want := range []string{"File", "Stmt", "SimpleCmd", "ParamExpr", "TestClause", "CaseClause"} {
		if !slices.Contains(nodes, want) {
			t.Errorf("node types do not include %s — they are found by the `Pos` method, so a miss here means the source was not read", want)
		}
	}
	consts, err := coverage.ConstantNames()
	if err != nil {
		t.Fatalf("ConstantNames: %v", err)
	}
	for _, tc := range []struct {
		typ, name string
		value     int64
	}{
		{"ParamOp", "ParamNone", 0},
		{"ParamOp", "ParamDefault", 1},
		{"Kind", "TokEOF", 0},
	} {
		if got := consts[tc.typ][tc.value]; got != tc.name {
			t.Errorf("%s(%d) read as %q, want %q — iota is resolved here, and an off-by-one names every constant wrongly",
				tc.typ, tc.value, got, tc.name)
		}
	}
	conds, err := coverage.CondOperators()
	if err != nil {
		t.Fatalf("CondOperators: %v", err)
	}
	for _, want := range []string{"-f", "-z", "-eq", "=~", "-v", "-prefix"} {
		if !slices.Contains(conds, want) {
			t.Errorf("condition operators do not include %q — the tables in syntax/cond.go and that file's own switch are both read, and %q is in one of them", want, want)
		}
	}
}

// TestLexerOnlyTokensAreNotInTheSurface. `Kind` is the whole token
// vocabulary and most of it never reaches a node, so listing it all would
// produce elements no case can ever mention — a permanent zero, and a work
// item nobody can do. The subset comes from the shell's own IsRedirect.
func TestLexerOnlyTokensAreNotInTheSurface(t *testing.T) {
	surface, err := coverage.Surface(nil)
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	var redirs, strays []string
	for _, e := range surface {
		switch {
		case e.Kind == coverage.KindRedirOp:
			redirs = append(redirs, e.Name)
		case strings.HasPrefix(e.Name, "Tok"):
			strays = append(strays, e.Name)
		}
	}
	if len(strays) != 0 {
		t.Errorf("token constants outside the redirection group are in the surface: %v", strays)
	}
	for _, want := range []string{"TokLess", "TokGreat", "TokDGreat", "TokDLess"} {
		if !slices.Contains(redirs, want) {
			t.Errorf("redirection operators do not include %s", want)
		}
	}
	for _, unwanted := range []string{"TokSemicolon", "TokEOF", "TokAndAnd"} {
		if slices.Contains(redirs, unwanted) {
			t.Errorf("%s is not a redirection operator and must not be an element", unwanted)
		}
	}
}

// TestABuiltinCountsInCommandPositionOnly. `echo read` says nothing about
// `read`, and counting it is exactly the overstatement this instrument is
// supposed to be careful about.
func TestABuiltinCountsInCommandPositionOnly(t *testing.T) {
	m := mentions(t, `echo read`, "echo", "read")
	if !has(m, coverage.KindBuiltin, "echo") {
		t.Error("the command word was not counted")
	}
	if has(m, coverage.KindBuiltin, "read") {
		t.Error("an argument that happens to name a builtin was counted as asking about it")
	}
}

// TestAWordThatIsNotALiteralNamesNothing: a command word the shell works out
// at run time is not a mention of whatever it comes to.
func TestAWordThatIsNotALiteralNamesNothing(t *testing.T) {
	m := mentions(t, `$cmd arg`, "echo", "read")
	for e := range m {
		if e.Kind == coverage.KindBuiltin {
			t.Errorf("an expanded command word was read as naming %q", e.Name)
		}
	}
}

// TestBothSpellingsOfATestCountAsTheSameOperator. `[ -f x ]` is a simple
// command and `[[ -f x ]]` is a node with an Op field; a corpus written in
// the portable spelling would otherwise read as never having tested a file.
func TestBothSpellingsOfATestCountAsTheSameOperator(t *testing.T) {
	for _, src := range []string{`[ -f x ]`, `[[ -f x ]]`, `test -f x`} {
		t.Run(src, func(t *testing.T) {
			m := mentions(t, src, "[", "test")
			if !has(m, coverage.KindCondOp, "-f") {
				t.Errorf("%q did not count -f as a condition operator", src)
			}
		})
	}
	// And a word that is not an operator is not made one by sitting there.
	m := mentions(t, `[ -f x ]`, "[")
	if has(m, coverage.KindCondOp, "x") {
		t.Error("an operand was counted as an operator")
	}
}

// TestTheNodeKindsAWordCarriesAreReached. The tree's operators live inside
// words as well as on statements, and a walker that stopped at a word would
// report every parameter-expansion operator as never mentioned.
func TestTheNodeKindsAWordCarriesAreReached(t *testing.T) {
	m := mentions(t, `echo "${x:-d}" $((1+2)) $(date)`, "echo")
	if !has(m, coverage.KindNode, "ParamExpr") {
		t.Error("a parameter expansion inside a word was not reached")
	}
	if !has(m, coverage.OperatorKind("ParamOp"), "ParamDefault") {
		t.Error("the operator of a parameter expansion was not reached")
	}
}

// TestUnaskedNamesWhatWasNotMentioned, and — the half that matters — does not
// name what was.
func TestUnaskedNamesWhatWasNotMentioned(t *testing.T) {
	col, err := coverage.Run("core", syntax.Core(), []string{"echo", "read"}, []coverage.Source{
		{Label: "one", Text: `echo hi > f`},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if col.Parsed != 1 {
		t.Fatalf("Parsed = %d, want 1", col.Parsed)
	}
	unasked := col.Unasked()
	var names []string
	for _, e := range unasked {
		names = append(names, e.Kind+" "+e.Name)
	}
	if slices.Contains(names, "builtin echo") {
		t.Error("a builtin the source named is reported as never mentioned")
	}
	if slices.Contains(names, "redirection operator TokGreat") {
		t.Error("an operator the source used is reported as never mentioned")
	}
	if !slices.Contains(names, "builtin read") {
		t.Error("a builtin nothing named is missing from the work-list — the zero is the half of this report that is a fact")
	}
}

// TestACaseThatWillNotParseIsNotAFinding. A snippet written for another
// shell's grammar is not this column's surface, and the report says how many
// were read rather than failing on the rest.
func TestACaseThatWillNotParseIsNotAFinding(t *testing.T) {
	col, err := coverage.Run("core", syntax.Core(), []string{"echo"}, []coverage.Source{
		{Label: "good", Text: `echo hi`},
		{Label: "bad", Text: `if; then`},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if col.Sources != 2 || col.Parsed != 1 {
		t.Errorf("Sources/Parsed = %d/%d, want 2/1", col.Sources, col.Parsed)
	}
}

// TestTheReportSaysWhatAMentionIsNot. A coverage number that overstates
// itself retires the question, so the caveat is part of the output rather
// than a thing a reader is expected to remember.
func TestTheReportSaysWhatAMentionIsNot(t *testing.T) {
	col, err := coverage.Run("core", syntax.Core(), []string{"echo"}, []coverage.Source{{Label: "x", Text: `echo hi`}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := coverage.Report([]coverage.Column{col}, 5)
	for _, want := range []string{"A mention is not coverage", "axis-sweep", "Report only"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not say %q", want)
		}
	}
}
