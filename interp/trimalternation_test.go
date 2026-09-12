// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Which arm of an alternation a longest match takes — the axis, both of its
// answers, the shapes that need no answer, and the refusal for a dialect that
// has not chosen.
//
// It is the *longest prefix trim's* question and the *substitution's*, which
// are the two places a match is asked for the longest piece at a position
// whose right-hand end is free to move. The trims came first; the
// substitution had the same rule and did not follow it (#2152).

// trimArmRun runs src with bare groups on for the parser and the runner, and
// the axis answered as named. Both halves are needed: the parser decides that
// `(a|ab)` is a group rather than four literal characters, and the matcher
// asks the runner's dialect the same question again.
func trimArmRun(t *testing.T, src string, answer Answer) (string, int) {
	t.Helper()
	enable := func(d *syntax.Dialect) {
		d.PatternAlternation = true
		// The flag group is the same match seen from the other side, so the
		// grammar for it is on here rather than in a helper of its own.
		d.ParamExpansionFlags = true
	}
	return runGrammar(t, src, enable, func(r *Runner) {
		d := syntax.Core()
		enable(&d)
		r.Dialect = &d
		sem := CoreSemantics()
		sem.LongestMatchTakesTheWrittenArm = answer
		// A second axis the substitution rows below reach the moment an arm
		// is empty, and one this test is not about: `${x//(|a)/X}` has an
		// empty match at the end of the value under one arm reading and one
		// sitting where `a` matched under the other, which are exactly the
		// two positions ReplacementEmptyMatchDeclined parts on. Answered so
		// the rows measure the arm order and nothing else.
		sem.ReplacementEmptyMatchDeclined = EmptyMatchDeclinedAtTheEnd
		r.Semantics = &sem
	})
}

func TestALongestPrefixTrimAsksWhichArmOfAnAlternationItTakes(t *testing.T) {
	for _, tc := range []struct {
		name, src, written, longest string
	}{
		{
			"the shorter arm first",
			`x=abc; printf '%s' "${x##(a|ab)}"`,
			"bc", "c",
		},
		{
			// The same two arms the other way round agree, which is why
			// this one needs no answer and the test above does.
			"the longer arm first",
			`x=abc; printf '%s' "${x##(ab|a)}"`,
			"c", "c",
		},
		{
			// An arm is a preference and not a restriction: the first arm
			// leaves the `c` nothing to match, so the search falls back.
			"a later arm the rest of the pattern needs",
			`x=abc; printf '%s' "${x##(a|ab)c}"`,
			"", "",
		},
		{
			// And within the arm it still takes as much as it can, which
			// is what says this is a search order and not "the shortest".
			"the first arm, matching as much as it can",
			`x=abc; printf '%s' "${x##(a*|ab)}"`,
			"", "",
		},
		{
			"an empty arm is an arm",
			`x=abc; printf '%s' "${x##(|a)}"`,
			"abc", "bc",
		},
		{
			"the arm order survives a star in front of the group",
			`x=abc; printf '%s' "${x##*(a|ab)}"`,
			"bc", "c",
		},
		{
			"two groups, each taking its first arm",
			`x=abc; printf '%s' "${x##(a|ab)(b|bc)}"`,
			"c", "",
		},
		{
			"a group nested in a group",
			`x=abc; printf '%s' "${x##((a|ab))}"`,
			"bc", "c",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := trimArmRun(t, tc.src, Yes)
			if out != tc.written || status != 0 {
				t.Errorf("written-arm answer: %s = %q (status %d), want %q",
					tc.src, out, status, tc.written)
			}
			out, status = trimArmRun(t, tc.src, No)
			if out != tc.longest || status != 0 {
				t.Errorf("longest answer: %s = %q (status %d), want %q",
					tc.src, out, status, tc.longest)
			}
		})
	}
}

