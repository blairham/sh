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
// Every signal but the two nobody can catch is offered. KILL and STOP are
// refused rather than accepted and quietly ignored, which is a deliberate
// divergence: all four shells take `trap … KILL` and then never fire it.
//
// The set is derived rather than listed, because a list is what it was — nine
// entries that left `trap 'x' CONT` refused as uncatchable in a shell where
// all four panel members catch it.
var trappableSignals = func() map[string]syscall.Signal {
	m := make(map[string]syscall.Signal, len(knownSignals))
	for _, k := range knownSignals {
		if k.Sig == syscall.SIGKILL || k.Sig == syscall.SIGSTOP {
			continue
		}
		m[k.Name] = k.Sig
	}
	return m
}()

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
	// pending is what the shell already knows has arrived, ahead of the
	// runtime telling it. Only `kill` puts anything here — see selfSignaled.
	pending []string
}

// sigs returns the shared state, creating it on first use.
func (r *Runner) sigs() *signalState {
	if r.signals == nil {
		r.signals = &signalState{traps: map[string]string{}}
	}
	return r.signals
}

// signalWord is what a `trap` condition turned out to name, which is three
// answers rather than two: a word can name a signal this shell will catch, a
// real signal nobody can catch, or nothing at all. The middle one is a
// different complaint from the last, and only the last is the dialect's to
// word.
type signalWord int

const (
	signalTrappable signalWord = iota
	signalUncatchable
	signalUnknown
)

// canonicalSignal reads a condition the way `trap` accepts it: a name with or
// without the SIG prefix, in any case, or a number.
//
// Whether the prefix is part of a name at all is a dialect's answer rather
// than this function's — see Semantics.SIGPrefixAccepted — and it is asked
// only where it decides something. A bare name never asks, and neither does
// `SIGNOPE`, which names no signal with the prefix taken off either.
func (r *Runner) canonicalSignal(s string) (string, syscall.Signal, signalWord) {
	up := strings.ToUpper(s)
	if name, ok := signalNumbers[s]; ok {
		up = name
	} else if trimmed, had := strings.CutPrefix(up, "SIG"); had && knownSignal(trimmed) {
		if !r.ask(r.sem().SIGPrefixAccepted, "the SIG prefix on a signal name") {
			// Not a name this dialect has, so it names nothing — which is
			// what dash reports it as.
			return "", 0, signalUnknown
		}
		up = trimmed
	}
	if sig, ok := trappableSignals[up]; ok {
		return up, sig, signalTrappable
	}
	if knownSignal(up) {
		return up, 0, signalUncatchable
	}
	return up, 0, signalUnknown
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

// selfSignaled records a signal a script aimed at this shell.
//
// It is recorded rather than sent, and that is the fix. os/signal forwards
// what the runtime catches on a goroutine of its own, so a drain that only
// reads the channel is asking a question whose answer depends on the
// scheduler. Under load that goroutine could still be waiting to run when the
// script ended, and a trapped signal was then not late but *lost* — measured
// at three failures in three hundred runs on a busy machine and none at all
// on an idle one, which is the shape that had this looking like a flaky
// corpus case rather than a defect.
//
// So the trip through the kernel is not taken. A signal a script sends the
// shell is for the shell's own traps: routing it through the process would
// mean a Runner embedded in another program could fire *that* program's
// handlers with a line of script, and would buy nothing, because the shell
// already knows what it just sent. Which leaves the delivery deterministic by
// construction rather than by timing.
//
// Everything aimed anywhere else is a real signal to a real process — see
// sendSignal for the three cases and which of them still make the call.
func (r *Runner) selfSignaled(name string) {
	s := r.sigs()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = append(s.pending, name)
}

// takePending reports the arrivals the handler has not run yet.
//
// What the shell recorded itself comes first and is already in order; what the
// runtime forwarded — a signal from some other process — is drained after it.
func (r *Runner) takePending() []string {
	s := r.signals
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	got := s.pending
	s.pending = nil
	if s.ch == nil {
		return got
	}
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
