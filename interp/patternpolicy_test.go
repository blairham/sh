// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// patternPolicyPatterns put every question the setup asks of a pattern's
// text, each with a pattern that answers it yes and one beside it that does
// not: a caret, an escaped `]`, a class, a class nothing closes, an unknown
// class, a backslash inside a bracket, a collating element and an
// equivalence class, a bracket nothing closes, a group nothing closes, a
// bracket a class left open, the empty brackets, and the shell's own class
// names. The plain ones at the end are for the rows that count the route.
var patternPolicyPatterns = []string{
	`[^b]*`, `a[^x]c`, `[\]a]*`, `a\]`, `[[:alpha:]]*`, `*[[:digit:]]`, `[[:alpha]*`,
	`[[:nope:]]bc`, `[a\b]*`, `\[ab]`, `[[.a.]]bc`, `[[=a=]]*`, `a[[.b`, `[ab`, `x[`,
	`(a|b`, `a(b|c)`, `(#b)(a)(*)`, `x[[:alpha:]`, `[]a]*`, `[!]*`, `[]`, `[[:IFS:]]*`,
	`*[[:WORD:]]`, `[[:IDENT:]]bc`, `@(a|b)*`, `a?c`, `[ab]*`, `*[a-c]`,
}

// patternPolicySubjects are the subjects every pattern is matched against.
var patternPolicySubjects = []string{"abc", "]a", "a]", "x", "", "a b"}

// patternPolicyCounted are the patterns matched outside a subshell, so that
// the parent runner's count of the route is the one the test reads. None of
// them asks a question any vector below leaves open.
var patternPolicyCounted = []string{`a?c`, `[ab]*`, `*[a-c]`, `?b?`, `[a-z]bc`}

// patternPolicyVector is one set of answers the setup's policies read.
type patternPolicyVector struct {
	name   string
	enable func(*syntax.Dialect)
	sem    func(*Semantics)
	setup  func(*Runner)
}

var patternPolicyVectors = []patternPolicyVector{
	{name: "test vector"},
	{
		name: "bad patterns, empty brackets, the roster",
		enable: func(d *syntax.Dialect) {
			d.PatternAlternation = true
		},
		sem: func(s *Semantics) {
			s.BracketCaretNegates = Yes
			s.BracketEscape = BracketEscapeIsOnlyAMember
			s.MatchedBracketSkipsToAnEscapedBracket = Yes
			s.PatternClasses = "IFS IFSSPACE WORD IDENT"
			s.UnknownCharacterClass = UnknownClassEndsTheScan
			s.UnterminatedCharacterClass = UnterminatedClassEmptiesTheBracket
			s.CollatingElements = NoCollatingElements
			s.UnterminatedBracket = BracketBadPattern
			s.UnterminatedBracketAfterASubExpression = BracketBadPattern
			s.EmptyBracketExpressionCompiles = Yes
		},
		setup: func(r *Runner) { r.SetMatchOption(ExtendedPatternOperators, true) },
	},
	{
		name: "the other answers",
		enable: func(d *syntax.Dialect) {
			d.ExtendedPattern = true
		},
		sem: func(s *Semantics) {
			s.BracketCaretNegates = No
			s.BracketEscape = BracketEscapeProtectsTheMember
			s.MatchedBracketSkipsToAnEscapedBracket = No
			s.PatternClasses = ""
			s.UnknownCharacterClass = UnknownClassEmptiesTheBracket
			s.UnterminatedCharacterClass = UnterminatedClassIsOrdinaryCharacters
			s.CollatingElements = CollatingElementsHoldNothing
			s.UnterminatedBracket = BracketLiteral
			s.UnterminatedBracketAfterASubExpression = BracketLiteral
			s.EmptyBracketExpressionCompiles = No
		},
	},
	{
		name: "every question open",
		sem: func(s *Semantics) {
			s.BracketCaretNegates = Unspecified
			s.BracketEscape = BracketEscapeUnspecified
			s.CollatingElements = CollatingElementsUnspecified
			s.UnknownCharacterClass = UnknownClassUnspecified
			s.UnterminatedCharacterClass = UnterminatedClassUnspecified
			s.UnterminatedBracket = BracketUnspecified
			s.UnterminatedBracketAfterASubExpression = BracketUnspecified
		},
	},
	{
		name: "bad patterns with groups left literal",
		sem: func(s *Semantics) {
			s.UnterminatedBracket = BracketBadPattern
			s.UnterminatedBracketAfterASubExpression = BracketBadPattern
			s.CollatingElements = CollatingElementsHoldNothing
			s.PatternClasses = "WORD"
		},
		setup: func(r *Runner) { r.SetMatchOption(MatchFoldsCase, true) },
	},
}

