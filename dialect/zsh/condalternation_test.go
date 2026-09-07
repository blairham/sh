// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// parseFails reports whether this dialect refuses the source outright, which
// answersRun cannot say: it fails the test on a parse error rather than
// returning one.
func parseFails(t *testing.T, src string) bool {
	t.Helper()
	_, err := syntax.Parse(src, zsh.Dialect())
	return err != nil
}

// A group standing where a pattern is read, end to end through this dialect.
//
// It is the commonest idiom in this shell's completion files — `_docker` and
// `_rg` are unusable without it — which is what puts it far above its two-file
// count in the real-script sweep (#826, #815).
func TestAGroupMayStartAPatternOperand(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`k=a; [[ $k == (a|b) ]] && echo hit || echo miss`, "hit"},
		{`k=c; [[ $k == (a|b) ]] && echo hit || echo miss`, "miss"},
		{`k=a; [[ $k = (a|b) ]] && echo hit || echo miss`, "hit"},
		{`k=a; [[ $k != (a|b) ]] && echo hit || echo miss`, "miss"},
		{`k=abc; [[ $k == (a|b)* ]] && echo hit || echo miss`, "hit"},
		{`k=xbc; [[ $k == (a|b)* ]] && echo hit || echo miss`, "miss"},
		{`k=ab; [[ $k == ((a|b)|x)(b|c) ]] && echo hit || echo miss`, "hit"},
		// The group is one word, so a blank inside it is pattern text.
		{`k="a b"; [[ $k == (a b) ]] && echo hit || echo miss`, "hit"},
		{`k=a; [[ $k == (a b) ]] && echo hit || echo miss`, "miss"},
		// Quoted it is a literal, which is the same per-span rule `a*` has.
		{`k=a; [[ $k == "(a|b)" ]] && echo hit || echo miss`, "miss"},
		{`k="(a|b)"; [[ $k == "(a|b)" ]] && echo hit || echo miss`, "hit"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
}

// What the printer promises is that printed source *means* the same thing, so
// the check has to be that the printed form still matches — not that printing
// is settled. A printer that quoted the group would round-trip perfectly and
// turn every pattern into a literal; measured by mutation, that is exactly
// what a stability-only test lets through.
func TestAPrintedPatternOperandStillMatches(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`k=a; [[ $k == (a|b) ]] && echo hit || echo miss`, "hit"},
		{`k=c; [[ $k == (a|b) ]] && echo hit || echo miss`, "miss"},
		{`k=abc; [[ $k == (a|b)* ]] && echo hit || echo miss`, "hit"},
		{`k=ab; [[ $k == ((a|b)|x)(b|c) ]] && echo hit || echo miss`, "hit"},
		// And the quoted one has to stay quoted, or printing would turn a
		// literal into a pattern in the other direction.
		{`k=a; [[ $k == "(a|b)" ]] && echo hit || echo miss`, "miss"},
		{`k="(a|b)"; [[ $k == "(a|b)" ]] && echo hit || echo miss`, "hit"},
	} {
		f, err := syntax.Parse(tc.src, zsh.Dialect())
		if err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		printed := syntax.Print(f)
		out, _ := answersRun(t, printed)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s printed as %q, which said %q, want %q", tc.src, printed, got, tc.want)
		}
	}
}

