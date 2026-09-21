// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package coverage_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/zsh"
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
//
// "Report only" used to be the last line of it and is not any more: the
// roll-up's zero gates, in internal/cmd/coverage's own test, and a caveat
// still saying nothing here gates anything would be telling a reader the
// opposite of what a red build is about to tell them (#3990).
func TestTheReportSaysWhatAMentionIsNot(t *testing.T) {
	col, err := coverage.Run("core", syntax.Core(), []string{"echo"}, []coverage.Source{{Label: "x", Text: `echo hi`}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := coverage.Report([]coverage.Column{col}, 5, []coverage.Origin{{Name: "cases", Count: 1}})
	for _, want := range []string{
		"A mention is not coverage", "axis-sweep",
		"The roll-up's zero is the one thing here that gates",
		"Every number\n    above it is report-only",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not say %q", want)
		}
	}
}

// TestTheRollupForgivesWhatOnlyOneGrammarHas. The operator vocabulary is one
// table shared by every dialect, so an operator only one shell has reads as
// never mentioned under the others. That per-column zero is not a work item;
// the roll-up is what says nobody asked about it anywhere.
func TestTheRollupForgivesWhatOnlyOneGrammarHas(t *testing.T) {
	asks, err := coverage.Run("asks", syntax.Core(), []string{"echo", "read"}, []coverage.Source{
		{Label: "a", Text: `read x`},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	silent, err := coverage.Run("silent", syntax.Core(), []string{"echo", "read"}, []coverage.Source{
		{Label: "b", Text: `echo hi`},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The column that never said `read` reports it, which is the context the
	// roll-up is read against.
	var silentMisses []string
	for _, e := range silent.Unasked() {
		if e.Kind == coverage.KindBuiltin {
			silentMisses = append(silentMisses, e.Name)
		}
	}
	if !slices.Contains(silentMisses, "read") {
		t.Errorf("the silent column does not report `read` as unmentioned; it has %v", silentMisses)
	}

	out := coverage.Report([]coverage.Column{asks, silent}, 0, nil)
	roll := out[strings.Index(out, "no dialect mentions these at all"):]
	names := rolledUp(t, roll, coverage.KindBuiltin)
	if slices.Contains(names, "read") {
		t.Errorf("a builtin one column mentioned is in the roll-up, which is the union of what was asked and not the intersection; it has %v", names)
	}
	if slices.Contains(names, "echo") {
		t.Errorf("a builtin the other column mentioned is in the roll-up; it has %v", names)
	}
}

// rolledUp reads one kind's names out of the roll-up section.
func rolledUp(t *testing.T, roll, kind string) []string {
	t.Helper()
	lines := strings.Split(roll, "\n")
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), kind+" (") && i+1 < len(lines) {
			return strings.Fields(lines[i+1])
		}
	}
	return nil
}

// TestTheReportSaysWhatItRead. The denominator of this instrument is whatever
// body of cases it was handed, and for a day it was handed half of one: the
// corpus, with our own 35-file suite sitting unread in the tree. The number
// was right and answered a question nobody meant to ask, which is a failure a
// reader cannot see unless the report says what it read (#2630).
func TestTheReportSaysWhatItRead(t *testing.T) {
	col, err := coverage.Run("core", syntax.Core(), []string{"echo"}, []coverage.Source{{Label: "x", Text: `echo hi`}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := coverage.Report([]coverage.Column{col}, 5, []coverage.Origin{
		{Name: "corpus cases", Count: 12},
		{Name: "files under share/suite", Count: 35},
	})
	if !strings.Contains(out, "read 12 corpus cases, 35 files under share/suite") {
		t.Errorf("the report does not say what it read; it begins\n%s", first(out, 4))
	}
	// A body of cases that could not be read has no count, and saying so is
	// the whole point: a report that quietly dropped it would understate the
	// denominator in exactly the way this line exists to prevent.
	out = coverage.Report([]coverage.Column{col}, 5, []coverage.Origin{
		{Name: "corpus cases", Count: 12},
		{Name: "share/suite — not found from here"},
	})
	if !strings.Contains(out, "read 12 corpus cases, share/suite — not found from here") {
		t.Errorf("an unread body of cases is not named; the report begins\n%s", first(out, 4))
	}
}

// first is the opening lines of a report, for a failure message.
func first(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// TestAGrammarReadingIsNotAMention is #3258. `ParamExpr.RawTailRead` holds a
// BraceQuotePolicy, the parse writes it only under a dialect whose POSIX mode
// moves the reading, and a reflection walk cannot tell that field unset from
// set to the zero. So `BraceQuoteProtectsAPatternOnly` read as mentioned by
// every parameter expansion under every dialect that never wrote it, and
// `BraceQuoteProtectsNothing` — zsh's reading, which no parse writes to a
// node — was a permanent zero. A type syntax.Dialect declares a field of is a
// reading, not a vocabulary a case spells.
func TestAGrammarReadingIsNotAMention(t *testing.T) {
	readings, err := coverage.ReadingTypes()
	if err != nil {
		t.Fatalf("ReadingTypes: %v", err)
	}
	if !readings["BraceQuotePolicy"] {
		t.Fatal("BraceQuotePolicy is not read as a Dialect field's type — the exclusion below would pass for the wrong reason")
	}
	ops, err := coverage.OperatorTypes()
	if err != nil {
		t.Fatalf("OperatorTypes: %v", err)
	}
	if slices.Contains(ops, "BraceQuotePolicy") {
		t.Errorf("a grammar reading is an operator vocabulary: %v", ops)
	}
	surface, err := coverage.Surface(nil)
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	for _, e := range surface {
		if strings.HasPrefix(e.Name, "BraceQuote") {
			t.Errorf("%s is in the surface", e)
		}
	}
	// The false mention itself, under a grammar that never writes the field
	// and under the one that does.
	src := `v=V; printf '[%s]\n' "${v-'a}b'}" "${v}"`
	for name, d := range map[string]syntax.Dialect{"core": syntax.Core(), "bash": bash.Dialect(), "zsh": zsh.Dialect()} {
		f, err := syntax.Parse(src, d)
		if err != nil {
			t.Fatalf("%s: parse: %v", name, err)
		}
		into := map[coverage.Element]int{}
		if err := coverage.Mentions(f, func(string) bool { return false }, into); err != nil {
			t.Fatalf("%s: mentions: %v", name, err)
		}
		for e := range into {
			if strings.HasPrefix(e.Name, "BraceQuote") {
				t.Errorf("%s: %q mentions %s", name, src, e)
			}
		}
		if !has(into, coverage.OperatorKind("ParamOp"), "ParamDefault") {
			t.Errorf("%s: the real operator in %q was not reached, so the exclusion took too much", name, src)
		}
	}
}

// TestEveryOperatorVocabularyHadItsZeroConsidered is a tripwire, and says so.
// A walk over a node field cannot tell unset from the type's zero (#3258).
// For the three vocabularies here that is sound: TokEOF, Kind's zero, is not a
// redirection and is never an element, ParamNone is what the parse
// decides for an expansion with no operator, and TerminatedByNothing is what
// it decides for a statement with no terminator — the last of a body, one
// closed by the `}` or `)` around it, and one ended by a `&`, which is
// recorded on Stmt.Background instead. A new node field of a new named
// type needs the same question asked of its zero before this list grows.
func TestEveryOperatorVocabularyHadItsZeroConsidered(t *testing.T) {
	ops, err := coverage.OperatorTypes()
	if err != nil {
		t.Fatalf("OperatorTypes: %v", err)
	}
	if want := []string{"Kind", "ParamOp", "Terminator"}; !slices.Equal(ops, want) {
		t.Errorf("operator vocabularies are %v, want %v. For each new one: is its zero a value the parse "+
			"decides, or what an unset field holds? If the second, a mention of it is false — see #3258", ops, want)
	}
}

// TestTheLedgerIsCheckedAgainstTheColumns. An unreachable element leaves the
// headline and is printed with its measurement; an entry the columns
// contradict is printed as stale rather than silently subtracted, because a
// ledger that could only grow would hide exactly what this report exists to
// show.
func TestTheLedgerIsCheckedAgainstTheColumns(t *testing.T) {
	col, err := coverage.Run("core", syntax.Core(), []string{"echo", "suspend", "read"}, []coverage.Source{
		{Label: "x", Text: `echo hi`},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	surfaceSize := len(col.Surface)
	ledger := []coverage.Unreachable{
		{Element: coverage.Element{Kind: coverage.KindBuiltin, Name: "suspend"}, Issue: 1, Measured: "it stops the harness"},
		{Element: coverage.Element{Kind: coverage.KindBuiltin, Name: "echo"}, Issue: 2, Measured: "claimed, and asked anyway"},
		{Element: coverage.Element{Kind: coverage.KindBuiltin, Name: "gone"}, Issue: 3, Measured: "no longer a builtin"},
	}
	out := coverage.ReportWithLedger([]coverage.Column{col}, 0, nil, ledger)
	roll := out[strings.Index(out, "no dialect mentions these at all"):]
	names := rolledUp(t, roll, coverage.KindBuiltin)
	if slices.Contains(names, "suspend") {
		t.Errorf("a ledgered element is still in the work-list: %v", names)
	}
	if !slices.Contains(names, "read") {
		t.Errorf("an unledgered element left the work-list: %v", names)
	}
	unasked := len(col.Unasked())
	head := fmt.Sprintf("no dialect mentions these at all — %d of %d reachable", unasked-1, surfaceSize-1)
	if !strings.Contains(roll, head) {
		t.Errorf("the headline does not count reachable elements; want %q in\n%s", head, first(roll, 3))
	}
	for _, want := range []string{
		"builtin suspend (#1)", "it stops the harness",
		"builtin echo (#2) — some case mentions it",
		"builtin gone (#3) — no dialect's surface holds it",
	} {
		if !strings.Contains(roll, want) {
			t.Errorf("the roll-up does not say %q:\n%s", want, roll)
		}
	}
	if strings.Contains(roll, "builtin suspend (#1) —") {
		t.Error("an entry the columns agree with is printed as stale")
	}
}

// TestTheCommittedLedgerCarriesItsEvidence. An entry with no issue and no
// measurement is a forgiveness, which is not what the ledger is for.
func TestTheCommittedLedgerCarriesItsEvidence(t *testing.T) {
	for _, u := range coverage.UnreachableByConstruction {
		if u.Issue == 0 || len(u.Measured) < 40 {
			t.Errorf("%s is ledgered without the measurement that says it is unreachable", u.Element)
		}
		if u.Element.Kind == "" || u.Element.Name == "" {
			t.Errorf("a ledger entry names no element: %+v", u)
		}
	}
}
