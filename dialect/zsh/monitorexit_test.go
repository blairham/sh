// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// This shell accounts for the jobs it is leaving behind when its **monitor**
// is on, whether or not anybody is at a prompt — and it is the only one of
// the five that does.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — on a pseudo-terminal. The discriminating
// grid is two rows by two, because the two nouns agree in every session a
// person ever has: this shell turns the monitor on for an interactive shell
// and refuses `set -m` without a terminal, so only a probe that moves one
// while holding the other can tell them apart. `sleep 3 &`, and then an exit:
//
//	interactive  monitor   written
//	no           off       nothing              zsh -f script
//	no           **on**    **both sentences**   zsh -fm script
//	yes          **off**   **nothing**          zsh -fiV +Z, unsetopt monitor
//	yes          on        both sentences       zsh -fiV +Z
//
// The answer moves with the monitor in both rows where interactivity is held
// fixed and does not move with interactivity in either row where the monitor
// is, so the monitor is the noun (#4542).
//
// **And it is this shell's own answer, not something every column does.**
// Measured the same day on a pseudo-terminal, over a script holding
// `sleep 30 &` and given `-m`: bash 5.3.20 and bash 3.2.57 write nothing,
// ksh93u+ writes nothing, dash writes nothing, and BusyBox ash 1.37.0 in the
// pinned alpine image writes nothing. dash is the one that needs saying
// twice, since it is *not* silent on the neighboring row — a **stopped** job
// at the end of the same script is `You have stopped jobs.` — which is a
// different question and is not moved by this axis.
//
// The engine's side of it is measured in interp, where a test can set the
// axis by hand and name no shell, and at a real invocation in cmd/zsh, where
// the monitor can actually be on. What is pinned here is this dialect's
// **answer**, because a script in this package cannot reach the state:
// `runZsh` has no terminal, so the monitor is off in every case it can build
// and a behavioral test here would measure the first row of the grid twice.

// The axis value, and that it is not the value four of the five hold.
func TestTheMonitorAloneAccountsForJobsAtExit(t *testing.T) {
	if got := zsh.Semantics().MonitorAloneAccountsForJobsAtExit; got != interp.Yes {
		t.Errorf("MonitorAloneAccountsForJobsAtExit = %v, want Yes", got)
	}
}

// And the two sentences it opens, which are this shell's own words and are
// the half that would be silently traded away by a gate that opened onto an
// empty wording.
//
// Both are already here — the hangup's since #4509 and the held exit's since
// long before — so what this guards is that the gate and the words do not
// drift apart: a dialect that answered the axis Yes while holding neither
// sentence would write nothing and look exactly like one that answered No.
func TestTheSentencesTheMonitorRouteWrites(t *testing.T) {
	d := zsh.Diagnostics()
	for _, c := range []struct{ name, got, want string }{
		{"the running jobs it is leaving", d.RunningJobsAtExit, "zsh: you have running jobs."},
		{"the suspended ones", d.StoppedJobsAtExit, "zsh: you have suspended jobs."},
		{"and what it hung up", d.JobsHUPedAtExit, "zsh: warning: 1 jobs SIGHUPed"},
	} {
		if got := interp.Wording(c.got, "", "zsh", 1); got != c.want {
			t.Errorf("%s renders %q, want %q", c.name, got, c.want)
		}
	}
}