// The axis is the *longest prefix* trim's and no other trim's, which is what
// keeps it off three rows the panel agrees about. The single `#` takes the
// shortest match whichever arm was written first, and both suffix trims take
// the longest — so every row here answers the same under either value.
func TestTheOtherThreeTrimsDoNotAskWhichArmWasWritten(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the shortest prefix, shorter arm first", `x=abc; printf '%s' "${x#(a|ab)}"`, "bc"},
		{"the shortest prefix, longer arm first", `x=abc; printf '%s' "${x#(ab|a)}"`, "bc"},
		{"the longest suffix, with an empty arm", `x=abc; printf '%s' "${x%%(|bc)}"`, "a"},
		{"the shortest suffix", `x=abc; printf '%s' "${x%(c|bc)}"`, "ab"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, a := range []Answer{Yes, No, Unspecified} {
				out, status := trimArmRun(t, tc.src, a)
				if out != tc.want || status != 0 {
					t.Errorf("axis %v: %s = %q (status %d), want %q",
						a, tc.src, out, status, tc.want)
				}
			}
		})
	}
}

// The same axis on the operator that replaces rather than trims, which asks
// it at every position rather than once.
//
// A substitution takes the longest match at each position exactly as
// `${x##pat}` does, so the arm the matcher would have preferred was decided
// before the matcher was consulted there too. Every row is a spelling of the
// same question, because the fault was in the one walk all of them go
// through.
func TestASubstitutionAsksWhichArmOfAnAlternationItTakes(t *testing.T) {
	for _, tc := range []struct {
		name, src, written, longest string
	}{
		{
			"the shorter arm first, replaced once",
			`x=abc; printf '%s' "${x/(a|ab)/X}"`,
			"Xbc", "Xc",
		},
		{
			"and everywhere",
			`x=abcabc; printf '%s' "${x//(a|ab)/X}"`,
			"XbcXbc", "XcXc",
		},
		{
			// The same two arms the other way round agree, which is why
			// this one needs no answer and the rows above do.
			"the longer arm first",
			`x=abc; printf '%s' "${x//(ab|a)/X}"`,
			"Xc", "Xc",
		},
		{
			"an empty arm is an arm, and the scan still makes progress",
			`x=abc; printf '%s' "${x/(|a)/X}"`,
			"Xabc", "Xbc",
		},
		{
			"an empty arm everywhere",
			`x=abc; printf '%s' "${x//(|a)/X}"`,
			"XaXbXc", "XXbXc",
		},
		{
			"a match that is not at the start",
			`x=abc; printf '%s' "${x/(b|bc)/X}"`,
			"aXc", "aX",
		},
		{
			"anchored at the start, where the end is still free",
			`x=abc; printf '%s' "${x/#(a|ab)/X}"`,
			"Xbc", "Xc",
		},
		{
			// An arm is a preference and not a restriction: the first arm
			// leaves the `c` nothing to match, so the search falls back and
			// the whole value goes.
			"a later arm the rest of the pattern needs",
			`x=abc; printf '%s' "${x/(a|ab)c/X}"`,
			"X", "X",
		},
		{
			"the first arm, matching as much as it can",
			`x=abc; printf '%s' "${x/(a*|ab)/X}"`,
			"X", "X",
		},
		{
			"the arm order survives a star in front of the group",
			`x=abc; printf '%s' "${x/*(a|ab)/X}"`,
			"Xbc", "Xc",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := trimArmRun(t, tc.src, Yes)
			if out != tc.written || status != 0 {
				t.Errorf("written-arm answer: %s = %q (status %d), want %q",
					tc.src, out, status, tc.written)
			}
			out, status = trimArmRun(t, tc.src, No)
			if out != tc.longest || status != 0 {
				t.Errorf("longest answer: %s = %q (status %d), want %q",
					tc.src, out, status, tc.longest)
			}
		})
	}
}

