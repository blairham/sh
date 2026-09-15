// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Which of a misplaced `break`'s two complaints comes out, when both are
// available: there is no loop to leave *and* the count is not a number.
//
// Every column writes exactly one of them and which one divides the panel, so
// this is an order rather than a wording — and it decides whether the script
// survives, because the count's complaint ends it everywhere it is written
// and the place's ends it in one column only. Ours read the count in every
// dialect, so a script bash names the loops for and carries on from stopped
// at 2 (#2299).
func TestWhetherAMisplacedLoopControlJudgesThePlaceFirst(t *testing.T) {
	base := func(order Answer) Semantics {
		s := CoreSemantics()
		s.LoopControlPlaceIsJudgedBeforeTheCount = order
		s.LoopControlOutsideALoopIsFatal = No
		s.FunctionCallIsALoopControlBoundary = Yes
		s.SubshellIsALoopControlBoundary = No
		return s
	}
	dg := Diagnostics{
		LoopControlOutsideALoop: "%[1]s: no loop here",
		LoopControlCount:        "%[1]s: %[2]s: unreadable count",
	}

	for _, tc := range []struct {
		name   string
		order  Answer
		src    string
		want   string
		wantSt int
		why    string
	}{
		{
			"the place first, with no loop at all", Yes,
			`echo t; break abc; echo "after=$?"`,
			"t\nsh: break: no loop here\nafter=0\n", 0,
			"the word is never read, so the place has the only complaint and the script runs on",
		},
		{
			"the count first, with no loop at all", No,
			`echo t; break abc; echo "after=$?"`,
			"t\nsh: break: abc: unreadable count\n", 2,
			"the other four columns: the word is read, refused, and the script ends before the place is looked at",
		},
		{
			"the place first, through the other builtin", Yes,
			`echo t; continue abc; echo "after=$?"`,
			"t\nsh: continue: no loop here\nafter=0\n", 0,
			"one reader for both, so the order is the same and the name in the sentence is not",
		},
		{
			"the place first, across a call boundary", Yes,
			`f(){ break abc; }; for i in 1 2; do f; echo body; done; echo "after=$?"`,
			"sh: break: no loop here\nbody\nsh: break: no loop here\nbody\nafter=0\n", 0,
			"the loop stack is not empty here — the boundary is what leaves the word with no loop it can reach, and the question is about reach",
		},
		{
			"the count first, across a call boundary", No,
			`f(){ break abc; }; for i in 1 2; do f; echo body; done; echo "after=$?"`,
			"sh: break: abc: unreadable count\n", 2,
			"the four that read the word first are unmoved by the boundary",
		},
		{
			"a reachable loop asks nothing", Unspecified,
			`for i in 1 2; do break abc; echo body; done; echo "after=$?"`,
			"sh: break: abc: unreadable count\n", 2,
			"there is a loop to leave, so the two orders answer alike and no dialect is asked",
		},
		{
			"a count that reads asks nothing", Unspecified,
			`echo t; break 1; echo "after=$?"`,
			"t\nsh: break: no loop here\nafter=0\n", 0,
			"the count is a number, so there is no second complaint for an order to choose between",
		},
		{
			"unanswered where it decides", Unspecified,
			`echo t; break abc; echo "after=$?"`,
			"t\nsh: a misplaced `break` being judged before its count is read: the shells disagree here and no dialect was chosen\nafter=2\n", 0,
			"both complaints are available and the strict core reports the axis rather than picking one of them",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := base(tc.order)
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
			if out != tc.want {
				t.Errorf("got %q, want %q — %s", out, tc.want, tc.why)
			}
			if st != tc.wantSt {
				t.Errorf("status %d, want %d — %s", st, tc.wantSt, tc.why)
			}
		})
	}
}
