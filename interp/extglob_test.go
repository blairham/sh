// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// matchWith runs a `case` under a dialect with the given pattern grammar,
// naming the flags rather than a shell.
func matchWith(t *testing.T, subject, pattern string, extended, alternation bool) string {
	t.Helper()
	d := syntax.Core()
	d.ExtendedPattern, d.PatternAlternation = extended, alternation
	// The subject is quoted so that an empty one is still a word: `case  in`
	// is a syntax error, and quoting changes nothing about the match.
	src := `case "` + subject + `" in ` + pattern + ") echo yes;; *) echo no;; esac"
	f, err := syntax.Parse(src, d)
	if err != nil {
		return "parse: " + err.Error()
	}
	var out bytes.Buffer
	s := PosixSemantics()
	r := &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String())
}

// The five quantifiers, each against something it should and should not match.
func TestExtendedPatternQuantifiers(t *testing.T) {
	for _, tc := range []struct{ subject, pattern, want string }{
		// Exactly one of the arms.
		{"abc", "@(abc|xyz)", "yes"},
		{"xyz", "@(abc|xyz)", "yes"},
		{"q", "@(abc|xyz)", "no"},
		{"abcabc", "@(abc)", "no"},
		// Zero or one.
		{"abc", "?(abc)", "yes"},
		{"", "?(abc)", "yes"},
		{"abcabc", "?(abc)", "no"},
		// One or more.
		{"a", "+(a)", "yes"},
		{"aaa", "+(a)", "yes"},
		{"", "+(a)", "no"},
		{"ab", "+(a|b)", "yes"},
		// Zero or more.
		{"", "*(a)", "yes"},
		{"aaa", "*(a)", "yes"},
		{"b", "*(a)", "no"},
		// Anything the arms do not match.
		{"b", "!(a)", "yes"},
		{"a", "!(a)", "no"},
		// A group is part of a larger pattern, not the whole of it — which is
		// what makes the repeating quantifiers need to try more than one
		// stopping point.
		{"aab", "+(a)b", "yes"},
		{"xabcy", "x@(abc|q)y", "yes"},
		{"xy", "x*(abc)y", "yes"},
		// Nested groups, and a `|` inside one that is not a separator of the
		// outer.
		{"abd", "@(a@(b|c)d)", "yes"},
		{"acd", "@(a@(b|c)d)", "yes"},
		{"aed", "@(a@(b|c)d)", "no"},
		// The other metacharacters still work inside an arm.
		{"axc", "@(a?c|q)", "yes"},
		{"abbbc", "@(a*c)", "yes"},
	} {
		if got := matchWith(t, tc.subject, tc.pattern, true, false); got != tc.want {
			t.Errorf("%s against %s = %s, want %s", tc.subject, tc.pattern, got, tc.want)
		}
	}
}

// A bare group is the other reading of the same text, and the two are not
// compatible: `@(abc|xyz)` matches `abc` under one and `@abc` under the other.
func TestABareGroupIsADifferentReading(t *testing.T) {
	for _, tc := range []struct {
		subject, pattern           string
		extended, alternationWants string
	}{
		{"abc", "@(abc|xyz)", "yes", "no"},
		{"@abc", "@(abc|xyz)", "no", "yes"},
		{"ab", "a(b|c)", "parse", "yes"},
		{"ac", "a(b|c)", "parse", "yes"},
		{"ad", "a(b|c)", "parse", "no"},
	} {
		got := matchWith(t, tc.subject, tc.pattern, true, false)
		if tc.extended == "parse" {
			if !strings.HasPrefix(got, "parse:") {
				t.Errorf("%s against %s: got %q, want a parse failure without bare groups", tc.subject, tc.pattern, got)
			}
		} else if got != tc.extended {
			t.Errorf("extended: %s against %s = %s, want %s", tc.subject, tc.pattern, got, tc.extended)
		}
		if got := matchWith(t, tc.subject, tc.pattern, false, true); got != tc.alternationWants {
			t.Errorf("alternation: %s against %s = %s, want %s", tc.subject, tc.pattern, got, tc.alternationWants)
		}
	}
}

// Without either flag the parentheses are not a group at all, and the word
// does not even reach the matcher.
func TestWithoutAFlagAGroupIsNotOne(t *testing.T) {
	for _, pattern := range []string{"@(abc|xyz)", "a(b|c)", "+(a)"} {
		got := matchWith(t, "abc", pattern, false, false)
		if !strings.HasPrefix(got, "parse:") {
			t.Errorf("%s: got %q, want a parse failure", pattern, got)
		}
	}
}

// runWith executes src under a dialect with the given pattern grammar. These
// are behavior rather than parsing: the lexer's exceptions all *parse* either
// way, and only what they produce tells them apart.
func runWith(t *testing.T, src string, extended, alternation bool) string {
	t.Helper()
	d := syntax.Core()
	d.ExtendedPattern, d.PatternAlternation = extended, alternation
	f, err := syntax.Parse(src, d)
	if err != nil {
		return "parse: " + err.Error()
	}
	var out bytes.Buffer
	s := PosixSemantics()
	r := &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String())
}

