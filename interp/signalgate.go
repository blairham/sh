// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "syscall"

// Every signal this interpreter asks the kernel to deliver comes through here,
// so the whole category passes the gate and reaches the event stream.
//
// It matters for the reason the file probes do, and rather more sharply: `kill
// -9 1234` leaves this process and ends another one, and while it went to
// syscall.Kill directly no policy could refuse it and no audit trail recorded
// it. Running /bin/kill would have been gated as the exec it is; the builtin
// that replaced it was not, so implementing a builtin quietly moved an action
// from inside the boundary to outside it.
//
// The gate sits at the system call rather than at the builtin, which is what
// draws the line in the right place. A signal a script aims at this shell
// alone never gets here at all — sendSignal answers it out of the trap table
// or stops the script — because nothing that stays inside one process is an
// action that leaves it.

// killProcess is syscall.Kill through the gate.
//
// A refusal is EPERM: the errno for a process this one may not signal. Not an
// error of the gate's own, and not a diagnostic of its own either — `kill`
// already reports EPERM in each dialect's wording, so a refused target reads
// exactly as a target the kernel would not let us have. That is the same
// bargain a denied stat makes by answering ENOENT, for the same reason: a
// refusal that identifies itself is an oracle for what the policy is hiding.
func (r *Runner) killProcess(pid int, sig syscall.Signal) error {
	if !r.signalAllowed(pid, sig) {
		return syscall.EPERM
	}
	return syscall.Kill(pid, sig)
}

// signalAllowed consults the gate about one signal and records it either way.
//
// The access record is emitted before the call rather than after it, unlike an
// open's: the auditable act is aiming a signal at a process, and whether the
// target was still there is the answer rather than the act — `kill -0` exists
// to ask exactly that question.
func (r *Runner) signalAllowed(pid int, sig syscall.Signal) bool {
	a := Action{Kind: ActionSignal, PID: pid, Signal: sig}
	if r.Gate != nil && r.Gate.Allow(r.ctx, a) == Deny {
		r.emit(r.ctx, Event{Kind: EventDenied, Action: a})
		return false
	}
	r.emit(r.ctx, Event{Kind: EventAccess, Action: a})
	return true
}
