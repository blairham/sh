// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Which arm of an alternation a longest prefix trim takes — the axis, both of
// its answers, the shapes that need no answer, and the refusal for a dialect
// that has not chosen.

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
		sem.LongestPrefixTrimTakesTheWrittenArm = answer
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
	for _, src := range []string{
		`x=abc; printf '%s' "${x##(ab|a)}"`,
		`x=abc; printf '%s' "${x##a*b}"`,
		`x=abc; printf '%s' "${x##zzz}"`,
	} {
		out, status := trimArmRun(t, src, Unspecified)
		if strings.Contains(out, "disagree") || status != 0 {
			t.Errorf("%s was refused with no disagreement to refuse: %q (status %d)",
				src, out, status)
		}
	}
}