// Three things a group must not swallow. Each of them still parses when the
// rule is too wide, which is why none of these can be a parsing test: the
// difference is only in what the word turns out to be.
func TestAGroupDoesNotSwallowWhatIsNotOne(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// A `(` that starts a word opens a subshell. Absorbed into the word,
		// `(echo hi)` becomes a command with that name.
		{"a subshell", `(echo hi)`, "hi"},
		// An empty `()` is a function definition.
		{"a function definition", `f() { echo hi; }; f`, "hi"},
		// A `(` straight after `=` opens an array literal. Absorbed, the
		// value becomes the text `(x y)`.
		{"an array literal", `a=(x y); echo "${a[@]}" ${#a[@]}`, "x y 2"},
		{"an array literal in a function", `f() { a=(x y); echo "${a[0]}"; }; f`, "x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runWith(t, tc.src, false, true); got != tc.want {
				t.Errorf("with bare groups: got %q, want %q", got, tc.want)
			}
			// And the same under the other flag, and under neither.
			if got := runWith(t, tc.src, true, false); got != tc.want {
				t.Errorf("with quantified groups: got %q, want %q", got, tc.want)
			}
			if got := runWith(t, tc.src, false, false); got != tc.want {
				t.Errorf("with no groups at all: got %q, want %q", got, tc.want)
			}
		})
	}
}

// A group nested inside a group needs no quantifier of its own, even in the
// dialect that requires one at the top level: `@(a|(b))` matches b there,
// while `a(b|c)` on its own is a syntax error. The lexer refuses the second,
// so by the time text reaches the matcher a bare paren came from somewhere the
// dialect allows.
func TestANestedGroupNeedsNoQuantifier(t *testing.T) {
	for _, tc := range []struct{ subject, pattern, want string }{
		{"b", "@(a|(b))", "yes"},
		{"a", "@(a|(b))", "yes"},
		{"(b)", "@(a|(b))", "no"},
		{"c", "@(a|(b))", "no"},
	} {
		if got := matchWith(t, tc.subject, tc.pattern, true, false); got != tc.want {
			t.Errorf("%s against %s = %s, want %s", tc.subject, tc.pattern, got, tc.want)
		}
	}
}

// condRun evaluates a `[[ ]]` under the given pattern grammar, so the two
// contexts can be compared with the same flags.
func condRun(t *testing.T, src string, extended, condOnly, alternation bool) string {
	t.Helper()
	d := syntax.Core()
	d.ExtendedPattern, d.ExtendedPatternInCondition, d.PatternAlternation = extended, condOnly, alternation
	f, err := syntax.Parse(src, d)
	if err != nil {
		return "parse: " + err.Error()
	}
	var out bytes.Buffer
	s := PosixSemantics()
	s.GlobExpansionResults = Yes
	r := &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String())
}

// Where the pattern stands decides how its parentheses are read, and it
// cannot be folded into "which shell this is".
//
// The argument for folding was that the lexer only produces a group where the
// dialect allows one, so reading them wherever they arrive is the same answer.
// That holds for text the lexer saw. It does not hold for text an *expansion*
// supplies, which reaches the matcher without passing the lexer at all.
func TestGroupsAreReadByWhereThePatternStands(t *testing.T) {
	const inCase = `p="(b)"; case b in $p) echo yes;; *) echo no;; esac`
	const inCond = `p="@(b|c)"; [[ b == $p ]] && echo yes || echo no`

	// Groups everywhere: an expanded group is one in both places.
	if got := condRun(t, inCase, true, true, false); got != "yes" {
		t.Errorf("groups everywhere, in a case: got %s, want yes", got)
	}
	// Groups in a condition only: the same text is literal in a case.
	if got := condRun(t, inCase, false, true, false); got != "no" {
		t.Errorf("groups in a condition only, in a case: got %s, want no", got)
	}
	if got := condRun(t, inCond, false, true, false); got != "yes" {
		t.Errorf("groups in a condition only, in a condition: got %s, want yes", got)
	}
	// No groups at all: literal in both.
	if got := condRun(t, inCase, false, false, false); got != "no" {
		t.Errorf("no groups, in a case: got %s, want no", got)
	}
	if got := condRun(t, inCond, false, false, false); got != "no" {
		t.Errorf("no groups, in a condition: got %s, want no", got)
	}
	// And the subject that the literal reading matches, which is the half
	// that shows the parentheses really were escaped rather than dropped.
	const literal = `p="(b)"; case "(b)" in $p) echo yes;; *) echo no;; esac`
	if got := condRun(t, literal, false, false, false); got != "yes" {
		t.Errorf("no groups, literal subject: got %s, want yes", got)
	}
}
