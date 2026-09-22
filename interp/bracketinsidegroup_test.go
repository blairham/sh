// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/syntax"

	. "github.com/blairham/sh/interp"
)

// A bracket expression inside a group (#3075).
//
// Five scans walk a pattern counting parentheses, and three of them did not
// know a bracket expression is one member after another rather than
// structure. So a `|` between two members split the alternation, and a `(`
// or a `)` between two members moved the depth the split was counted at —
// which is what made it invisible, since in `([][()|*?^#~<>])` the
// parenthesis pair inside the bracket balances and the counter is back at
// nought when the bar arrives.
//
// The first arm then carried an unterminated `[` and the whole pattern was
// refused. It was the largest thing between this shell and a shipped
// completion system: the function most of that tree reaches builds exactly
// that pattern, and the refusal is *printed*, so it scribbled over the line
// a person was typing.
//
// Measured 2026-09-15 with `[[ $s == @($p) ]]`, each probe in a script file
// of its own — bash 5.3.20 and bash 3.2.57 with `extglob` set, ksh93u+
// 2012-08-01, and zsh 5.9.2 with `kshglob`:
//
//	| probe                  | bash 5.3 | bash 3.2 | ksh93u+ | zsh 5.9.2 |
//	| `a` vs `@([a\|b])`     | match    | match    | match   | match     |
//	| `\|` vs `@([a\|b])`    | match    | match    | match   | match     |
//	| `\|` vs `@([]\|])`     | match    | match    | match   | match     |
//	| `a` vs `@([ab])`       | match    | match    | match   | match     |
//	| `a` vs `@(x\|a)`       | match    | match    | match   | match     |
//
// dash and BusyBox ash have neither the group nor `[[ ]]`, so they cannot be
// asked. Four columns agree with each other and with the fifth reading zsh's
// own bare-group spelling, and none of them splits at that bar — **a plain
// bug, not an axis.** Nothing is added to Semantics or to Dialect.
//
// The rows below are written as matches and non-matches rather than as "the
// pattern was accepted", because acceptance is the cheap half: a reading that
// took the bracket and then forgot its members would accept every row here
// and match the wrong subjects.

// bracketGroupGrammar is the bare-group alternation these rows are written
// in, which is one dialect's spelling of the quantified group the rest of the
// panel writes `@( … )`, plus the flag that turns an expansion's result into
// a pattern so a probe can carry its pattern in a variable.
func bracketGroupGrammar(d *syntax.Dialect) {
	d.PatternAlternation = true
	d.ParamTildeFlag = true
}

// runBracketGroup runs one probe with that grammar and the two answers the
// rows need, neither of which is the subject: an expansion's result may
// supply the syntax of a group — without it the pattern arrives as text and
// every row matches nothing — and an unterminated `[` is a bad pattern,
// which is the answer the one row written with one is measured against.
func runBracketGroup(t *testing.T, subject, pattern string) (string, int) {
	t.Helper()
	src := `s=` + shellQuote(subject) + `; p=` + shellQuote(pattern) +
		`; if [[ $s == ${~p} ]]; then printf M; else printf N; fi`
	return runGrammar(t, src, bracketGroupGrammar, func(r *Runner) {
		sem := *r.Semantics
		sem.ExpansionResultSuppliesGroupSyntax = Yes
		sem.UnterminatedBracket = BracketLiteral
		r.Semantics = &sem
	})
}

