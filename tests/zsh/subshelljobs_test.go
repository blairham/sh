// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// A subshell of this shell sees the jobs the shell around it started —
// when the shell around it is running the **monitor**.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0). The discriminating grid is two rows by two,
// because the issue was filed as a fact about an *interactive* shell and the
// two nouns agree in every session a person has: this shell turns the monitor
// on for an interactive shell and refuses `set -m` without a terminal, so
// only a probe that moves one while holding the other can tell them apart.
// `sleep 3 & (jobs); jobs`, on a pseudo-terminal:
//
//	interactive  monitor   `(jobs)`
//	no           off       nothing        zsh -f script
//	no           **on**    **the row**    zsh -fm script
//	yes          **off**   **nothing**    zsh -fi, then `unsetopt monitor`
//	yes          on        the row        zsh -fiV +Z
//
// The answer moves with the monitor in both rows where interactivity is held
// fixed and does not move with interactivity in either row where the monitor
// is, so the monitor is the noun (#4538).
//
// **And it is this shell's own answer, not something every column does.**
// Measured the same day on a pseudo-terminal, `-m` moves nothing in the other
// four: bash 5.3.20 and bash 3.2.57 keep the parent's jobs for a substitution
// and for a simple pipeline element and clear them for a compound, with the
// option and without; ksh93u+ keeps them everywhere either way; dash clears
// them everywhere either way; and BusyBox ash 1.37.0 in the pinned alpine
// image answers the same both ways too. So the fourth value of the axis
// belongs to this preset alone.
//
// The listing itself is measured in interp, where a test can set the axis by
// hand and name no shell, and at a session in cmd/zsh, where the monitor can
// really be on. What is pinned here is the dialect's **answers** — the axis
// value and the wording — because a script in this package cannot reach the
// state: `runZsh` has no terminal, so the monitor is off in every case below
// and a behavioral test here would measure the third row of the grid twice.

// The axis value, and that it is not one of the three it is easily mistaken
// for. This preset read Cleared until #4538, which is right about every case
// a script can reach and wrong about every session.
func TestASubshellKeepsTheParentsJobsUnderTheMonitor(t *testing.T) {
	if got := zsh.Semantics().SubshellJobTable; got != interp.SubshellJobsKeptUnderTheMonitor {
		t.Errorf("SubshellJobTable = %v, want kept under the monitor", got)
	}
}

// The refusal the verbs that would *act* on such a job give, which is the
// other half of the answer: the rows are there to be looked at.
//
// Measured in a subshell of a `-fm` zsh 5.9.2 on a pseudo-terminal, with two
// running jobs in the parent — `( jobs %2 )` writes the row and
// `( kill -0 %1 )` answers 0, while
//
//	( wait %2 )     <script>:wait:N: can't manipulate jobs in subshell, 1
//	( disown %1 )   <script>:disown:N: can't manipulate jobs in subshell, 1
//
// The builtin's name is in the location rather than in the sentence, which is
// this shell's rule for every message, so the wording carries no verb.
func TestTheSubshellRefusalIsThisShellsWording(t *testing.T) {
	got := zsh.Diagnostics().JobsNotManipulableInASubshell
	if got == "" {
		t.Fatal("JobsNotManipulableInASubshell is empty; this is the one shell that can be in the state")
	}
	if want := "can't manipulate jobs in subshell"; interp.Wording(got, "", "wait") != want {
		t.Errorf("renders %q, want %q", interp.Wording(got, "", "wait"), want)
	}
	if strings.Contains(got, "%[1]s") {
		t.Errorf("wording %q names the builtin; this shell puts it in the location", got)
	}
	// 1 rather than the engine's default by accident: measured at 1 for both
	// verbs, and left at zero here so the default is what answers.
	if got := zsh.Diagnostics().JobsNotManipulableInASubshellStatus; got != 0 && got != 1 {
		t.Errorf("JobsNotManipulableInASubshellStatus = %d, want the measured 1", got)
	}
	// `fg` and `bg` take the other door and say something else, which is the
	// wording that was already here: a subshell of this shell has the
	// monitor off — measured, `( print ${options[monitor]} )` under `-fm` is
	// `off` where the same read outside is `on` — so they refuse as a shell
	// with no job control refuses.
	if got := zsh.Diagnostics().NoJobControl; got != "no job control in this shell." {
		t.Errorf("NoJobControl = %q, want the wording fg and bg give in a subshell", got)
	}
}
