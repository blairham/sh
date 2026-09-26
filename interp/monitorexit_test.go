// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.MonitorAloneAccountsForJobsAtExit: a shell that is leaving says
// what it is leaving behind, with the **monitor** on and nobody at a prompt.
//
// The state under test is the one a script is in after `set -m` — the monitor
// is running and there is nobody to hold an exit for. Exactly one column of
// the panel writes anything from there and four say nothing, so the gate
// cannot be Runner.JobControl, which is a prompt and nothing else.
//
// This file names no shell, which is the rule for a test in this package: the
// four rows of the panel are recorded on the axis and in each dialect's own
// vector, and what is asked here is that the axis moves the engine.

// The two sentences, as wordings a dialect could hold. Distinctive enough
// that a test cannot mistake one for the other, and written here rather than
// taken from a preset so that this file asserts on the engine's choice and
// not on anybody's words.
const (
	monitorExitRunning = "TESTSH-RUNNING-JOBS"
	monitorExitHUPed   = "TESTSH-HUPED"
)

// monitorExitShell is a shell about to leave with one `&` job still in its
// table, and a sink it writes its parting sentences to.
//
// Both switches are on, because what is under test is the gate in front of
// them rather than either of them: Runner.ChecksRunningJobsAtExit is the
// `checkjobs` half and Runner.SendsHangupToJobsAtExit is the `huponexit`
// half, and a row that left one off could not tell a gate that closed from a
// switch that was never thrown.
func monitorExitShell(t *testing.T, jobControl bool, axis Answer) (*fakeJobs, *Runner, *os.File) {
	t.Helper()
	f, r := hupJobsAt(t, jobControl, false, false, true,
		func(s *Semantics, d *Diagnostics) {
			s.MonitorAloneAccountsForJobsAtExit = axis
			// No login shell required, so the row is about this axis and not
			// about that one. The dialect that answers this one Yes also
			// answers that one No.
			s.HangupAtExitNeedsALoginShell = No
			s.HangupAtExitSkipsStoppedJobs = Yes
			d.RunningJobsAtExit = monitorExitRunning
			d.JobsHUPedAtExit = monitorExitHUPed + " %[2]d"
		})
	r.SetChecksRunningJobsAtExit(true)
	r.SetChecksStoppedJobsAtExit(true)
	out := sink(t)
	r.Stdout, r.Stderr = out, out
	return f, r, out
}

// monitorExitLeave runs the source, ends the shell, and hands back everything
// it wrote on the way out.
//
// Finish explicitly, because jobRun2 uses RunPart — a shell that has not
// ended — and both sentences are things a shell says *as* it ends. The sink
// is read after Finish rather than by jobRun2, which swaps its own in and
// restores ours before the shell is ended.
func monitorExitLeave(t *testing.T, r *Runner, out *os.File, src string) string {
	t.Helper()
	jobRun2(t, r, src)
	r.Finish(t.Context())
	b, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Both answers, and the sentences move together because the axis is the one
// gate in front of both.
func TestTheMonitorAloneMayAccountForJobsAtExit(t *testing.T) {
	for _, tc := range []struct {
		why     string
		axis    Answer
		account bool
	}{
		{"the monitor is enough", Yes, true},
		{"the monitor is not enough, which is four of the five", No, false},
		// Read and not asked, for the reason the axis gives: a preset that
		// has not chosen had better say nothing than write "the shells
		// disagree here" as a shell leaves.
		{"unanswered reads as the quieter answer", Unspecified, false},
	} {
		t.Run(tc.why, func(t *testing.T) {
			f, r, out := monitorExitShell(t, false, tc.axis)
			got := monitorExitLeave(t, r, out, "/usr/bin/true &")
			for _, want := range []string{monitorExitRunning, monitorExitHUPed} {
				if strings.Contains(got, want) != tc.account {
					t.Errorf("wrote %q; %q present = %v, want %v",
						got, want, strings.Contains(got, want), tc.account)
				}
			}
			// And the signal itself, not only the sentence about it: a gate
			// that let the wording through while sending nothing, or sent
			// while saying nothing, would pass half of the row above.
			want := 0
			if tc.account {
				want = 1
			}
			if n := hangups(f); n != want {
				t.Errorf("%d hangups, want %d", n, want)
			}
		})
	}
}

// And a prompt is still enough on its own, in a dialect that does not count
// the monitor.
//
// This is the half that would go missing if the new grant replaced the old
// one rather than joining it: every shell in the panel accounts for its jobs
// at a prompt, and only one of them does it from a script. The hangup read
// Runner.Interactive before #4542 and this row is what holds that ground.
func TestAPromptStillAccountsForJobsWithoutTheMonitorAxis(t *testing.T) {
	f, r, out := monitorExitShell(t, true, No)
	got := monitorExitLeave(t, r, out, "/usr/bin/true &")
	if !strings.Contains(got, monitorExitHUPed) {
		t.Errorf("the session wrote %q, want the hangup reported", got)
	}
	if n := hangups(f); n != 1 {
		t.Errorf("%d hangups, want 1", n)
	}
	// The prompt route writes its own sentence through HoldsExitForJobs, from
	// the `exit` builtin and from the end of input, rather than through the
	// one this axis opens — so what is asserted here is the hangup, and the
	// prompt's sentence has tests of its own beside HoldsExitForJobs.
}

// The two sentences are two mechanisms with two switches, and the axis is one
// gate in front of both rather than a switch of its own.
//
// Measured 2026-09-25 against the one shell that reaches this, on a
// pseudo-terminal over a `-m` script with a running job: turning the
// check-jobs half off leaves the hangup sentence alone, turning the hangup
// half off leaves the check-jobs sentence alone, and turning the monitor off
// removes both. A single switch modeled for the pair would pass the first row
// of this table and fail the other two.
func TestTheTwoPartingSentencesHaveTheirOwnSwitches(t *testing.T) {
	for _, tc := range []struct {
		why                   string
		checkJobs, hangUp     bool
		wantRunning, wantHUPd bool
	}{
		{"both switches on", true, true, true, true},
		{"the check-jobs half off", false, true, false, true},
		{"the hangup half off", true, false, true, false},
		{"both off", false, false, false, false},
	} {
		t.Run(tc.why, func(t *testing.T) {
			_, r, out := monitorExitShell(t, false, Yes)
			r.SetChecksRunningJobsAtExit(tc.checkJobs)
			r.SetChecksStoppedJobsAtExit(tc.checkJobs)
			r.SetSendsHangupToJobsAtExit(tc.hangUp)
			got := monitorExitLeave(t, r, out, "/usr/bin/true &")
			if strings.Contains(got, monitorExitRunning) != tc.wantRunning {
				t.Errorf("wrote %q, want the running-jobs sentence = %v", got, tc.wantRunning)
			}
			if strings.Contains(got, monitorExitHUPed) != tc.wantHUPd {
				t.Errorf("wrote %q, want the hangup sentence = %v", got, tc.wantHUPd)
			}
		})
	}
}
