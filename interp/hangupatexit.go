// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "syscall"

// hangUpJobsIfAsked sends SIGHUP to the jobs this shell still has, for a
// dialect whose option asks for it — and says how many it sent to, where the
// dialect has words for that.
//
// Two shells reach this and they are not the same shell. The option is the
// switch in both — bash's `shopt -s huponexit`, zsh's `setopt hup`, which zsh
// starts on — and everything around it is an axis, because bash and zsh
// answer each of the three differently:
//
//   - whether a login shell is required as well as an interactive one, which
//     is Semantics.HangupAtExitNeedsALoginShell;
//   - whether a stopped job is left out of the send, which is
//     Semantics.HangupAtExitSkipsStoppedJobs;
//   - whether any of it happens before the EXIT trap, which is
//     Semantics.HangupAtExitPrecedesTheExitTrap and is read by Finish rather
//     than here.
//
// Whether there is anybody to account to is Runner.accountsForJobsAtExit,
// which is a prompt or — in the one dialect that says so — the monitor on its
// own. It read Runner.Interactive until #4542, and the two agree in every
// shell a person sits at, because a session turns the monitor on: what that
// reading missed was a `zsh -fm` script, which hangs its jobs up and says so
// with no prompt anywhere.
//
// One function for both, rather than a second one beside it. A dialect that
// grew its own copy would be a copy that had to acquire each later fix
// separately, which is how the first one loses them.
//
// A failure to signal is dropped rather than reported, but it is not counted
// either: the sentence says how many jobs were SIGHUPed, so a job with no
// process of its own — a builtin or a compound command on a cloned runner —
// is outside the number as well as outside the send. The shell is already
// leaving and a job that finished between the last bookkeeping and this call
// is an ESRCH nobody asked about; bash says nothing in that case and neither
// does this.
//
// Not in a subshell: a clone's ending is not the session's, and its jobs are
// the parent's to account for.
func (r *Runner) hangUpJobsIfAsked() {
	if r.inSubshell || !r.hangUpJobsAtExit || !r.accountsForJobsAtExit() {
		return
	}
	if r.sem().HangupAtExitNeedsALoginShell == Yes && !r.LoginShell {
		return
	}
	skipStopped := r.sem().HangupAtExitSkipsStoppedJobs == Yes
	sent := 0
	for _, j := range r.jobs {
		if j == nil {
			continue
		}
		if j.startedWithoutMonitor && r.sem().HangupAtExitSkipsJobsStartedWithoutTheMonitor == Yes {
			// See Semantics.HangupAtExitSkipsJobsStartedWithoutTheMonitor.
			continue
		}
		if j.Finished() && !r.finishedJobCountsAtExit(j) {
			continue
		}
		if j.Finished() {
			// Counted, and not sent to. A job that finished and was never
			// reported is still in the table, and zsh counts the table.
			// Measured 2026-10-05 through a pty, `zsh -f -c` against zsh
			// 5.9.2 (#6125): `set -m; setopt nonotify; sleep 0.1 & /bin/sleep
			// 0.5; echo end` ends with `warning: 1 jobs SIGHUPed`, and two
			// finished jobs beside a running one is `3 jobs`. Once `jobs` has
			// reported it, it is gone and the warning is too. The table here
			// holds a finished job only until it is reported, and
			// finishedJobCountsAtExit leaves out the ones whose report went
			// to nobody.
			//
			// Not signaled: it has no process group left to reach, and
			// reaching for one is how a shell ends up signaling whatever was
			// given the number next. Only the one dialect with the sentence
			// reads the count.
			//
			// And the shell leaves with 1, whatever the last command left:
			// the hangup that reached nothing is a failure, and it is the
			// last thing the shell does (#6170). Measured 2026-10-05 through
			// a pty against zsh 5.9.2: `f(){return 5}; set -m; setopt
			// nonotify; sleep 0.1 & /bin/sleep 0.5; echo end; f` leaves with
			// 1, and an EXIT trap there still sees 0; with only a running
			// job counted it leaves with 5; with `nohup`, or `set +m` before
			// the end, nothing is counted and it leaves with 5; and with
			// both streams sent to /dev/null it still leaves with 1, so it
			// is the count and not the sentence that decides.
			sent++
			r.status = 1
			continue
		}
		if j.Stopped && skipStopped {
			continue
		}
		if r.signalJob(j, syscall.SIGHUP) == nil {
			sent++
		}
	}
	if sent == 0 {
		// Nothing received anything, so there is nothing to report — and a
		// session that ends with an empty job table must not acquire a line
		// it never had. Measured: the reference is silent on both.
		return
	}
	if w := r.diag().JobsHUPedAtExit; w != "" {
		// Written plainly, exactly as the held-exit sentence beside it is and
		// for the same reason: the shell names itself in the wording and
		// there is no line number for a prompt's diagnostics to carry.
		r.noticef("%s\n", Wording(w, "", r.jobsHungUpName(), sent))
	}
}

// finishedJobCountsAtExit reports whether a finished job still in the table
// counts toward the hangup warning. It does unless its report has already
// been made to nobody: with NOTIFY at its default, `zsh -c 'set -m; sleep 0.1
// & /bin/sleep 0.5; echo end'` writes no warning, and with `setopt nonotify`
// it writes one. NOTIFY turned back on after the job finished still counts it.
// Measured 2026-10-05 against zsh 5.9.2 through a pty (#6125). A job not yet
// noticed is judged under the option as it stands now.
//
// Only where a command string ran off its end. Every other way out reports the
// job instead, `[1]  + done  sleep 0.1`, and counts nothing for it: an
// explicit `exit` in the string, a script file with or without one, and an
// interactive `exit`, all measured the same day. reportFinishedJobsAtExit
// writes that report (#6148).
func (r *Runner) finishedJobCountsAtExit(j *Job) bool {
	if r.Route != RouteCommandString || r.exitRan || !r.diag().JobsAtExitOnACommandString {
		return false
	}
	return r.finishedJobStillOwed(j)
}

// finishedJobStillOwed reports whether a finished job still in the table has
// had no report, not even one made to nobody. A job not yet noticed is judged
// under NOTIFY as it stands now. See finishedJobCountsAtExit for the
// measurements.
func (r *Runner) finishedJobStillOwed(j *Job) bool {
	if r.noticedJobs[j] {
		return !j.reportedToNobody
	}
	return !r.monitor || !r.reportsFinishedJobsToNobody()
}

// reportFinishedJobsAtExit writes the notices the finished jobs are still
// owed, as the shell leaves, and lets those jobs go. See
// Diagnostics.FinishedJobsReportedAtExit.
//
// Called by `exit` before it asks whether to hold for a running job, which is
// atTheExit, and by Finish before the sentence about jobs left behind, which
// is the order the lines come in. Reported jobs leave the table, so the second
// call and the hangup after both find nothing more to say about them.
func (r *Runner) reportFinishedJobsAtExit(atTheExit bool) {
	if r.inSubshell || !r.monitor || !r.diag().FinishedJobsReportedAtExit {
		return
	}
	if r.Route == RouteCommandString && !atTheExit && !r.exitRan && r.diag().JobsAtExitOnACommandString {
		// The command string that ran off its end counts the job in its
		// hangup warning instead. See finishedJobCountsAtExit.
		return
	}
	r.reapJobs()
	for _, line := range r.takeFinishedJobNoticesWhere(r.finishedJobStillOwed) {
		r.printf("%s\n", line)
	}
}
