// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Semantics.EmptyBracketExpressionCompiles: what `[]` and `[!]` are — a
// bracket the POSIX reading leaves unterminated, or one that ends at the `]`
// written first and holds no members at all. Which preset gives which answer
// is asserted in the dialect packages and never here.

// emptyBracketSem answers this axis and pins the two it would otherwise be
// confused with, so a row that moves is moving for this reason: an
// unterminated bracket is a literal `[`, and one a sub-expression left open
// is too.
func emptyBracketSem(compiles Answer) Semantics {
	s := permissive()
	s.EmptyBracketExpressionCompiles = compiles
	s.UnterminatedBracket = BracketLiteral
	s.UnterminatedBracketAfterASubExpression = BracketLiteral
	// A `^` negates here, which is the other spelling of the negation and a
	// question of its own — pinned so the `[^]` row below is about this axis.
	s.BracketCaretNegates = Yes
	return s
}

func matchesEmpty(t *testing.T, compiles Answer, pattern, subject string) bool {
	t.Helper()
	src := `case "` + subject + `" in ` + pattern + `) echo Y;; *) echo n;; esac`
	out, st := run(t, src, withSem(emptyBracketSem(compiles)))
	if st != 0 {
		t.Fatalf("%s vs %q: status %d, out %q", pattern, subject, st, out)
	}
	return out == "Y\n"
}

// **The controls come first, and they are the point.** A bracket that closes
// later reads the `]` written first as a *member* under both answers, so
// these four rows must not move — an implementation keyed on "a `]` written
// first closes the bracket" gets every one of them wrong while passing the
// rows below.
func TestABracketThatClosesLaterReadsTheFirstBracketAsAMember(t *testing.T) {
	for _, c := range []struct {
		name, pattern, subject string
		want                   bool
	}{
		{"a one-member set", "[]]", "]", true},
		{"a two-member set", "[]a]", "a", true},
		{"and its other member", "[]a]", "]", true},
		{"a four-member set", "[]a~b]", "~", true},
		{"a negated set holding it", "[!]]", "a", true},
		{"and the member it excludes", "[!]]", "]", false},
		{"a member it does not hold", "[]a]", "z", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, compiles := range []Answer{Yes, No} {
				if got := matchesEmpty(t, compiles, c.pattern, c.subject); got != c.want {
					t.Errorf("%v: %s vs %q = %v, want %v", compiles, c.pattern, c.subject, got, c.want)
				}
			}
		})
	}
}

// And the rows that move: a bracket the member reading cannot close.
//
// Under Yes it ends at that `]` and holds nothing, so `[]` matches no
// character at all and `[!]` — its negation — matches **any one**. Under No
// it is unterminated, and what that means is the neighboring axis's, pinned
// to the literal reading above.
func TestABracketNothingElseClosesIsEmptyOrUnterminated(t *testing.T) {
	for _, c := range []struct {
		name, pattern, subject string
		yes, no                bool
	}{
		{
			// The negated empty set matches any one character; the literal
			// reading is three characters and matches none of them.
			"the negated empty set", "[!]", "z", true, false,
		},
		{"a caret negates too", "[^]", "z", true, false},
		{
			// And the literal reading matches the three characters it is.
			"the literal reading of it", "[!]", "[!]", false, true,
		},
		{
			// The empty set holds nothing, so it matches nothing — where
			// the literal reading matches the two characters `[]`.
			"the empty set", "[]", "[]", false, true,
		},
		{
			// A subject of one character is what the empty set would match
			// if it held anything, and it holds nothing.
			"the empty set against one character", "[]", "a", false, false,
		},
		{
			// With text behind it, which is the row that says the bracket
			// ends at the `]` rather than swallowing what follows.
			"the empty set with a literal after it", "[]a", "[]a", false, true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := matchesEmpty(t, Yes, c.pattern, c.subject); got != c.yes {
				t.Errorf("compiles: %s vs %q = %v, want %v", c.pattern, c.subject, got, c.yes)
			}
			if got := matchesEmpty(t, No, c.pattern, c.subject); got != c.no {
				t.Errorf("unterminated: %s vs %q = %v, want %v", c.pattern, c.subject, got, c.no)
			}
		})
	}
}

// The structural scans have to know too, and that is not a detail: a group is
// found by a walk that steps over bracket expressions, and a walk that could
// not close `[]` gave up on the **whole group** — so `([]|a)` matched nothing
// at all, the arm `a` included, on every surface at once. The third row is
// the control that says the group machinery is sound without a bracket in it.
func TestAnEmptyBracketInsideAGroupDoesNotCostTheGroup(t *testing.T) {
	for _, c := range []struct {
		name, pattern, subject string
		want                   bool
	}{
		{"an empty set beside an arm that matches", "([]|a)", "a", true},
		{"the same group with the arms swapped", "(a|[])", "a", true},
		{"a group with no bracket in it", "(q|a)", "a", true},
		{"the empty arm itself still matches nothing", "([]|q)", "a", false},
		{"a negated empty set as an arm", "([!]|q)", "a", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Through a prefix trim rather than a `case` arm: an arm's own
			// optional `(` would take the group's, and the row would be
			// measuring the parser instead.
			src := `v="` + c.subject + `"; echo "[${v#` + c.pattern + `}]"`
			out, st := runGrammar(t, src,
				func(d *syntax.Dialect) { d.PatternAlternation = true },
				withSem(emptyBracketSem(Yes)))
			if st != 0 {
				t.Fatalf("status %d, out %q", st, out)
			}
			// The arm took the one character it matched, or nothing did.
			if got := out == "[]\n"; got != c.want {
				t.Errorf("%s vs %q = %q, want trimmed=%v", c.pattern, c.subject, out, c.want)
			}
		})
	}
}