// And where the substitution has no arm question to ask, which is the same
// boundary the trims have: it is only there when the operator wants the
// longest match **and** the end of that match is free to move.
//
// `/%` pins the end to the end of the value, so every match at a given start
// is the same length; `(S)` asks for the shortest, which is the minimum over
// every arm, so the first arm that matches at all matches exactly there. Both
// are measured on zsh 5.9.2 rather than reasoned from the shape — see the
// corpus rows — and every row here answers the same under either value of the
// axis, which is what the rows assert.
//
// The implementation skips the preparation for these two rather than guarding
// the behavior, so mutating that condition away leaves this test green. That
// is the correct outcome and is why the rows are written against the axis's
// three values rather than against the condition.
func TestTheSubstitutionsThatDoNotAskWhichArmWasWritten(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"anchored at the end", `x=abc; printf '%s' "${x/%(c|bc)/X}"`, "aX"},
		{"anchored at the end, longer arm first", `x=abc; printf '%s' "${x/%(bc|c)/X}"`, "aX"},
		{"the shortest match", `x=abc; printf '%s' "${(S)x//(a|ab)/X}"`, "Xbc"},
		{"the shortest match, longer arm first", `x=abc; printf '%s' "${(S)x//(ab|a)/X}"`, "Xbc"},
		{"no alternation at all", `x=abc; printf '%s' "${x//a*b/X}"`, "Xc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, a := range []Answer{Yes, No, Unspecified} {
				out, status := trimArmRun(t, tc.src, a)
				if out != tc.want || status != 0 {
					t.Errorf("axis %v: %s = %q (status %d), want %q",
						a, tc.src, out, status, tc.want)
				}
			}
		})
	}
}

// The two readings of the same match, seen from the other side: the `(M)`
// flag keeps what the pattern took rather than what it left, and a `(#b)`
// group reports the arm the search settled on. Both follow the axis, because
// both are the one match trimSpan found.
func TestTheArmReachesWhatTheMatchReports(t *testing.T) {
	out, _ := trimArmRun(t, `x=abc; printf '%s' "${(M)x##(a|ab)}"`, Yes)
	if out != "a" {
		t.Errorf("the kept half under the written-arm answer = %q, want %q", out, "a")
	}
	out, _ = trimArmRun(t, `x=abc; printf '%s' "${(M)x##(a|ab)}"`, No)
	if out != "ab" {
		t.Errorf("the kept half under the longest answer = %q, want %q", out, "ab")
	}
}

// An unanswered dialect is refused by name rather than given one of the two
// readings — and only where they differ. A pattern whose arms agree, and a
// pattern with no alternation in it at all, are answered as they always were.
func TestAnUnansweredArmOrderIsRefusedOnlyWhereTheReadingsDiffer(t *testing.T) {
	out, status := trimArmRun(t, `x=abc; printf '%s' "${x##(a|ab)}"`, Unspecified)
	if !strings.Contains(out, "the shells disagree here and no dialect was chosen") {
		t.Errorf("an unanswered axis was not refused: %q", out)
	}
	if status == 0 {
		t.Errorf("status %d, want a failure for an unanswered axis", status)
	}
	// And the substitution reaches the same refusal, which is the half that
	// was missing: an unanswered dialect answered `${x//(a|ab)/X}` at status
	// 0 with one of the two readings picked for it.
	out, status = trimArmRun(t, `x=abc; printf '%s' "${x//(a|ab)/X}"`, Unspecified)
	if !strings.Contains(out, "the shells disagree here and no dialect was chosen") {
		t.Errorf("an unanswered axis was not refused at a substitution: %q", out)
	}
	if status == 0 {
		t.Errorf("status %d, want a failure for an unanswered axis", status)
	}
	for _, src := range []string{
		`x=abc; printf '%s' "${x##(ab|a)}"`,
		`x=abc; printf '%s' "${x##a*b}"`,
		`x=abc; printf '%s' "${x##zzz}"`,
		`x=abc; printf '%s' "${x//(ab|a)/X}"`,
		`x=abc; printf '%s' "${x//a*b/X}"`,
		`x=abc; printf '%s' "${x/%(c|bc)/X}"`,
	} {
		out, status := trimArmRun(t, src, Unspecified)
		if strings.Contains(out, "disagree") || status != 0 {
			t.Errorf("%s was refused with no disagreement to refuse: %q (status %d)",
				src, out, status)
		}
	}
}
