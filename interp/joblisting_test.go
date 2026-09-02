// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a `jobs` listing contains and what order it is in, both of which the
// shells split two ways — so both are the dialect's to answer and neither is
// something the core may decide.
func TestWhatAJobsListingContains(t *testing.T) {
	// Instant, so the job has certainly ended by the time `jobs` runs.
	const src = `true & sleep 0.05; jobs; echo "---"; jobs`

	t.Run("a finished job is listed once and then forgotten", func(t *testing.T) {
		out := listing(t, `true & sleep 0.05; jobs; echo "---"; jobs`, Yes, No)
		if !strings.Contains(out, "Done") {
			t.Errorf("out = %q, want the finished job reported", out)
		}
		// The listing that reports it is the listing that forgets it. Not an
		// axis: every shell mentions a finished job at most once, and one
		// that kept them would grow a listing for the whole session.
		after := out[strings.Index(out, "---"):]
		if strings.Contains(after, "Done") {
			t.Errorf("after = %q, want it reported once and forgotten", after)
		}
	})

	t.Run("a dialect can leave it out entirely", func(t *testing.T) {
		out := listing(t, src, No, No)
		if strings.Contains(out, "Done") || strings.Contains(out, "true") {
			t.Errorf("out = %q, want a finished job never mentioned", out)
		}
	})

	t.Run("and is forgotten either way", func(t *testing.T) {
		// The job goes even where it was never printed, or a listing nobody
		// reads would keep them forever.
		var r *Runner
		if _, st := run(t, `true & sleep 0.05; jobs`, withListing(No, No, &r)); st != 0 {
			t.Fatalf("status %d", st)
		}
		if n := len(r.Jobs()); n != 0 {
			t.Errorf("%d jobs left, want the finished one forgotten", n)
		}
	})
}

// Two shells list the oldest first and two the newest, and the job keeps its
// own number either way — the number says which job `%2` means, so it cannot
// be the row's place in the listing.
func TestWhichEndAJobsListingStartsFrom(t *testing.T) {
	const src = `sleep 0.3 & sleep 0.3 & jobs`
	for _, tc := range []struct {
		name    string
		newest  Answer
		wantTop string
	}{
		{"oldest first", No, "[1]"},
		{"newest first", Yes, "[2]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := listing(t, src, Yes, tc.newest)
			lines := strings.Split(strings.TrimSpace(out), "\n")
			if len(lines) != 2 {
				t.Fatalf("listed %d jobs, want 2:\n%s", len(lines), out)
			}
			if !strings.HasPrefix(lines[0], tc.wantTop) {
				t.Errorf("top row is %q, want it to start %s", lines[0], tc.wantTop)
			}
			// Both numbers are present whichever end it starts from.
			if !strings.Contains(out, "[1]") || !strings.Contains(out, "[2]") {
				t.Errorf("out = %q, want both job numbers", out)
			}
		})
	}
}

// The core refuses rather than picking an end, and asks only where the answer
// decides something: one job is in the same place either way.
func TestAJobsListingRefusesOnlyWhereTheAnswerMatters(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		refuses   bool
	}{
		{"two jobs, so the order decides", `sleep 0.3 & sleep 0.3 & jobs`, true},
		{"one job, so it does not", `sleep 0.3 & jobs`, false},
		{"a finished job, so whether to list it decides", `true & sleep 0.05; jobs`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, func(r *Runner) {
				sem := CoreSemantics()
				r.Semantics = &sem
			})
			if got := strings.Contains(out, "no dialect was chosen"); got != tc.refuses {
				t.Errorf("refused = %v, want %v (out %q)", got, tc.refuses, out)
			}
		})
	}
}

func withListing(finished, newest Answer, into **Runner) func(*Runner) {
	return func(r *Runner) {
		sem := CoreSemantics()
		sem.JobsListFinishedJobs = finished
		sem.JobsListNewestFirst = newest
		r.Semantics = &sem
		if into != nil {
			*into = r
		}
	}
}

func listing(t *testing.T, src string, finished, newest Answer) string {
	t.Helper()
	out, st := run(t, src, withListing(finished, newest, nil))
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	return out
}
