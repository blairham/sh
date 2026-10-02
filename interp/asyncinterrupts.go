// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os/signal"
	"sync"
	"syscall"
)

// An asynchronous list in a shell without job control runs with SIGINT and
// SIGQUIT ignored, which is the standard's rule and every shell in the panel's
// behavior. Measured 2026-10-02 with `sleep 1 & sleep 0.2; kill -SIG $!;
// sleep 0.2; kill -0 $!` under `-c`: bash 5.3.20, dash, ksh93u+ and zsh 5.9.2
// all leave the job alive for INT and for QUIT, and `jobs` in bash still
// lists it Running (#5414).
//
// A real shell gets this by forking and setting the two dispositions in the
// child. A Runner has no fork of its own — a background body is a goroutine —
// so the dispositions are set where a fork would inherit them: on the process,
// for the length of the start of each program the body runs. That is process
// state, which this package may not change on its own say-so, so the change
// is the front end's hook, StartIgnoringInterrupts, and where it is nil the
// child starts with the signals the process has.
//
// The body's own handling is not this. A signal aimed at a background body
// with no process of its own reaches its inbox, which INT and QUIT never end
// — see endsABodyByDefault.

// asyncSpawn orders the starts that change the process's dispositions against
// every other start: a program started in the window would inherit the
// ignore. A start that changes nothing shares the lock; one that ignores
// takes it alone.
var asyncSpawn sync.RWMutex

// startWithAsyncDispositions runs start with the dispositions this runner's
// children are owed.
func (r *Runner) startWithAsyncDispositions(start func() error) error {
	if !r.asyncIgnoresInterrupts || r.StartIgnoringInterrupts == nil ||
		(signal.Ignored(syscall.SIGINT) && signal.Ignored(syscall.SIGQUIT)) {
		asyncSpawn.RLock()
		defer asyncSpawn.RUnlock()
		return start()
	}
	asyncSpawn.Lock()
	defer asyncSpawn.Unlock()
	err := r.StartIgnoringInterrupts(start)
	// The ignore undid every subscription the process had for the two, the
	// script's own traps among them, so those go back as well.
	r.resubscribeTrapped(syscall.SIGINT, syscall.SIGQUIT)
	return err
}

// resubscribeTrapped puts back this shell's own subscription for each of
// sigs that a trap holds an action for.
func (r *Runner) resubscribeTrapped(sigs ...syscall.Signal) {
	s := r.signals
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sig := range sigs {
		name, ok := signalName(sig)
		if !ok {
			continue
		}
		if action, trapped := s.traps[name]; trapped && action != "" {
			signal.Notify(s.ch, sig)
		}
	}
}