// patternPolicyScript matches every pattern against every subject in `[[ ]]`
// and in a `case`, each in a subshell of its own so that a refusal is part of
// what both runs print rather than the end of the script, and then the
// counted patterns in the shell itself.
func patternPolicyScript() string {
	var b strings.Builder
	b.WriteString("WORDCHARS='*?_'; IFS=' :'\n")
	for i, p := range patternPolicyPatterns {
		fmt.Fprintf(&b, "p='%s'\n", p)
		for j, s := range patternPolicySubjects {
			fmt.Fprintf(&b, "s='%s'\n", s)
			fmt.Fprintf(&b, "( [[ $s == $p ]]; echo \"c%d.%d $?\" ) 2>&1\n", i, j)
			fmt.Fprintf(&b, "( case $s in $p) echo \"k%d.%d 1\";; *) echo \"k%d.%d 0\";; esac ) 2>&1\n", i, j, i, j)
		}
	}
	for i, p := range patternPolicyCounted {
		fmt.Fprintf(&b, "p='%s'\n", p)
		for j, s := range patternPolicySubjects {
			fmt.Fprintf(&b, "s='%s'; [[ $s == $p ]]; echo \"n%d.%d $?\"\n", s, i, j)
			fmt.Fprintf(&b, "case $s in $p) echo \"m%d.%d 1\";; *) echo \"m%d.%d 0\";; esac\n", i, j, i, j)
		}
	}
	b.WriteString("echo policy-end\n")
	return b.String()
}

// patternPolicyRun runs src under v with the route held on or off, and
// returns what it printed, its status and how many matches the route set up.
func patternPolicyRun(t *testing.T, src string, v patternPolicyVector, on bool) (string, int, int) {
	t.Helper()
	d := syntax.Core()
	if v.enable != nil {
		v.enable(&d)
	}
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sem := testSemantics()
	if v.sem != nil {
		v.sem(&sem)
	}
	var buf bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Env: testPATH(), Dialect: &d})
	if v.setup != nil {
		v.setup(r)
	}
	was := SetPatternPolicyRouteForTest(on)
	defer SetPatternPolicyRouteForTest(was)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error(), -1, PatternPolicyRoutedForTest(r)
	}
	return buf.String(), st, PatternPolicyRoutedForTest(r)
}

// TestThePatternPolicyRouteAnswersAsTheScansDo runs patternPolicyScript under
// each vector with the route that reads a pattern's text facts from its
// family on and off, and requires byte-identical output and status. It also
// requires the route to set up at least one match per counted row with it on
// and none with it off, and the script to have reached its end.
//
// The route is a package variable and these rows hold it, so they run one
// after another rather than side by side.
func TestThePatternPolicyRouteAnswersAsTheScansDo(t *testing.T) {
	src := patternPolicyScript()
	atLeast := 2 * len(patternPolicyCounted) * len(patternPolicySubjects)
	for _, v := range patternPolicyVectors {
		t.Run(v.name, func(t *testing.T) {
			full, fullStatus, fullRouted := patternPolicyRun(t, src, v, false)
			short, shortStatus, routed := patternPolicyRun(t, src, v, true)
			if short != full || shortStatus != fullStatus {
				t.Errorf("with the route:\n%s(status %d)\nwithout it:\n%s(status %d)", short, shortStatus, full, fullStatus)
			}
			if fullRouted != 0 {
				t.Errorf("the route set up %d matches with it off; want none", fullRouted)
			}
			if routed < atLeast {
				t.Errorf("the route set up %d matches; want at least %d", routed, atLeast)
			}
			if !strings.Contains(full, "policy-end\n") {
				t.Errorf("the script did not reach its end:\n%s", full)
			}
		})
	}
}

