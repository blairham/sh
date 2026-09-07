// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s})
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
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s})
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
	return condRunGlob(t, src, extended, condOnly, alternation, Yes)
}

// condRunGlob is condRun with the axis that decides whether an expansion's
// result is a pattern at all, which is the other half of how parentheses from
// a variable are read.
func condRunGlob(t *testing.T, src string, extended, condOnly, alternation bool, globs Answer) string {
	t.Helper()
	d := syntax.Core()
	d.ExtendedPattern, d.ExtendedPatternInCondition, d.PatternAlternation = extended, condOnly, alternation
	f, err := syntax.Parse(src, d)
	if err != nil {
		return "parse: " + err.Error()
	}
	var out bytes.Buffer
	s := PosixSemantics()
	s.GlobExpansionResults = globs
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s})
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

// A shell that does not re-read an expansion as a pattern escapes what it
// produced, and the parentheses have to be in that set where they are
// metacharacters — otherwise a `(b)` from a variable becomes a group in the
// one shell that has bare groups and does *not* re-read expansions.
func TestParenthesesFromAnExpansionAreEscaped(t *testing.T) {
	const asGroup = `p="(b)"; case b in $p) echo yes;; *) echo no;; esac`
	const asText = `p="(b)"; case "(b)" in $p) echo yes;; *) echo no;; esac`

	// Bare groups, and an expansion's result is not a pattern: literal.
	if got := condRunGlob(t, asGroup, false, false, true, No); got != "no" {
		t.Errorf("got %s, want no — the parentheses came from a variable", got)
	}
	if got := condRunGlob(t, asText, false, false, true, No); got != "yes" {
		t.Errorf("got %s, want yes — escaped, not dropped", got)
	}
	// The same shell where an expansion's result *is* a pattern reads the
	// group, which is what keeps this about the escaping and not about
	// groups being switched off.
	if got := condRunGlob(t, asGroup, false, false, true, Yes); got != "yes" {
		t.Errorf("got %s, want yes — the expansion is a pattern here", got)
	}
	// And a shell with no groups leaves them alone either way.
	if got := condRunGlob(t, asText, false, false, false, No); got != "yes" {
		t.Errorf("got %s, want yes", got)
	}
}

// A quantified group reaches the **filesystem**, which it did not before
// #1042: `echo @(a|b)` was passed through as text where every shell with the
// construct lists the files.
//
// The gap was in the predicate that decides whether a field is a pattern at
// all. It counts `*`, `?`, a closed `[`, a range where the dialect has one
// and — since #995 — a bare `(` where the dialect has those; it did not count
// a *quantified* group, so a field holding one and nothing else never reached
// the walk. `*(a|b)` and `?(a|b)` worked all along and hid it, their
// quantifier being a metacharacter in its own right, which is why the gap
// showed up as three of the five quantifiers rather than as the construct.
//
// Measured 2026-09-07 in a directory holding `a` and `b`, on ksh93u+
// 2012-08-01 and on bash 5.3.15 and 3.2.57 under `shopt -s extglob`. All
// three answer `a b` to `@(a|b)`, `+(a|b)`, `?(a|b)` and `*(a|b)`, and `b` to
// `!(a)`.
func TestAQuantifiedGroupReachesTheFilesystem(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ pattern, want string }{
		// The three that were passed through as text.
		{`@(a|b)`, `[a][b]`},
		{`+(a|b)`, `[a][b]`},
		{`!(a)`, `[b]`},
		// And the two whose quantifier was already a metacharacter, which
		// is what made the gap invisible.
		{`?(a|b)`, `[a][b]`},
		{`*(a|b)`, `[a][b]`},
		// A group behind a literal is the same construct in a different
		// place, and matches nothing here — `a@(x|y)` names no file, so the
		// word is passed through, which is ksh93's answer too.
		{`a@(x|y)`, `[a@(x|y)]`},
		// A quantifier with no group behind it is an ordinary character:
		// `@x` is a file name, not a pattern, in every shell in the panel.
		{`@a`, `[@a]`},
	} {
		got := runExtendedIn(t, dir, `printf "[%s]" `+tc.pattern)
		if got != tc.want {
			t.Errorf("%s = %q, want %q", tc.pattern, got, tc.want)
		}
	}
	// Without the flag the field is not a pattern at all and the text
	// stands, which is the same answer read the other way round: the
	// predicate reads the dialect and not the characters.
	if got := runPlainIn(t, dir, `printf "[%s]" @a`); got != `[@a]` {
		t.Errorf("without the flag: got %q, want %q", got, `[@a]`)
	}
}

// runExtendedIn runs src in dir under a dialect with quantified groups, which
// is the flag the rows above are about.
func runExtendedIn(t *testing.T, dir, src string) string {
	t.Helper()
	return runGlobIn(t, dir, src, true)
}

func runPlainIn(t *testing.T, dir, src string) string {
	t.Helper()
	return runGlobIn(t, dir, src, false)
}

