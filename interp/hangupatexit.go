// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "syscall"

// hangUpJobsIfAsked sends SIGHUP to every job this shell still has, for the
// dialect whose option asks for it.
//
// Three conditions, all measured — see SendsHangupToJobsAtExit for the rows:
// the option, an interactive shell, and a login shell. Any one of them absent
// and nothing is signaled, which is what the negative rows pin: a
// non-interactive login shell with the option set signals nothing, and neither
// does an interactive shell that is not a login shell.
//
// A failure to signal is dropped rather than reported. The shell is already
// leaving, the jobs are the operating system's business from here, and a job
// that finished between the last bookkeeping and this call is an ESRCH nobody
// asked about — bash says nothing in that case either.
//
// Not in a subshell: a clone's ending is not the session's, and its jobs are
// the parent's to account for.
func (r *Runner) hangUpJobsIfAsked() {
	if r.inSubshell || !r.hangUpJobsAtExit || !r.Interactive || !r.LoginShell {
		return
	}
	for _, j := range r.jobs {
		if j == nil || j.Finished() {
			// Only what is still there. A finished job has no process group
			// to reach, and reaching for one is how a shell ends up signaling
			// whatever was given the number next.
			continue
		}
		_ = r.signalJob(j, syscall.SIGHUP)
	}
}
