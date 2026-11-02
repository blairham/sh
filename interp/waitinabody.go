// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "syscall"

// A trapped signal that reaches a **body** sitting in `wait` cuts the wait
// short there exactly as it does at the top level: `wait` returns 128+N at
// once, and the body's handler runs right after it (#7086). POSIX says so for
// "the shell", and a body is one — a fork with a trap table of its own.
//
// It did not here, because a body's wait blocked on the job alone. What a
// body is sent goes to lists of its own rather than to the shell's — the
// inbox, for a signal the shell aimed at the job (bodyinbox.go), and the
// body's process, for one a copy of the body aimed at its number
// (selfkillbody.go) — and the shell's wake, which the top level's wait
// selects on, belongs to the shell's list. So the handler waited for the
// inner job to end, and `wait` reported 0. Each of those lists now pokes a
// wake of its own as it takes a handled arrival, and a body's `wait` selects
// on both and asks the lists what came, which keeps the lists the one record
// of what arrived.
//
// Measured 2026-11-02 under `-c`, each shell in a process group of its own,
// against bash 5.3.20, bash 3.2.57, zsh 5.9.2, ksh93u+ and dash; the body's
// pid from `$BASHPID`, `$sysparams[pid]` or a child's `$PPID`:
//
//	( trap "echo T" TERM; sleep 1 & wait; echo after $? ) & sleep 0.3
//	kill $!; wait %1; echo st=$?
//	        T, after 143, st=0 at 0.3 s; ksh93 after 271
//	        here, before: T, after 0 at 1.0 s, the inner job waited out
//	… wait $! or wait %1 in place of the body's wait
//	        after 143; ksh93 after 1, its status for a named wait cut
//	        short (Semantics.WaitForAJobFailsWhenInterrupted)
//	( trap "echo U" USR1; p=<pid>; ( sleep 0.1; kill -USR1 $p; sleep 0.4 ) &
//	  wait $!; echo w=$? )
//	        U, w=158 at 0.1 s; ksh93 w=1 — the sender a copy of the body
//	        here, before: U, w=0 at 0.5 s
//
// Not an ignored signal, which queues nothing — `trap "" TERM` in the first
// row waits the second out at 0 in all five — and not one left untrapped,
// which ends the body by the death that path already has. Nor CHLD, which is
// a split: bash 5.3, zsh and ksh93 wait the inner job out with `kill -CHLD`
// sent to the body, and dash cuts the wait short at 148. This shell waits it
// out in every dialect, as the top level's wait does (see pendingTrap).
//
// **And an arrival a `&` job raised before its `$!` was settled waits past
// the shell's next moment between commands.** A real shell forks the job and
// goes straight on, so the job's `kill` lands while the shell is already in
// whatever came next. Here the shell waits for the job's pid to settle
// (Runner.background), the job runs its `kill` meanwhile, and the arrival was
// answered at the very next statement — before a `wait` there had begun, so
// the wait waited the job out. Measured the same way:
//
//	( trap "echo U" USR1; p=<pid>; kill -USR1 $p & wait $!; echo w=$? )
//	        U, w=158 in bash 5.3, zsh and dash; ksh93 U, w=1
//	        here, before: U, w=0
//	( trap "echo U" USR1; p=<pid>; ( kill -USR1 $p; sleep 0.3 ) & wait; echo w=$? )
//	        U, w=158 at once; ksh93 w=286; here, before: w=0 at 0.3 s
//	trap "echo U" USR1; kill -USR1 $$ & wait $!; echo w=$?
//	        U, w=158 in all but ksh93, w=1; here, before: U, w=0
//	trap "echo U" USR1; ( kill -USR1 $$; sleep 0.5 ) & sleep 0.2; wait $!
//	        U at 0.2 s, once the foreground `sleep` is done, and then the
//	        wait waits the job out; here, before: U at once
//	trap "echo U" USR1; kill -USR1 $$ &
//	        nothing in all five: the shell is gone before the signal lands
//
// bash 3.2 cannot reach the body rows, having no `$BASHPID`. A death is not
// held: it is taken before the handlers are, at the same moment as before.

// aBodysTrappedArrival is what ends a body's `wait`: a signal waiting for this
// body that its own table answers with a handler — sent by the shell to the
// job, by a copy of the body to its number, or raised by the body itself.
func (r *Runner) aBodysTrappedArrival() (syscall.Signal, bool) {
	handled := func(key string) bool {
		return key != "CHLD" && key != "CLD" && r.traps[key] != ""
	}
	for _, key := range r.selfPending {
		if handled(key) {
			return trapConditionSignal(key), true
		}
	}
	if r.inbox != nil {
		if key, ok := r.inbox.firstPending(handled); ok {
			return trapConditionSignal(key), true
		}
	}
	if r.copiesSignaled() {
		if a, ok := r.process.firstArrival(func(a bodyArrival) bool {
			return catchableSignal(a.sig) && handled(trapKey(a.name, a.sig))
		}); ok {
			return a.sig, true
		}
	}
	return 0, false
}

// bodysWakes are what is poked as an arrival reaches one of this body's
// lists. Nil where the body has no such list, which a select never takes.
func (r *Runner) bodysWakes() (fromTheShell, fromACopy <-chan struct{}) {
	if r.inbox != nil {
		fromTheShell = r.inbox.wake
	}
	if r.process != nil {
		fromACopy = r.process.wake
	}
	return fromTheShell, fromACopy
}

// firstPending is the first arrival waiting in the inbox that handled says
// yes to.
func (b *bodyInbox) firstPending(handled func(string) bool) (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, key := range b.pending {
		if handled(key) {
			return key, true
		}
	}
	return "", false
}

// firstArrival is the first signal a copy left this body that want says yes
// to.
func (p *Process) firstArrival(want func(bodyArrival) bool) (bodyArrival, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, a := range p.arrivals {
		if want(a) {
			return a, true
		}
	}
	return bodyArrival{}, false
}

// pokeWake tells a waiter to look again, without blocking: one token says
// everything ten would. See signalState.poke.
func pokeWake(wake chan struct{}) {
	select {
	case wake <- struct{}{}:
	default:
	}
}

// anArrivalWaits reports that something has arrived for this shell — this
// body, where it is one — that its next moment between commands would answer.
func (r *Runner) anArrivalWaits() bool {
	if r.inSubshell {
		return len(r.selfPending) > 0 || r.copiesSignaled() ||
			(r.inbox != nil && r.inbox.hasPending())
	}
	s := r.signals
	return s != nil && !s.nothingArrived()
}

// hasPending reports that a handled arrival is waiting in the inbox.
func (b *bodyInbox) hasPending() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending) > 0
}

// holdWhatTheJobRaised notes, as a `&` finishes starting its job, that
// something arrived while the shell waited for the job's pid: that arrival is
// left for the moment after the next one. arrivedBefore is whether something
// was already waiting as the job started, which is not the job's and is not
// held. See the measurements above.
func (r *Runner) holdWhatTheJobRaised(arrivedBefore bool) {
	if !arrivedBefore && r.anArrivalWaits() {
		r.arrivalsHeldPastTheNextMoment = true
	}
}

// takeTheHold reports, once, that the handlers waiting are held past this
// moment. See holdWhatTheJobRaised.
func (r *Runner) takeTheHold() bool {
	held := r.arrivalsHeldPastTheNextMoment
	r.arrivalsHeldPastTheNextMoment = false
	return held
}
