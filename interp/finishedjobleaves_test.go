// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// finishedLeaves runs src with FinishedJobLeavesTheTable set to a, the rest of
// the vector as the core has it.
func finishedLeaves(t *testing.T, a Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		s := *r.Semantics
		s.FinishedJobLeavesTheTable = a
		r.Semantics = &s
	})
}

// **A table with nothing finished in it asks nothing.** The axis is about a
// job that has ended, so a spec naming a running job, a spec naming none,
// and a listing of running jobs all answer with the vector unanswered.
func TestAJobTableWithNothingFinishedAsksNothing(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`/bin/sleep 0.3 & wait %%; echo "running=$?"`, "running=0\n"},
		{`wait %1; echo "none=$?"`, "none=127\n"},
		{`/bin/sleep 0.2 & jobs >/dev/null; wait; echo listed`, "listed\n"},
		// Ended, but before the shell reaped any child of its own: not
		// noticed, so still nothing to ask.
		{`(exit 5) & :; wait %%; echo "unnoticed=$?"`, "unnoticed=5\n"},
	} {
		out, _ := finishedLeaves(t, Unspecified, c.src)
		if !strings.HasSuffix(out, c.want) || strings.Contains(out, "disagree") {
			t.Errorf("%s gave %q, want it to end %q and refuse nothing", c.src, out, c.want)
		}
	}
}

// **A finished job the shell has noticed is the disagreement**, so an
// unanswered vector refuses it by name, and each answer does what it says.
func TestAFinishedJobIsWhereTheAxisIsAsked(t *testing.T) {
	src := `(exit 4) & /bin/sleep 0.3; wait %%; echo "cur=$?"`
	if out, _ := finishedLeaves(t, Unspecified, src); !strings.Contains(out, "the shells disagree here and no dialect was chosen") {
		t.Errorf("unanswered: %q, want the refusal", out)
	}
	if out, _ := finishedLeaves(t, No, src); !strings.HasSuffix(out, "cur=4\n") {
		t.Errorf("kept: %q, want cur=4", out)
	}
	if out, _ := finishedLeaves(t, Yes, src); !strings.HasSuffix(out, "cur=127\n") {
		t.Errorf("dropped: %q, want cur=127", out)
	}
}

// **With job control and the monitor on, a finished job waits for its report**
// even where the dialect drops one nobody will report. The notice before the
// next prompt, or a listing, is what lets it go in every column — measured on
// zsh 5.9.2 on a pseudo-terminal with `setopt nonotify`, where `jobs` lists
// `exit 4` first — so the spec still finds it here.
func TestAFinishedJobWithAReportComingStays(t *testing.T) {
	out, _ := runGrammar(t, `set -m; (exit 4) & /bin/sleep 0.3; wait %%; echo "cur=$?"`, nil, func(r *Runner) {
		s := *r.Semantics
		s.FinishedJobLeavesTheTable = Yes
		s.MonitorNeedsATerminal = No
		r.Semantics = &s
		r.JobControl = true
	})
	if !strings.HasSuffix(out, "cur=4\n") {
		t.Errorf("got %q, want cur=4", out)
	}
}
