// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

// Signal traps.
//
// A shell does not run a handler the moment the signal arrives. It finishes
// what it is doing and runs the handler between commands — measured: a child
// started before the signal still prints its output first, in all four shells.
// So arrival is recorded here and delivery happens in stmt, which is the only
// place that knows a command has finished.
//
// Only the signals a script can sensibly catch are offered. KILL and STOP
// cannot be caught by anyone and are refused rather than accepted and quietly
// ignored.
var trappableSignals = map[string]syscall.Signal{
	"HUP":  syscall.SIGHUP,
	"INT":  syscall.SIGINT,
	"QUIT": syscall.SIGQUIT,
	"ABRT": syscall.SIGABRT,
	"ALRM": syscall.SIGALRM,
	"TERM": syscall.SIGTERM,
	"USR1": syscall.SIGUSR1,
	"USR2": syscall.SIGUSR2,
	"PIPE": syscall.SIGPIPE,
}

// signalNumbers is the other spelling: `trap … 2` is `trap … INT`.
var signalNumbers = map[string]string{
	"1": "HUP", "2": "INT", "3": "QUIT", "6": "ABRT",
	"13": "PIPE", "14": "ALRM", "15": "TERM",
}

// signalState is the machinery, shared by every runner in one process.
//
// It is behind a pointer for a reason the race detector found: clone copies a
// Runner by value, and a mutex must not be copied. Sharing is also the honest
// model — signal handlers are installed per *process*, and nothing here forks
// for a subshell.
type signalState struct {
	mu    sync.Mutex
	traps map[string]string
	ch    chan os.Signal
}

// sigs returns the shared state, creating it on first use.
func (r *Runner) sigs() *signalState {
	if r.signals == nil {
		r.signals = &signalState{traps: map[string]string{}}
	}
	return r.signals
}

// canonicalSignal reads a condition the way `trap` accepts it: a name with or
// without the SIG prefix, in any case, or a number.
func canonicalSignal(s string) (string, syscall.Signal, bool) {
	up := strings.ToUpper(strings.TrimPrefix(strings.ToUpper(s), "SIG"))
	if name, ok := signalNumbers[s]; ok {
		up = name
	}
	sig, ok := trappableSignals[up]
	return up, sig, ok
}

// trapSignal records what to run when a signal arrives.
//
// body is nil to restore the default, and a pointer to "" to ignore the
// signal — which are different things: an ignored signal does not kill the
// shell and does not run anything.
func (r *Runner) trapSignal(name string, sig syscall.Signal, body *string) {
	s := r.sigs()
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case body == nil:
		delete(s.traps, name)
		signal.Reset(sig)
	case *body == "":
		s.traps[name] = ""
		signal.Ignore(sig)
	default:
		s.traps[name] = *body
		if s.ch == nil {
			// Buffered so a burst is not lost between commands.
			s.ch = make(chan os.Signal, 32)
		}
		signal.Notify(s.ch, sig)
	}
}

// takePending drains what the runtime has delivered.
//
// Drained here rather than by a goroutine, because a goroutine introduces a
// window: `kill -INT $$` would return before the collector had run, and the
// handler would fire one command late — sometimes. The corpus cannot record a
// sometimes.
func (r *Runner) takePending() []string {
	s := r.signals
	if s == nil || s.ch == nil {
		return nil
	}
	var got []string
	for {
		select {
		case sig := <-s.ch:
			if name, ok := signalName(sig); ok {
				got = append(got, name)
			}
		default:
			return got
		}
	}
}

// signalName maps a delivered signal back to the name a trap was set under.
func signalName(s os.Signal) (string, bool) {
	sys, ok := s.(syscall.Signal)
	if !ok {
		return "", false
	}
	for name, want := range trappableSignals {
		if want == sys {
			return name, true
		}
	}
	return "", false
}

// runPendingTraps runs the handlers for whatever arrived, between commands.
//
// The status is put back afterwards: a handler that runs commands of its own
// must not change what `$?` reports to the script it interrupted. Which status
// the *handler* sees is an axis — zsh shows the one from before the command
// that triggered it, the other three the one that command produced.
func (r *Runner) runPendingTraps(ctx context.Context) {
	for _, name := range r.takePending() {
		s := r.sigs()
		s.mu.Lock()
		body, ok := s.traps[name]
		s.mu.Unlock()
		if !ok || body == "" {
			continue
		}
		outer := r.status
		if r.ask(r.sem().SignalHandlerSeesEarlierStatus, "the status a signal handler sees") {
			r.status = r.statusBefore
		}
		ctl := r.ctl
		r.ctl = controlNone
		r.runTrapBody(ctx, body)
		if r.ctl == controlNone {
			r.ctl = ctl
			r.status = outer
		}
	}
}

// stopSignals releases the handlers a runner installed.
func (r *Runner) stopSignals() {
	s := r.signals
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ch != nil {
		signal.Stop(s.ch)
		s.ch = nil
	}
}
