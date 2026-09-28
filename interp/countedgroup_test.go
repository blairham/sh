// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// countedFixture is the directory every row below is measured in: `a`, `aa`,
// `aaa` and `aaaa`, which is #4927's panel. Four names rather than one
// because the construct is a *count* — one repetition too few and one too
// many are what separate `{2,3}(a)` from `+(a)`, and a fixture holding only
// `aa` would pass for both.
func countedFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"a", "aa", "aaa", "aaaa"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// countedWords runs one snippet in that directory with the field counter in
// front of it, under a grammar that has quantified groups *and* a count in
// front of one.
//
// The brace axes are pinned rather than left to a refusal, for the reason
// braceProduced pins them: every row here is about a brace, and a row that
// came out right because an axis went unanswered would be measuring the
// refusal.
func countedWords(t *testing.T, dir, src string) string {
	t.Helper()
	out, _ := runGrammar(t, emptyAltCounter+src+"\n", func(d *syntax.Dialect) {
		d.ExtendedPattern = true
		d.ExtendedPatternInCondition = true
		d.CountedPatternGroup = true
	}, func(r *Runner) {
		r.Dir = dir
		s := *r.Semantics
		s.BraceExpansion = Yes
		s.BraceStopsFieldSplitting = Yes
		s.BraceEmptyAlternativeIsAField = Yes
		s.BraceRescanEntersFailedGroup = No
		s.BraceOutputRereadAsText = Yes
		s.BraceScanReadsProducedText = Yes
		s.BraceBodyReadAfterExpansion = Yes
		s.BraceFanExpandsEachNameOnItsOwn = No
		s.BraceRangeEndpointsExpanded = Yes
		// Two rows hand a parenthesis to the pattern from a value, and
		// what a shell does with one is an axis of its own. Pinned to the
		// column these rows were measured in, so that a refusal cannot be
		// mistaken for the brace rule answering.
		s.ExpansionResultSuppliesGroupSyntax = No
		s.BraceFreesProducedGroupSyntax = No
		s.BraceMakesAProducedStarOrBracketText = No
		r.Semantics = &s
	})
	return strings.TrimSpace(out)
}

// The ten rows of #4927, as they stand in a word.
//
// Measured 2026-09-27 against `/bin/ksh` `Version AJM 93u+ 2012-08-01` —
// `go version -m` says *not a Go executable* — with each case run under
// `env -i PATH=/usr/bin:/bin` in a directory of its own holding the four
// names above, created by the shell under test.
//
// **The first row is the control and it is not decoration.** It is a brace
// list in the same position with no `(` behind it, and it expands. A row
// under it that does not move is therefore the parenthesis deciding, rather
// than the fixture being absent or this helper being dead.
func TestACountInFrontOfAGroupIsNotABraceList(t *testing.T) {
	dir := countedFixture(t)
	for _, tc := range []struct{ name, src, want string }{
		{"the control", `f {z,y}a`, `2 | [za] [ya]`},

		// The count, in its four spellings.
		{"two or three", `f {2,3}(a)`, `2 | [aa] [aaa]`},
		{"exactly two", `f {2}(a)`, `1 | [aa]`},
		{"two or more", `f {2,}(a)`, `3 | [aa] [aaa] [aaaa]`},
		{"at most two", `f {,2}(a)`, `2 | [a] [aa]`},
		{"nought or one", `f {0,1}(a)`, `1 | [a]`},
		{"an alternation inside", `f {2,3}(a|b)`, `2 | [aa] [aaa]`},

		// #4933's row: the brace is suppressed by what follows the `}` and
		// not by its contents, so a brace whose contents are not a count at
		// all is still not a list. `{z,y}` is the control's own list one
		// character short of this, and here it does not expand.
		{"a count that is not one", `f {z,y}(a)`, `1 | [{z,y}(a)]`},

		// Which brace, and which parenthesis.
		{"a produced brace is a list", `g='{2,3}(a)'; f $g`, `2 | [2(a)] [3(a)]`},
		{"a quoted parenthesis", `f {z,y}"(a)"`, `2 | [z(a)] [y(a)]`},
		{"an escaped parenthesis", `f {2,3}\(a\)`, `2 | [2(a)] [3(a)]`},
		{"a produced parenthesis is not the next character", `g='a(b'; f {z,y}$g`, `2 | [za(b] [ya(b]`},
		{"only the brace in front of it", `f {2,3}(a){x,y}`, `2 | [{2,3}(a)x] [{2,3}(a)y]`},
		{"and a brace with no group at all", `f {2,3}`, `2 | [2] [3]`},
		{"nor one in front of a quantifier", `f {1,2}@(a)`, `2 | [1@(a)] [2@(a)]`},

		// The count is read where the group stands rather than at the front
		// of the word, and a group that matches nothing leaves the word.
		{"mid-word", `f x{2,3}(a)`, `1 | [x{2,3}(a)]`},
		{"with a literal behind it", `f {2,3}(a)b`, `1 | [{2,3}(a)b]`},
		{"and a second group behind it", `f {2}(a)@(a)`, `1 | [aaa]`},
		{"and a second count behind it", `f {2}(a){2}(a)`, `1 | [aaaa]`},
	} {
		if got := countedWords(t, dir, tc.src); got != tc.want {
			t.Errorf("%s: %s = %q, want %q", tc.name, tc.src, got, tc.want)
		}
	}
}

