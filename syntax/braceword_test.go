// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// The two reserved braces reaching into a word.
//
// One dialect recognizes `{` and `}` as reserved words by their *characters*
// rather than by the blanks around them, so `a(){print A}` is a function whose
// body is a brace group. The axes are [Dialect.OpenBraceNeedsNoBlank] and
// [Dialect.CloseBraceAlwaysReserved]; both hold the measurements.
//
// The rows are written as the *program* rather than as "it parsed", because
// parsing is the cheap half: what the token boundary does is decide which
// words a command has, and only printing the tree back says which.

// braceWords is the grammar these rows need, named by the rule.
func braceWords(d *Dialect) {
	d.OpenBraceNeedsNoBlank = true
	d.CloseBraceAlwaysReserved = true
	// A group closing on the same line as its last command has nothing
	// between them, which this dialect allows and the core does not.
	d.EmptyCompoundBody = true
}

// parsesTo asserts that src parses under d and prints back as want.
func parsesTo(t *testing.T, d Dialect, src, want string) {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	if got := strings.TrimRight(Print(f), "\n"); got != want {
		t.Errorf("%s\n got %q\nwant %q", src, got, want)
	}
}

// refuses asserts that src does not parse under d.
func refuses(t *testing.T, d Dialect, src string) {
	t.Helper()
	if _, err := Parse(src, d); err == nil {
		t.Errorf("%s: parsed, want a refusal", src)
	}
}

func TestAnOpenBraceNeedsNoBlankAfterIt(t *testing.T) {
	d := Core()
	braceWords(&d)
	for _, tc := range []struct{ name, src, want string }{
		{
			// The issue's own shape, and the shortest spelling of a
			// one-line function.
			"a function body with no blank at either end",
			"a(){print A}",
			"a() { print A; }",
		},
		{
			"a brace group standing on its own",
			"{print A}",
			"{ print A; }",
		},
		{
			// The `{` is the reserved word, so what follows it is a command
			// name rather than the first element of a brace expansion.
			"a word that would otherwise be a brace expansion",
			"{a,b}",
			"{ a,b; }",
		},
		{
			"several commands in one glued group",
			"{echo A; echo B}",
			"{ echo A; echo B; }",
		},
		{
			"a glued group in a pipeline",
			"{print A}|cat",
			"{ print A; } | cat",
		},
	} {
		t.Run(tc.name, func(t *testing.T) { parsesTo(t, d, tc.src, tc.want) })
	}
}

func TestAnOpenBraceIsOnlyReservedWhereACommandMayBegin(t *testing.T) {
	d := Core()
	braceWords(&d)
	for _, tc := range []struct{ name, src, want string }{
		{
			// Argument position keeps the brace in the word, which is why
			// the trailing one is what refuses `echo {print A}` — the group
			// never opened, so the `}` closes nothing.
			"an argument",
			"echo {print}",
			"echo {print}",
		},
		{
			"a redirection's target",
			"echo hi > {a}",
			"echo hi > {a}",
		},
		{
			"a `for` list",
			"for i in {a,b}; do echo $i; done",
			"for i in {a,b}; do echo $i; done",
		},
		{
			"a `case` subject",
			"case {a} in x) echo m;; esac",
			"case {a} in x) echo m ;; esac",
		},
	} {
		t.Run(tc.name, func(t *testing.T) { parsesTo(t, d, tc.src, tc.want) })
	}
}

func TestAQuotedOpenBraceOpensNoGroup(t *testing.T) {
	d := Core()
	braceWords(&d)
	// Quoting takes the reserved reading away, so the word is a command name
	// and the group never opens — which the trailing `}` then has nothing to
	// close.
	for _, src := range []string{`'{'print A}`, `\{print A}`, `"{"print A}`} {
		refuses(t, d, src)
	}
}

func TestACloseBraceEndsTheWordItStandsAtTheEndOf(t *testing.T) {
	d := Core()
	braceWords(&d)
	for _, tc := range []struct{ name, src, want string }{
		{
			"closing a group with no blank in front of it",
			"{ echo A}",
			"{ echo A; }",
		},
		{
			"closing a group from inside a function body",
			"f() { echo A}",
			"f() { echo A; }",
		},
		{
			// The brace ends the word wherever the word ends, so an operator
			// after it is as good as a blank.
			"closing a group in front of an operator",
			"{ echo A}&&echo B",
			"{ echo A; } && echo B",
		},
	} {
		t.Run(tc.name, func(t *testing.T) { parsesTo(t, d, tc.src, tc.want) })
	}
}

func TestACloseBraceInTheMiddleOfAWordIsText(t *testing.T) {
	d := Core()
	braceWords(&d)
	for _, tc := range []struct{ name, src, want string }{
		{"not at the end of the word", "echo a}b", "echo a}b"},
		{"at the front of one", "echo }a", "echo }a"},
		{"paired with a brace in the same word", "echo {a}", "echo {a}"},
		{"paired later in the same word", "echo a{b}", "echo a{b}"},
		{"paired across a run of text", "echo a}{b}", "echo a}{b}"},
		{"quoted", `echo A"}"`, `echo A"}"`},
		{"escaped", `echo A\}`, `echo A\}`},
	} {
		t.Run(tc.name, func(t *testing.T) { parsesTo(t, d, tc.src, tc.want) })
	}
}

func TestACloseBraceThatPairsWithNothingEndsTheWord(t *testing.T) {
	d := Core()
	braceWords(&d)
	// Each of these leaves a bare `}` standing where no group is open, which
	// is the refusal the reserved reading produces. Written as refusals
	// because that is what the shell answers: the pairing is only visible
	// where it *fails*.
	for _, src := range []string{
		"echo A}",
		"echo A} B",
		"echo A}|cat",
		`echo "A"}`,
		"echo {a}}",
		"echo a{b}c}",
		"echo ${x}}", // an expansion's braces are its own and pair with nothing
		"echo x=}",   // an argument, so the assignment carve-out does not apply
	} {
		refuses(t, d, src)
	}
}

func TestAnAssignmentValueKeepsItsCloseBrace(t *testing.T) {
	d := Core()
	braceWords(&d)
	for _, tc := range []struct{ name, src, want string }{
		{"a value ending in a brace", "x=a}", "x=a}"},
		{"a value that is only a brace", "x=}", "x=}"},
		{"an appending assignment", "x+=a}", "x+=a}"},
	} {
		t.Run(tc.name, func(t *testing.T) { parsesTo(t, d, tc.src, tc.want) })
	}
}

func TestTheCoreLeavesBothBracesInTheWord(t *testing.T) {
	// Without the flags the braces are ordinary characters, which is what
	// every other shell in the panel does: `a(){print A}` names a command
	// `{print` there.
	d := Core()
	parsesTo(t, d, "echo A}", "echo A}")
	parsesTo(t, d, "echo {print A}", "echo {print A}")
	parsesTo(t, d, "{print A}", "{print A}")
}
