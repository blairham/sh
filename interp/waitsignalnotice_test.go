// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `wait` that named something and reaped a child a signal ended has a
// sentence of its own in one dialect — the builtin and the process id, where
// the same shell's report for a *foreground* command a signal killed names
// neither.

// waitNoticeRun runs src with the one wording under test, and hands back what
// the shell wrote where it writes a complaint.
func waitNoticeRun(t *testing.T, src string, notice bool) string {
	t.Helper()
	var errs strings.Builder
	run(t, src, func(r *Runner) {
		sem := CoreSemantics()
		// The encoding that makes the signal readable back out of the
		// status, which is the dialect this wording belongs to.
		sem.SignalDeathStatusIsTwoFiftySix = Yes
		dg := Diagnostics{}
		if notice {
			dg.WaitSignalNotice = "wait: %[1]d: signal %[2]s"
		}
		r.Semantics, r.Diagnostics, r.Stderr = &sem, &dg, &errs
	})
	return errs.String()
}

// TestWaitNamesTheSignalThatEndedTheChildItReaped pins the wording, the two
// routes that reach it, and the three shapes that must stay silent.
func TestWaitNamesTheSignalThatEndedTheChildItReaped(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      string
	}{
		{"by pid", "sh -c 'kill -TERM $$' &\nwait $!\n", "signal Terminated"},
		{"by job spec", "sh -c 'kill -TERM $$' &\nwait %1\n", "signal Terminated"},
		{"a bare wait says nothing", "sh -c 'kill -TERM $$' &\nwait\n", ""},
		{"an ordinary exit says nothing", "sh -c 'exit 3' &\nwait $!\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errs := waitNoticeRun(t, tc.src, true)
			switch {
			case tc.want == "" && strings.Contains(errs, "signal"):
				t.Errorf("stderr %q, want silence", errs)
			case tc.want != "" && !strings.Contains(errs, tc.want):
				t.Errorf("stderr %q, want %q in it", errs, tc.want)
			}
		})
	}

	// And a vector with no wording says nothing at all, which is what four of
	// the five columns want — the silence is the absence of the string rather
	// than a second rule.
	errs := waitNoticeRun(t, "sh -c 'kill -TERM $$' &\nwait $!\n", false)
	if strings.TrimSpace(errs) != "" {
		t.Errorf("with no wording, stderr %q, want nothing", errs)
	}
}