// And the boundary: a group is read where a *pattern* is read and nowhere
// else in the construct, so these stay refused in this dialect too.
func TestAGroupIsRefusedWhereNoPatternIsRead(t *testing.T) {
	for _, src := range []string{
		`[[ -n (a|b) ]]`,
		`k=a; [[ (a|b) == $k ]]`,
		`k=a; [[ $k == () ]]`,
		`k=a; [[ $k ==(a|b) ]]`,
	} {
		if !parseFails(t, src) {
			t.Errorf("%s: parsed, want a syntax error", src)
		}
	}
	// And the reading does not escape the operand it was turned on for: a
	// `(` after the condition is the subshell it has always been. Found by
	// mutation — leaving the flag set past the operand left every `(` after
	// a `[[ … == … ]]` scanned as a word, so `&& (echo x)` became a command
	// named `(echo x)` and still parsed.
	for _, tc := range []struct{ src, want string }{
		{`[[ 1 == 1 ]] && (echo x)`, "x"},
		{`k=a; [[ $k == (a|b) ]]; (echo w)`, "w"},
		{`k=a; [[ $k == (a|b) ]] && (echo y)`, "y"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}

	// While the parentheses this shell has always had keep working.
	for _, tc := range []struct{ src, want string }{
		{`k=a; [[ ( -n $k ) ]] && echo hit || echo miss`, "hit"},
		{`k=a; [[ ( $k == a ) ]] && echo hit || echo miss`, "hit"},
		{`(( 1 + 1 == 2 )) && echo hit || echo miss`, "hit"},
		{`k=a; [[ $k =~ (a|b) ]] && echo hit || echo miss`, "hit"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
}

// A numeric range inside a group that *starts* a pattern, end to end through
// this dialect. It is the version gate every powerlevel10k config opens with,
// and none of that configuration ever loaded because the line would not parse
// (#1217).
//
// Behavioral rather than a parse check, because parsing is not the promise:
// the group is passed to the matcher as text, so a scanner that took the
// right bytes and handed on the wrong ones would parse perfectly and match
// the wrong subject. The `miss` rows are what make that visible.
func TestANumericRangeMayStartAPatternGroup(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The gate itself, on both sides of every bound.
		{`k=5.9; [[ $k == (5.<1->*|<6->.*) ]] && echo hit || echo miss`, "hit"},
		{`k=6.1; [[ $k == (5.<1->*|<6->.*) ]] && echo hit || echo miss`, "hit"},
		{`k=4.3; [[ $k == (5.<1->*|<6->.*) ]] && echo hit || echo miss`, "miss"},
		{`k=5.0; [[ $k == (5.<1->*|<6->.*) ]] && echo hit || echo miss`, "miss"},
		// All four spellings of the range, each with the subject that says
		// the *bounds* were read and not merely the shape.
		{`k=6; [[ $k == (<->) ]] && echo hit || echo miss`, "hit"},
		{`k=x; [[ $k == (<->) ]] && echo hit || echo miss`, "miss"},
		{`k=5; [[ $k == (<1-9>) ]] && echo hit || echo miss`, "hit"},
		{`k=10; [[ $k == (<1-9>) ]] && echo hit || echo miss`, "miss"},
		{`k=7; [[ $k == (<6->) ]] && echo hit || echo miss`, "hit"},
		{`k=5; [[ $k == (<6->) ]] && echo hit || echo miss`, "miss"},
		{`k=4; [[ $k == (<-9>) ]] && echo hit || echo miss`, "hit"},
		{`k=40; [[ $k == (<-9>) ]] && echo hit || echo miss`, "miss"},
		// A group nested inside the leading one: the outer group is what the
		// word-leading scanner reads, so it has to carry the inner `<`.
		{`k=x6; [[ $k == (x(<6->)) ]] && echo hit || echo miss`, "hit"},
		{`k=x5; [[ $k == (x(<6->)) ]] && echo hit || echo miss`, "miss"},
		{`k=6; [[ $k == ((<6->)|x) ]] && echo hit || echo miss`, "hit"},
		{`k=y; [[ $k == ((<6->)|x) ]] && echo hit || echo miss`, "miss"},
		// The same scanner from a `case` arm, whose own paren makes the
		// group's `((`.
		{`case 6 in ((<6->)) echo hit;; *) echo miss;; esac`, "hit"},
		{`case 4 in ((<6->)) echo hit;; *) echo miss;; esac`, "miss"},
		// Quoted it is four characters, which is the per-span rule a group
		// and a `*` already have.
		{`k=6; [[ $k == "(<6->)" ]] && echo hit || echo miss`, "miss"},
		{`k="(<6->)"; [[ $k == "(<6->)" ]] && echo hit || echo miss`, "hit"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
}

// And the boundary, from the same end: admitting the range must not admit
// every `<`. Each of these still fails to parse in this dialect, as it does
// in zsh 5.9.2 — measured 2026-09-07, each in a script file of its own under
// `env -i`.
//
// Asserted as a refusal through the dialect rather than as a message, because
// the wording is #1111's business; what this pins is that the shape test is
// the whole of the exception.
func TestOnlyTheRangeShapeSurvivesALeadingGroup(t *testing.T) {
	for _, src := range []string{
		`k=x; [[ $k == (a<b) ]] && echo hit || echo miss`,
		`k=x; [[ $k == (a>b) ]] && echo hit || echo miss`,
		`k=x; [[ $k == (a;b) ]] && echo hit || echo miss`,
		`k=x; [[ $k == (a&b) ]] && echo hit || echo miss`,
		`k=x; [[ $k == (<a-b>) ]] && echo hit || echo miss`,
		`k=x; [[ $k == (<1-2-3>) ]] && echo hit || echo miss`,
		`k=x; [[ $k == (<-->) ]] && echo hit || echo miss`,
		`k=x; [[ $k == (<>) ]] && echo hit || echo miss`,
		`k=x; [[ $k == (<1) ]] && echo hit || echo miss`,
	} {
		if !parseFails(t, src) {
			t.Errorf("%s: parsed, where the `<` is not a range and ends the word", src)
		}
	}
}