func runGlobIn(t *testing.T, dir, src string, extended bool) string {
	t.Helper()
	d := syntax.Core()
	d.ExtendedPattern = extended
	f, err := syntax.Parse(src, d)
	if err != nil {
		return "parse: " + err.Error()
	}
	var out bytes.Buffer
	s := PosixSemantics()
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dir: dir, Dialect: &d, Semantics: &s})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String())
}

// The leading-period rule is **not** suspended by a quantifier, and this is
// the row the issue expected to go the other way.
//
// #1042 read `*(.)` answering `.` and `..` as evidence that the quantifier is
// exempt from the rule "in both other shells". Measured 2026-09-07, it is one
// shell and not two: in a directory holding `a` and `b`,
//
//	ksh93u+          echo *(.)   →  . ..
//	bash 5.3.15      echo *(.)   →  *(.)   — no match, passed through
//	bash 3.2.57      echo *(.)   →  *(.)   — the same
//
// A pattern's leading character here is the `*`, not a literal period, so a
// hidden name is not matched — and `.` and `..` are not entries this walk
// offers at all. bash agrees on both counts; ksh93 offers them and is the
// divergence `pat/a-trailing-group-is-a-list-of-qualifiers` records in its
// column, for that reason rather than for the one the issue supposed.
//
// So this asserts what the fix does *not* change, which is the half a
// predicate about metacharacters has no business deciding.
func TestAQuantifierDoesNotSuspendTheLeadingPeriodRule(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a", ".hid"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ pattern, want string }{
		// The pattern reaches the walk now — that is the fix — and matches
		// no hidden name, so the word is passed through.
		{`*(.)`, `[*(.)]`},
		{`@(.)`, `[@(.)]`},
		// A group whose own first character is a literal period is a
		// **second axis** and is not this fix's. Two answers, and they
		// split a shell rather than two shells — measured 2026-09-07 in a
		// directory holding `a` and `.hid`:
		//
		//	                  bash 5.3.15  bash 3.2.57  ksh93u+
		//	echo @(.hid)      .hid         @(.hid)      .hid
		//	echo @(a|.hid)    .hid a       a            .hid a
		//	echo *(.hid)      .hid         *(.hid)      .hid
		//	echo .@(hid)      .hid         .hid         .hid
		//
		// So bash 5.3 and ksh93 look *inside* the group for a literal
		// period and any alternative will do, where bash 3.2 does not —
		// and the last row, whose period is outside the group, is the one
		// all three agree on. A version difference inside one preset is
		// the shape `${x^^}` has and it is not answerable by a grammar
		// flag, so it is measured here and left as it stands rather than
		// guessed at. This answers as bash 3.2 does.
		{`@(.hid)`, `[@(.hid)]`},
		{`@(a|.hid)`, `[a]`},
		// The period outside the group is the row every shell agrees on,
		// and it works here.
		{`.@(hid)`, `[.hid]`},
	} {
		if got := runExtendedIn(t, dir, `printf "[%s]" `+tc.pattern); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.pattern, got, tc.want)
		}
	}
}

// A quantified group is a metacharacter at the **expansion** sites too, which
// is the other half of one predicate read twice: a field with a metacharacter
// in it is escaped where the dialect does not glob the result of an expansion,
// and one with none is left alone because it has nothing to protect.
//
// It needs a dialect no preset is — quantified groups *and* a refusal to glob
// an expansion's result — because the two shells with the construct both glob
// such a result and the shell that does not has no quantified groups. That is
// a fact about today's panel and not about the code: the two questions are
// independent, the sites read both, and a mutant that dropped the flag at
// either of them survived every row until this one.
//
// What it asserts is the *escaping*: with the flag, `@(a|b)` arriving from a
// value is protected and prints as six characters; without it the same field
// is thought to hold nothing worth protecting.
func TestAQuantifiedGroupFromAnExpansionIsEscaped(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(src string) string {
		t.Helper()
		d := syntax.Core()
		d.ExtendedPattern = true
		f, err := syntax.Parse(src, d)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		var out bytes.Buffer
		s := PosixSemantics()
		// The axis no preset combines with the flag above.
		s.GlobExpansionResults = No
		r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dir: dir, Dialect: &d, Semantics: &s})
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(out.String())
	}
	// The scalar site: an unquoted `$p` whose value is a group.
	if got, want := run(`p='@(a|b)'; printf "[%s]" $p`), `[@(a|b)]`; got != want {
		t.Errorf("scalar: got %q, want %q", got, want)
	}
	// And the list site, which is a second call with its own flags: an
	// array's elements go through a different path from a scalar's value.
	if got, want := run(`set -- '@(a|b)'; printf "[%s]" $@`), `[@(a|b)]`; got != want {
		t.Errorf("list: got %q, want %q", got, want)
	}
	// The row that says the escaping is what did it rather than the value
	// simply never being a pattern: written in the source, the same six
	// characters do reach the filesystem.
	if got, want := run(`printf "[%s]" @(a|b)`), `[a][b]`; got != want {
		t.Errorf("written out: got %q, want %q", got, want)
	}
}
