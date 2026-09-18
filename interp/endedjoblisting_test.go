// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// One column reaps a `&` job only while it is watching its children, so with
// no monitor its listing goes on calling an ended job a running one. This
// implementation always knows — a background statement runs on a goroutine
// that finishes whether anybody asked for the monitor or not — so the blind
// spot has to be reproduced on purpose, which is what
// Semantics.EndedJobIsListedAsRunningWithoutTheMonitor is.
//
// The monitor is what tells the two apart, and that is the whole test: the
// same script, the same finished job, one `set -m` between them.
func TestAnEndedJobIsListedAsRunningWithoutTheMonitor(t *testing.T) {
	const ended = `true & sleep 0.05; jobs`

	endedListing := func(t *testing.T, src string, blind Answer) string {
		t.Helper()
		out, st := run(t, src, func(r *Runner) {
			sem := CoreSemantics()
			sem.JobsListFinishedJobs = Yes
			sem.JobsShowBackgroundCommand = Yes
			sem.EndedJobIsListedAsRunningWithoutTheMonitor = blind
			// `set -m` is what the rows below turn on, and a shell with no
			// terminal is refused it where the vector says it needs one.
			sem.MonitorNeedsATerminal = No
			r.Semantics = &sem
		})
		if st != 0 {
			t.Fatalf("status %d: %s", st, out)
		}
		return out
	}

	t.Run("the blind reading says Running with no monitor", func(t *testing.T) {
		out := endedListing(t, ended, Yes)
		if !strings.Contains(out, "Running") {
			t.Errorf("out = %q, want the ended job listed as running", out)
		}
		if strings.Contains(out, "Done") {
			t.Errorf("out = %q, want the end not noticed", out)
		}
	})

	t.Run("and the end is noticed once the monitor is on", func(t *testing.T) {
		out := endedListing(t, "set -m\n"+ended, Yes)
		if !strings.Contains(out, "Done") {
			t.Errorf("out = %q, want the ended job reported under the monitor", out)
		}
	})

	// The other reading is the one four of the five columns hold, and it is
	// what the axis has to be moved off to reach the rows above: a shell that
	// knows says so whether or not it was asked to watch.
	t.Run("the other reading notices either way", func(t *testing.T) {
		for _, src := range []string{ended, "set -m\n" + ended} {
			out := endedListing(t, src, No)
			if !strings.Contains(out, "Done") {
				t.Errorf("out = %q for %q, want the ended job reported", out, src)
			}
		}
	})
}
