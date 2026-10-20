// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// A stopped job wins the jobs-at-exit sentence here wherever it sits (#4544).
//
// The reading this shell's code carried for both dialects, and it is bash's.
// Measured 2026-09-26 on a pseudo-terminal against `/opt/homebrew/bin/bash` —
// GNU bash 5.3.20(1)-release, `go version -m` on it says it is not a Go
// executable — with `shopt -s checkjobs`, two `sleep 30 &` and one `kill
// -STOP`, the order swapped between the rows: `There are stopped jobs.` both
// times, with the listing underneath showing the running one in the place the
// table put it.
//
// The pair matters more than the value. zsh answers the same grid the other
// way, so an implementation with one rule for both was right in one column by
// accident of how the first measurement happened to be arranged.
func TestThisPresetPrefersTheStoppedJobsAtExitSentence(t *testing.T) {
	if got := bash.Semantics().JobsAtExitSentenceFollowsTheTableOrder; got != interp.No {
		t.Errorf("JobsAtExitSentenceFollowsTheTableOrder is %v, want No", got)
	}
}