func TestABracketExpressionInsideAGroupIsOneMember(t *testing.T) {
	for _, tc := range []struct {
		name, subject, pattern string
		want                   bool
	}{
		{
			// The issue's own pattern, reduced to what it is about: the
			// members are every character a completion has to quote, and
			// two of them are the bar and the parentheses.
			"a bar between two members", "a", "([a|b])", true,
		},
		{
			// The bar is a member, so it matches as one.
			"the bar itself", "|", "([a|b])", true,
		},
		{
			// A `]` first is a literal member, and the bar behind it is
			// still inside the bracket.
			"a bar behind a literal close bracket", "|", "([]|])", true,
		},
		{
			// The parentheses that made it invisible: they balance inside
			// the bracket, so a depth counter alone reads the bar as
			// top-level.
			"parentheses among the members", "(", "([][()|*?^#~<>])", true,
		},
		{"and the bar among those members", "|", "([][()|*?^#~<>])", true},
		{
			// A member that is not in the bracket still does not match,
			// which is what says the bracket was read rather than skipped.
			"a subject outside the bracket", "z", "([a|b])", false,
		},
		{
			// A bracket followed by more pattern inside one arm: the arm is
			// `[a|b]x` and the whole of it has to be one arm.
			"a bracket at the front of an arm", "ax", "([a|b]x|y)", true,
		},
		{"the other arm of the same pattern", "y", "([a|b]x|y)", true},
		{
			// And the arm is not `b]x`, which is what a wrong split would
			// have left behind.
			"the wrong split's arm", "b]x", "([a|b]x|y)", false,
		},
		{
			// The bracket in the second arm rather than the first.
			"a bracket in a later arm", "a", "(x|[a|b])", true,
		},
		{
			// An *unterminated* `[` reaches the end of the pattern, so the
			// `)` behind it is inside the bracket's reach and closes
			// nothing: this is not a group at all, and the text is the six
			// ordinary characters it is written as. Measured 2026-09-22 on
			// bash 5.3.20 with `extglob`, where `@([a|b)` matches the
			// string `@([a|b)` and matches neither `b` nor `[a`.
			//
			// The shell these rows are worded for answers `bad pattern`
			// instead — it agrees the group never closed and reports that
			// rather than falling back to text, which is a different axis
			// and not this one. What all three readings share is that the
			// `[` did not leave `b` standing as an arm.
			"an unterminated bracket closes no group", "b", "([a|b)", false,
		},
		{"nor is the text in front of the bar an arm", "[a", "([a|b)", false},
		{"and the whole of it is ordinary text", "([a|b)", "([a|b)", true},
		{
			// The two controls that were already right and must stay so: a
			// group with no bracket, and a bracket with no group.
			"a group with no bracket", "a", "(x|a)", true,
		},
		{"a group that does not match", "a", "(x|y)", false},
		{"a bracket with no group", "a", "[a|b]", true},
		{"and the bar it holds", "|", "[a|b]", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBracketGroup(t, tc.subject, tc.pattern)
			want := "N"
			if tc.want {
				want = "M"
			}
			if out != want || st != 0 {
				t.Errorf("%q vs %q: %q at %d, want %q at 0", tc.subject, tc.pattern, out, st, want)
			}
		})
	}
}

// A parenthesis inside a bracket expression is a member too (#3075).
//
// The same blind spot seen from the other side, and it is the half that
// decides whether the group is a group at all: the scan looking for the `)`
// that closes a `(` counted the ones inside the bracket, so `([(])` had no
// closing parenthesis and was not read as a group.
//
// Measured 2026-09-15: `[[ '(' == ${~p} ]]` with `p='([(])'` is 0 on zsh
// 5.9.2, and `@([(])` matches `(` in bash 5.3.20, bash 3.2.57 with `extglob`
// and ksh93u+. Unanimous again.
func TestAParenthesisInsideABracketIsAMember(t *testing.T) {
	for _, tc := range []struct {
		name, subject, pattern string
		want                   bool
	}{
		{"an open paren as the only member", "(", "([(])", true},
		{"a close paren as the only member", ")", "([)])", true},
		{"an open paren beside an arm", "(", "([(]|z)", true},
		{"the arm behind it", "z", "([(]|z)", true},
		{
			// The subject that would match if the group had been read as
			// ordinary text instead.
			"the pattern's own characters", "([(])", "([(])", false,
		},
		{
			// An unbalanced paren inside the bracket, which is the shape a
			// depth counter cannot survive at all.
			"an unbalanced paren among members", "(", "([ab(])", true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBracketGroup(t, tc.subject, tc.pattern)
			want := "N"
			if tc.want {
				want = "M"
			}
			if out != want || st != 0 {
				t.Errorf("%q vs %q: %q at %d, want %q at 0", tc.subject, tc.pattern, out, st, want)
			}
		})
	}
}

// shellQuote wraps a literal in single quotes for a snippet, doubling nothing
// because no probe here holds one.
func shellQuote(s string) string { return "'" + s + "'" }
