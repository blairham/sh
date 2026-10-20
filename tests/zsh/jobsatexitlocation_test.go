// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// This shell locates the two jobs-at-exit lines in a script, and a held `exit
// N` there leaves with 1 (#4545).
//
// Measured 2026-09-26 on a pseudo-terminal, `zsh -fm <script>` against
// `/opt/homebrew/bin/zsh` — zsh 5.9.2 (aarch64-apple-darwin25.4.0), `go
// version -m` on it says it is not a Go executable. Eight scripts, and the
// shipped binary now answers every one of them byte for byte:
//
//	sleep 3 &                                  :3 and :3, status 0
//	sleep 3 & / print x                        :4 and :4, status 0
//	sleep 3 & / setopt no_check_jobs           :4, the warning alone, 0
//	sleep 3 & / setopt no_check_jobs / print y :5, the warning alone, 0
//	sleep 3 & / exit 7 / print AFTER           :3 and :4, status 1, no AFTER
//	sleep 3 & / exit 7 / exit 9                :3 and :4, status 1, no exit 9
//	sleep 3 & / setopt no_check_jobs / exit 7  :4, the warning alone, 7
//	sleep 3 & / setopt nohup / exit 7          :4, the sentence alone, 1
//
// The sixth row is what says the shell really stops at the `exit` rather than
// going back to the input for another line, and the seventh is what says the
// status belongs to the *sentence* rather than to having jobs — the same
// script with the check switched off leaves with the 7 it was given.
//
// A session is the control and is unchanged: `zsh: you have running jobs.`
// with no line, on both sentences.
func TestThisPresetLocatesTheJobsAtExitLinesInAScript(t *testing.T) {
	dg := zsh.Diagnostics()
	if !dg.JobsAtExitLocatedInAScript {
		t.Errorf("JobsAtExitLocatedInAScript is false, want true")
	}
	if got := dg.HeldExitInAScriptStatus; got != 1 {
		t.Errorf("HeldExitInAScriptStatus is %d, want 1", got)
	}
	// And the prompt route's status is a different number for a different
	// thing: there the shell stays, and it says so at 0.
	if got := dg.StoppedJobsAtExitStatus; got != 0 {
		t.Errorf("StoppedJobsAtExitStatus is %d, want 0 — the held exit at a prompt is not this", got)
	}
}