// TestThePatternPolicyScriptPutsEveryQuestion requires the patterns above to
// answer yes to every question the text facts hold, so the comparison is not
// one where a question was never put.
func TestThePatternPolicyScriptPutsEveryQuestion(t *testing.T) {
	t.Parallel()
	put := map[string]bool{}
	for _, p := range patternPolicyPatterns {
		for q, yes := range PatternTextFactsTrueForTest(p) {
			put[q] = put[q] || yes
		}
	}
	for q, yes := range put {
		if !yes {
			t.Errorf("no pattern answers %s yes", q)
		}
	}
	if len(put) == 0 {
		t.Fatal("the facts asked no questions")
	}
}

// TestPatternTextFactsAreTheScansOwn reads each pattern's text facts in one
// pass and requires every answer to be the one the scan it stands for gives,
// under each setting the scan takes.
func TestPatternTextFactsAreTheScansOwn(t *testing.T) {
	t.Parallel()
	patterns := append(slices.Clone(patternPolicyPatterns),
		``, `[`, `\`, `[\`, `[^`, `[:`, `[[:`, `[[:a:]`, `[[.`, `[[=`, `[]]`, `[!]]`, `[^]`, `(`, `((a)`,
		`\(a`, `a\[b`, `[a\]`, `[[:alpha:]]]`, `[x[:a:]y]`, `[[.].]]`, `(a|[b)`, `[(]`, `[a(]b)`,
		`a very long pattern [[:alpha:]] with ( an open group`, `and a long one with [ an open bracket too`,
		`é[^é]`, "[\xff]", `[a-]\]`, `*([a\]b]|c)`,
		// The two a random search found whose scans part on the setting: a
		// collating delimiter read only once `[]` closes, and an equivalence
		// class that leaves the bracket open only under the reading that has
		// them.
		`!^[][==\.`, `[[=:=]`)
	for _, p := range patterns {
		if diff := PatternTextFactsDisagreementForTest(p); diff != "" {
			t.Errorf("%q: %s", p, diff)
		}
	}
}

// thePatternPolicyTextReads are the calls in matchPatternChecked that are
// handed the pattern's text rather than its facts, as the route was checked
// against them. Each reads the text for something other than a policy's
// question — the shortcut that answers before the setup, the facts
// themselves, the character question that is about the subject too, the
// refusal, the record, and the match.
//
// When this test fails, a call that reads the pattern's text has been added
// to the setup or taken out of it. Decide whether the new call asks a
// question of the text alone; if it does, make it one of patternTextFacts and
// read it from t, and then update the list.
var thePatternPolicyTextReads = []string{
	"caseFoldReachesBeyondASCII", "fatalPattern", "matchPatternIn", "patternHasAnOperator",
	"patternMatchCountsCharacters", "plainPatternMatch", "policyText", "recordingPatternOpts",
	"tildeModifierOpts",
}

// TestThePatternPolicySetupReadsOnlyItsFacts fails when matchPatternChecked
// gains or loses a call that is handed the pattern's text. See
// thePatternPolicyTextReads.
func TestThePatternPolicySetupReadsOnlyItsFacts(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "semantics.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var body *ast.BlockStmt
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == "matchPatternChecked" {
			body = fn.Body
		}
	}
	if body == nil {
		t.Fatal("found no matchPatternChecked: the guard is looking in the wrong place")
	}
	calls := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		for _, a := range call.Args {
			if id, ok := a.(*ast.Ident); ok && id.Name == "pattern" {
				switch fn := call.Fun.(type) {
				case *ast.SelectorExpr:
					calls[fn.Sel.Name] = true
				case *ast.Ident:
					calls[fn.Name] = true
				}
			}
		}
		return true
	})
	var got []string
	for c := range calls {
		got = append(got, c)
	}
	slices.Sort(got)
	if len(got) == 0 {
		t.Fatal("nothing in matchPatternChecked is handed the pattern: the guard is looking in the wrong place")
	}
	added, removed := difference(got, thePatternPolicyTextReads), difference(thePatternPolicyTextReads, got)
	if len(added) > 0 || len(removed) > 0 {
		t.Errorf("matchPatternChecked now hands the pattern's text to [%s] and no longer to [%s]: read "+
			"interp/patternpolicy.go against the change, then update thePatternPolicyTextReads",
			strings.Join(added, ", "), strings.Join(removed, ", "))
	}
}
