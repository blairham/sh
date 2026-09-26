// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "syscall"

// A signal a script aims at **the pid the running shell answers as its own**
// is delivered to that shell's traps.
//
// That sentence has one noun in it and the noun is the *pid*, not the context.
// sendSignal used to key the same rule on `os.Getpid()`, which is the process,
// and the process is the shell only at the top level. Inside a body a real
// shell would have forked, this shell answers "which process am I" with the
// group it leads rather than with the process it is in — see procanchor.go and
// dialect/zsh's subshellPid — so a subshell that killed the number it had just
// been given aimed a real signal at the group's placeholder leader, which is
// not a shell, has no traps, and dies without anything noticing (#4596).
//
// Measured 2026-09-26 against zsh 5.9.2 and bash 5.3.20, both `/opt/homebrew`,
// both `go version -m` → not a Go executable:
//
//	                                          zsh 5.9.2   bash 5.3   here, before
//	( trap 'print T; exit 19' TERM
//	  kill <own pid> )                        T, 19       T, 19      ret=0, 0
//	( trap 'exit 11' USR1
//	  kill -USR1 <own pid> )                  11          11         0
//	( kill <own pid> )        — untrapped     143         143        0
//	( trap 'print E' EXIT
//	  kill <own pid> )        — untrapped     143, no E   143, no E  0, E
//	( kill -KILL <own pid> )                  137         137        0
//	( trap 'print T' TERM
//	  kill $$ )               — the parent    parent dies, sender runs on: unchanged
//
// The last row is the pair that holds the noun fixed. Same construct, same
// context, same signal, same trap — only the *number* differs, and both real
// shells move: the body's own number runs the body's trap, the parent's number
// kills the parent and the body carries on. A rule keyed on "am I in a
// subshell" cannot tell those two apart, and one keyed on `os.Getpid()` gets
// the second right and the first wrong, which is exactly what was shipped.
//
// **This is not a dialect split.** The issue reported that real bash does not
// fire the trap on `( trap … TERM; kill $BASHPID )`; measured here it does,
// both as a script file and as the last command of a `-c`, with the same
// status and the same untrapped 143. So the two shells that can reach the
// shape at all agree about it, there is nothing to ask a Semantics axis, and
// the fix is the core's. `$BASHPID` and `$sysparams[pid]` are the only two
// ways in — both answer through [Runner.SubshellProcessGroup] — so nothing in
// the three dialects without such a parameter can reach this code.
//
// Nothing leaves the process, which is the rule `interp` already holds for
// `$$` and holds for the same reason: a line of script must not be able to
// fire an embedder's handlers. The anchor is *not* signaled either, and that
// is the other half of the fix rather than an omission — it is a placeholder
// whose only job is to exist, and a script that has just been handed the group
// id may still be about to start something in it.

// aimedAtThisBody reports that a pid names the forked body this runner *is*,
// rather than the process it runs in or anyone else's.
//
// The group is read without starting one, and that is not an optimisation: a
// body that has never been asked which process it is has no anchor, and a
// script that was never given the number cannot have named it. Starting one
// here to compare against would fork a process for every `kill` in every
// subshell in order to learn that the answer is no.
func (r *Runner) aimedAtThisBody(pid int) bool {
	a := r.bodyAnchor
	if a == nil || pid <= 0 {
		return false
	}
	a.mu.Lock()
	pgid := a.pgid
	a.mu.Unlock()
	// This body's own and never the one around it. `existing` walks outward
	// so that a command a body starts joins the group it is contained in;
	// identity does not walk, because a real shell forked again for the inner
	// body and the inner body is not the outer one.
	return pgid > 0 && pgid == pid
}

// signalThisBody settles a signal the running forked body aimed at itself.
//
// The shape of the switch is sendSignal's, deliberately: it is the same
// question asked of a shell that has a group instead of a process, so the
// answers line up case for case and a change to one is visible as a difference
// from the other. What differs is which table decides and which shell ends.
//
//   - The table is this body's own. A subshell starts with the parent's
//     handled signals back at their defaults, so the parent's trap is exactly
//     what must not answer here — see trapsubshell.go.
//   - The death is this body's own. signalDeath on the body sets its status to
//     128+N and stops it, and the parent reads that status out of the
//     parentheses as a real shell reads it out of a fork. It does not reach
//     recordSharedDeath, which is where a signal aimed at `$$` goes and which
//     ends the process.
//
// The arrival goes on the body's own pending list rather than the shared one,
// for the reason brokenPipeAbsorbed puts a broken pipe there: the process
// never had this signal, and the handler that answers it is the one the body
// set. See Runner.runPendingTraps, which drains that list between the body's
// commands — so the handler runs before the `kill` line's own status is read,
// which is the measured order in both references.
func (r *Runner) signalThisBody(name string, sig syscall.Signal) error {
	key := trapKey(name, sig)
	body, trapped := r.trapTable()[key]
	if !catchableSignal(sig) {
		// KILL and STOP leave a listing rather than a handler, exactly as at
		// the top level: `( trap 'print T' TERM; kill -KILL <own pid> )` is
		// 137 and prints nothing in both references. See sendSignal.
		trapped = false
	}
	switch {
	case trapped && body != "":
		r.selfSignaledBody(key)
	case !trapped && fatalSignal(name, sig) && r.untrappedSignalIgnored(name):
		// The default action was taken away from the kernel and nothing put
		// in its place, so the raise does not happen. Reported as sent,
		// because the builtin succeeded.
	case !trapped && fatalSignal(name, sig):
		r.signalDeath(name, sig)
	default:
		// An ignored signal, and the ones whose default action suspends or
		// resumes rather than ends. sendSignal hands those to the kernel
		// because the process really can be stopped; a forked body cannot,
		// since it is a goroutine, and the only process this pid names is a
		// placeholder that is not running the script. Stopping *that* would
		// suspend nothing the script can see and would take the group's
		// leader away from a body still using it, so nothing is sent.
	}
	return nil
}

// selfSignaledBody records a signal the running body aimed at itself.
//
// selfSignaled's argument, one boundary in: recorded rather than sent, so
// delivery is deterministic by construction instead of by whether os/signal's
// goroutine got to run — and on the body's list, because the process never had
// the signal and the parent must not run its handler for it.
func (r *Runner) selfSignaledBody(key string) {
	r.selfPending = append(r.selfPending, key)
	s := r.sigs()
	s.mu.Lock()
	defer s.mu.Unlock()
	// A `wait` blocked elsewhere re-reads what is pending when it is poked,
	// and the poke is cheap enough not to be worth deciding about.
	s.poke()
}
