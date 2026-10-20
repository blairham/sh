// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// The jobs-at-exit sentence is chosen by the first job in the table here, and
// not by preferring a stopped one (#4544).
//
// Measured 2026-09-26 on a pseudo-terminal against `/opt/homebrew/bin/zsh` —
// zsh 5.9.2 (aarch64-apple-darwin25.4.0), `go version -m` on it says it is not
// a Go executable — started `-fiV +Z`, two `sleep 30 &` and one `kill -STOP`,
// with the order swapped between the rows:
//
//	%1 running, %2 stopped   zsh: you have running jobs.
//	%1 stopped, %2 running   zsh: you have suspended jobs.
//
// Three jobs confirm it rather than two: `run, stop, run` is running and
// `stop, run, stop` is suspended. It is **not** the current job, which is the
// more plausible noun — stopping `%2` makes `%2` current, and the first row
// still says running — and it is not interactivity, since a `zsh -fm` script
// answers the same pair.
func TestThisPresetChoosesTheJobsAtExitSentenceByTheTable(t *testing.T) {
	if got := zsh.Semantics().JobsAtExitSentenceFollowsTheTableOrder; got != interp.Yes {
		t.Errorf("JobsAtExitSentenceFollowsTheTableOrder is %v, want Yes", got)
	}
}
