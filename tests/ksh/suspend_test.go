// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// What ksh93 says about a job it stopped and resumed, and what it does not say
// when asked to leave one behind. Measured through a pseudo-terminal on
// 2026-09-05: `sleep 40`, ^Z, `bg`, `exit`.
func TestWhatKshSaysAboutASuspendedJob(t *testing.T) {
	dg := ksh.Diagnostics()
	if dg.JobStoppedNotice != "" {
		t.Errorf("JobStoppedNotice = %q, want the listing row", dg.JobStoppedNotice)
	}
	if dg.JobStoppedNoticeOnANewLine {
		t.Error("ksh93 writes the notice on the same line as the echo")
	}
	if dg.JobResumedInForeground != "" {
		t.Errorf("JobResumedInForeground = %q, want the command alone", dg.JobResumedInForeground)
	}
	// A tab between the number and the command, and no space before the `&`.
	if got, want := dg.JobResumedInBackground, "[%[1]d]\t%[3]s&"; got != want {
		t.Errorf("JobResumedInBackground = %q, want %q", got, want)
	}
	if ksh.Semantics().StoppedJobsHoldTheExit != interp.No {
		t.Error("ksh93 leaves rather than holding its exit for a stopped job")
	}
	if dg.StoppedJobsAtExit != "" {
		t.Errorf("StoppedJobsAtExit = %q, want nothing said", dg.StoppedJobsAtExit)
	}
}