// The same operator in a condition and a `case` arm, where there is no brace
// expansion at all — so these rows are the *matcher's* alone and the ones
// above are the matcher and the brace scan together.
func TestACountedGroupIsTheSameOperatorInEverySurface(t *testing.T) {
	for _, tc := range []struct{ subject, pattern, want string }{
		// Three is inside the count, one is too few and four too many. A row
		// that only asked whether `aaa` matched would pass for `+(a)`.
		{"aa", "{2,3}(a)", "yes"},
		{"aaa", "{2,3}(a)", "yes"},
		{"a", "{2,3}(a)", "no"},
		{"aaaa", "{2,3}(a)", "no"},
		// Mid-pattern, so it is not a prefix on the word.
		{"aaa", "a{1,2}(a)", "yes"},
		// A group of more than one character, and an alternation in it.
		{"abab", "{2}(ab)", "yes"},
		{"abab", "{2}(a|ab|b)", "yes"},
		// Both ends omitted is nought and no ceiling — `*(a)` written with a
		// brace — and a lower bound of nought with a ceiling of nought lets
		// the group stand for no text and nothing else.
		{"", "{,}(a)", "yes"},
		{"aaaaa", "{,}(a)", "yes"},
		{"", "{0,0}(a)", "yes"},
		{"a", "{0,0}(a)", "no"},
		{"", "{,0}(a)", "yes"},
		// A ceiling under the floor is satisfied by no number of
		// repetitions, and the answer is that nothing matches rather than
		// that the pattern is refused.
		{"a", "{3,2}(a)", "no"},
		{"aa", "{3,2}(a)", "no"},
		{"aaa", "{3,2}(a)", "no"},
		// A count that is not a count: the braces and the parentheses are
		// characters, and the text matches itself.
		{"{z,y}(a)", "{z,y}(a)", "yes"},
		{"{z,y}a", "{z,y}(a)", "no"},
		{"{z,y}(a|b)", "{z,y}(a|b)", "yes"},
		{"{z,y}(a)", "{z,y}(a|b)", "no"},
		{"{a}(b)", "{a}(b)", "yes"},
		{"{-1,2}(a)", "{-1,2}(a)", "yes"},
		{"{2,3,4}(a)", "{2,3,4}(a)", "yes"},
		// **And it is not quoted**, which is the row that separates "the
		// parentheses stop being syntax" from "the run goes literal": a
		// wildcard inside them is still a wildcard, and a *quantified* group
		// inside them is still a group, while a bare one is text.
		{"{z,y}(ab)", "{z,y}(a?)", "yes"},
		{"{z,y}(ab)", "{z,y}(a*)", "yes"},
		{"{z,y}(abb)", "{z,y}(a+(b))", "yes"},
		{"{z,y}(a(b))", "{z,y}(a(b))", "yes"},
		{"{z,y}(ab)", "{z,y}(a(b))", "no"},
		// Inside a group, though, a bare parenthesis is a group again — the
		// same answer `@(a|(b))` gives — so the rule above is about the
		// suppressed parentheses and not about the dialect.
		{"ab", "{1}(a(b))", "yes"},
		{"b", "{1}(a|(b))", "yes"},
		// A leading zero is a digit like any other, and a sign is not.
		{"aa", "{02,3}(a)", "yes"},
		{"aa", "{+2,3}(a)", "no"},
		// The ceiling is read into a signed 32-bit integer there, and one
		// past it is not a count at all.
		{"aa", "{2,2147483647}(a)", "yes"},
		{"aa", "{2,2147483648}(a)", "no"},
	} {
		for _, surface := range []string{"case", "condition"} {
			if got := countedMatch(t, surface, tc.subject, tc.pattern); got != tc.want {
				t.Errorf("%s: %q against %s = %s, want %s",
					surface, tc.subject, tc.pattern, got, tc.want)
			}
		}
	}
}

// countedMatch asks one surface whether a subject matches a pattern.
//
// Two surfaces rather than one because #4935's argument is that the operator
// belongs to the pattern *language*: a row that only ever asked a `case` arm
// would pass for a reading wired into one reader.
func countedMatch(t *testing.T, surface, subject, pattern string) string {
	t.Helper()
	src := `case "` + subject + `" in ` + pattern + ") echo yes;; *) echo no;; esac"
	if surface == "condition" {
		src = `[[ "` + subject + `" == ` + pattern + ` ]] && echo yes || echo no`
	}
	out, _ := runGrammar(t, src+"\n", func(d *syntax.Dialect) {
		d.ExtendedPattern = true
		d.ExtendedPatternInCondition = true
		d.CountedPatternGroup = true
		d.DoubleBracket = true
	}, nil)
	return strings.TrimSpace(out)
}

// Without the flag nothing here is a count, and the grammar is what says so
// first: the parenthesis is not let into the word at all.
//
// The **positive** half is what makes this worth writing. A dialect with the
// flag off refusing `{2,3}(a)` proves only that something refused it, and a
// helper that refused every source would say the same — so the row above it
// runs the identical text with the flag on and requires the match.
func TestACountIsOneDialectsAlone(t *testing.T) {
	dir := countedFixture(t)
	if got := countedWords(t, dir, `f {2,3}(a)`); got != `2 | [aa] [aaa]` {
		t.Fatalf("with the flag: %q, want %q", got, `2 | [aa] [aaa]`)
	}
	d := syntax.Core()
	d.ExtendedPattern = true
	if _, err := syntax.Parse("f {2,3}(a)\n", d); err == nil {
		t.Error("without the flag: parsed, want the parenthesis reported unexpected")
	} else if !strings.Contains(err.Error(), "unexpected") {
		t.Errorf("without the flag: %v, want the parenthesis reported unexpected", err)
	}
}
