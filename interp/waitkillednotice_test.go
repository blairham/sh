// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether a `wait` that reaped a background job says a signal ended it is
// Semantics.WaitReportsTheSignalThatEndedTheJob, and the *sentence* is the
// vector's second answer: a dialect with a `wait`-flavored wording writes
// that one, and a dialect without writes the one a signal that ended a
// foreground command earns.
//
// Two answers in one place, because the alternative is two implementations of
// the same reap — and a second copy is how one route gains a sentence the
// other never learns about.
func TestWhatAWaitSaysAboutTheSignalThatEndedTheJob(t *testing.T) {
	const src = "sh -c 'kill -TERM $$' &\nwait $!\n"

	reap := func(t *testing.T, reports Answer, own string) string {
		t.Helper()
		var errs strings.Builder
		run(t, src, func(r *Runner) {
			sem := CoreSemantics()
			// The encoding that makes the signal readable back out of the
			// status, which every dialect reaching here holds.
			sem.SignalDeathStatusIsTwoFiftySix = Yes
			sem.WaitReportsTheSignalThatEndedTheJob = reports
			dg := Diagnostics{
				WaitSignalNotice: own,
				// The general sentence, which is the one a vector with no
				// wording of its own falls back to.
				KilledCommandNotice:           "%[2]s",
				KilledCommandNoticeUnprefixed: true,
			}
			r.Semantics, r.Diagnostics, r.Stderr = &sem, &dg, &errs
		})
		return errs.String()
	}

	t.Run("a wording of its own is what is written", func(t *testing.T) {
		errs := reap(t, Yes, "wait: %[1]d: signal %[2]s")
		if !strings.Contains(errs, "wait: ") || !strings.Contains(errs, "signal Terminated") {
			t.Errorf("stderr %q, want the wait-flavored sentence", errs)
		}
	})

	t.Run("and with none, the sentence a killed command earns", func(t *testing.T) {
		errs := reap(t, Yes, "")
		// The words are the host's — this machine puts the number after them
		// — so the claim is which sentence was reached, not its spelling.
		if !strings.HasPrefix(strings.TrimSpace(errs), "Terminated") {
			t.Errorf("stderr %q, want the general killed-command sentence", errs)
		}
	})

	// The axis is what decides whether anything is said at all, so a vector
	// that answers No stays silent however many wordings it holds.
	t.Run("a shell that does not report says nothing", func(t *testing.T) {
		for _, own := range []string{"", "wait: %[1]d: signal %[2]s"} {
			if errs := reap(t, No, own); strings.TrimSpace(errs) != "" {
				t.Errorf("stderr %q with wording %q, want silence", errs, own)
			}
		}
	})

	// And the status is not this question's to touch: the signal made it
	// whether or not the shell remarked on it.
	t.Run("the status is the same either way", func(t *testing.T) {
		for _, reports := range []Answer{Yes, No} {
			var errs strings.Builder
			_, st := run(t, src+"echo st=$?\n", func(r *Runner) {
				sem := CoreSemantics()
				sem.SignalDeathStatusIsTwoFiftySix = Yes
				sem.WaitReportsTheSignalThatEndedTheJob = reports
				dg := Diagnostics{KilledCommandNotice: "%[2]s", KilledCommandNoticeUnprefixed: true}
				r.Semantics, r.Diagnostics, r.Stderr = &sem, &dg, &errs
			})
			if st != 0 {
				t.Errorf("status %d with %v, want the script to run on", st, reports)
			}
		}
	})
}
