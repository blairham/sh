// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
)

// What dash says about a job it stopped and resumed, and what it does not say
// when asked to leave one behind. Measured through a pseudo-terminal on
// 2026-09-05: `sleep 40`, ^Z, `bg`, `exit`.
func TestWhatDashSaysAboutASuspendedJob(t *testing.T) {
	dg := dash.Diagnostics()
	if dg.JobStoppedNotice != "" {
		t.Errorf("JobStoppedNotice = %q, want the listing row", dg.JobStoppedNotice)
	}
	// The row lands straight after the `^Z` the terminal echoed, which is the
	// half bash and zsh do differently.
	if dg.JobStoppedNoticeOnANewLine {
		t.Error("dash writes the notice on the same line as the echo")
	}
	if dg.JobResumedInForeground != "" {
		t.Errorf("JobResumedInForeground = %q, want the command alone", dg.JobResumedInForeground)
	}
	if got, want := dg.JobResumedInBackground, "[%[1]d] %[3]s"; got != want {
		t.Errorf("JobResumedInBackground = %q, want %q", got, want)
	}
	// And it leaves: `exit` with a job stopped exits at once and says nothing.
	if dash.Semantics().StoppedJobsHoldTheExit != interp.No {
		t.Error("dash leaves rather than holding its exit for a stopped job")
	}
	if dg.StoppedJobsAtExit != "" {
		t.Errorf("StoppedJobsAtExit = %q, want nothing said", dg.StoppedJobsAtExit)
	}
}
