// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Three things about a listing's command column that are not the format
// string, and each is a field because one shell does it and the others do not.
func TestTheCommandColumnOfAJobsListing(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		dg        Diagnostics
		show      Answer
		want      string
	}{
		{
			// One shell puts the `&` back, and only while the job runs.
			"the ampersand comes back while it runs",
			`sleep 0.3 & jobs`,
			Diagnostics{JobRunningShowsAmpersand: true},
			Yes,
			"sleep 0.3 &",
		},
		{
			// The same job listed after it has ended has none, which is why
			// this is about rendering the line and not about the text kept.
			"and is gone once it has ended",
			`true & sleep 0.05; jobs`,
			Diagnostics{JobRunningShowsAmpersand: true},
			Yes,
			"Done                    true\n",
		},
		{
			// A job that ended badly says so, and the two shells that
			// distinguish it disagree about how — `Exit 1` against
			// `Done(1)` — so the status is a verb rather than part of a
			// fixed word.
			"a job that failed reports its status",
			`false & sleep 0.05; jobs`,
			Diagnostics{JobExited: "Exit %[1]d"},
			Yes,
			"Exit 1",
		},
		{
			// And a dialect that says nothing different keeps its one word
			// for both, which is what an empty JobExited means.
			"where the dialect has no separate word, it is still Done",
			`false & sleep 0.05; jobs`,
			Diagnostics{},
			Yes,
			"Done",
		},
		{
			// The separate word is for a job that *failed*. A dialect having
			// one does not make it the word for every finished job, which is
			// the way round that reads as working: `Exit 0`.
			"a job that succeeded is Done even where there is another word",
			`true & sleep 0.05; jobs`,
			Diagnostics{JobExited: "Exit %[1]d"},
			Yes,
			"Done",
		},
		{
			"and is absent where the dialect does not ask for it",
			`sleep 0.3 & jobs`,
			Diagnostics{},
			Yes,
			"sleep 0.3\n",
		},
		{
			// A shell that kept no text prints whatever it prints instead —
			// an empty column in one and a placeholder in another.
			"a placeholder stands in where the command is not shown",
			`sleep 0.3 & jobs`,
			Diagnostics{JobUnknownCommand: "<command unknown>"},
			No,
			"<command unknown>",
		},
		{
			"and nothing at all where the dialect has no placeholder",
			`sleep 0.3 & jobs`,
			Diagnostics{},
			No,
			"Running                 \n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := renderJobs(t, tc.src, tc.dg, tc.show)
			if !strings.Contains(out, tc.want) {
				t.Errorf("out = %q, want it to contain %q", out, tc.want)
			}
		})
	}
}

func renderJobs(t *testing.T, src string, dg Diagnostics, show Answer) string {
	t.Helper()
	out, st := run(t, src, func(r *Runner) {
		sem := CoreSemantics()
		sem.JobsListNewestFirst = No
		sem.JobsListFinishedJobs = Yes
		sem.JobsShowBackgroundCommand = show
		r.Semantics, r.Diagnostics = &sem, &dg
	})
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	return out
}
