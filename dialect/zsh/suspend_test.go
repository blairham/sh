// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// What zsh says about a job it stopped, resumed and was asked to leave
// behind. Measured through a pseudo-terminal against zsh 5.9.2 on 2026-09-05:
// `sleep 40`, ^Z, `bg`, `exit`.
func TestWhatZshSaysAboutASuspendedJob(t *testing.T) {
	dg := zsh.Diagnostics()
	// A sentence rather than a listing row, and the only one of the four that
	// names no job number: `zsh: suspended  sleep 40`, two spaces.
	if got, want := dg.JobStoppedNotice, "%[3]s: suspended  %[4]s"; got != want {
		t.Errorf("JobStoppedNotice = %q, want %q", got, want)
	}
	if !dg.JobStoppedNoticeOnANewLine {
		t.Error("the notice starts on a line of its own here, as bash's does")
	}
	// `fg` and `bg` print the same row, with a state no listing ever shows.
	const row = "[%[1]d]  %[2]s continued  %[3]s"
	if got := dg.JobResumedInForeground; got != row {
		t.Errorf("JobResumedInForeground = %q, want %q", got, row)
	}
	if got := dg.JobResumedInBackground; got != row {
		t.Errorf("JobResumedInBackground = %q, want %q", got, row)
	}
	// The state column is the listing's, so the two line up under each other.
	if !strings.Contains(dg.JobLine, "%-11") {
		t.Errorf("JobLine = %q, want the 11-wide state column the resume row spells out", dg.JobLine)
	}
	if got, want := dg.StoppedJobsAtExit, "%[1]s: you have suspended jobs."; got != want {
		t.Errorf("StoppedJobsAtExit = %q, want %q", got, want)
	}
	// And the held `exit` reports nothing: `echo $?` after the refusal says 0
	// here, where bash's says 1.
	if got := dg.StoppedJobsAtExitStatus; got != 0 {
		t.Errorf("StoppedJobsAtExitStatus = %d, want 0", got)
	}
	if zsh.Semantics().StoppedJobsHoldTheExit != interp.Yes {
		t.Error("zsh stays rather than leaving a job stopped")
	}
}
