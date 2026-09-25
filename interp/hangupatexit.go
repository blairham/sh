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
	if r.inSubshell || !r.hangUpJobsAtExit || !r.Interactive {
		return
	}
	if r.sem().HangupAtExitNeedsALoginShell == Yes && !r.LoginShell {
		return
	}
	skipStopped := r.sem().HangupAtExitSkipsStoppedJobs == Yes
	sent := 0
	for _, j := range r.jobs {
		if j == nil || j.Finished() {
			// Only what is still there. A finished job has no process group
			// to reach, and reaching for one is how a shell ends up signaling
			// whatever was given the number next.
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
		r.errf("%s\n", Wording(w, "", r.name(), sent))
	}
}
