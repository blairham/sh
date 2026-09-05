// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// What bash says about a job it stopped, resumed and was asked to leave
// behind. Measured through a pseudo-terminal against bash 5.3.15 on
// 2026-09-05: `sleep 40`, ^Z, `bg`, `exit`.
func TestWhatBashSaysAboutASuspendedJob(t *testing.T) {
	dg := bash.Diagnostics()
	// ^Z prints the listing's own row, so there is nothing of its own to say
	// — and a newline first, because the terminal has just echoed `^Z` where
	// the cursor was and left it there.
	if dg.JobStoppedNotice != "" {
		t.Errorf("JobStoppedNotice = %q, want the listing row", dg.JobStoppedNotice)
	}
	if !dg.JobStoppedNoticeOnANewLine {
		t.Error("the notice starts on a line of its own here")
	}
	// `fg` names the command and nothing else.
	if dg.JobResumedInForeground != "" {
		t.Errorf("JobResumedInForeground = %q, want the command alone", dg.JobResumedInForeground)
	}
	// `bg` puts the row's head in front of it and the `&` after: `[1]+ sleep
	// 40 &`, with one space after the marker where a listing row has two.
	if got, want := dg.JobResumedInBackground, "[%[1]d]%[2]s %[3]s &"; got != want {
		t.Errorf("JobResumedInBackground = %q, want %q", got, want)
	}
	if got, want := dg.StoppedJobsAtExit, "There are stopped jobs."; got != want {
		t.Errorf("StoppedJobsAtExit = %q, want %q", got, want)
	}
	// And the held `exit` reports a builtin that failed — `echo $?` after the
	// refusal says 1, where zsh's says nothing of the kind.
	if got := dg.StoppedJobsAtExitStatus; got != 1 {
		t.Errorf("StoppedJobsAtExitStatus = %d, want 1", got)
	}
	if bash.Semantics().StoppedJobsHoldTheExit != interp.Yes {
		t.Error("bash stays rather than leaving a job stopped")
	}
}
