// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/blairham/sh/internal/sigbook"
)

// Two signals from another process never reach os/signal on this kernel, and
// the kernel can still be asked about them.
//
// The runtime keeps PROF for its profiler and answers it in its own handler
// before os/signal is consulted, on every platform. And it drops every PIPE
// in its signal trampoline here, the kernel's copy of a broken pipe and one
// sent by `kill` alike, because this kernel does not deliver the first on the
// thread that wrote (golang/go#33384). So an untrapped PROF from outside ended
// nothing and a trapped PROF or PIPE ran no handler, where bash 5.3, bash 3.2,
// ksh93, zsh and dash all die of the first and run the second (#7046, #7056).
//
// A kqueue sees them anyway. EVFILT_SIGNAL records every attempt to deliver a
// signal to the process, after the handler the process installed has had it,
// and touches neither that handler nor the mask. Measured 2026-11-04 on macOS
// arm64: it sees a PROF, a PIPE and an XFSZ sent from another process, and the
// kernel's copy of a write into a broken pipe or past `ulimit -f`, all with the
// runtime's handlers installed and, for PROF, with os/signal subscribed the
// way the descriptor hold in lowfds_unix.go subscribes it — whose channel
// still receives nothing, so the two do not meet.
//
// What it sees, it passes on as os/signal would have: to every channel in
// sigbook that asked for the signal, which is where a top-level `trap` is
// subscribed, and for PROF to the watch as well, which answers an untrapped
// one the way it answers USR1. Not an untrapped PIPE: the kernel's copy of
// the shell's own in-process writes — a pipeline element's, a subshell's, a
// here-document's writer — looks exactly like one sent from outside, and the
// watch dying of it would end the whole shell for a subshell's broken pipe.
// Only a handler for the signal is given PIPE, and the shell's credits for its
// own writes take those copies out of its way (interp's writeSignalAbsorbed).
//
// The profiler's own ticks are PROF too, and are not the shell's to answer: a
// run of `make pgo` profiles shells in-process. While ITIMER_PROF is armed
// nothing that arrives is passed on, nor for a second after the last tick.
//
// Its descriptor is lifted above every number a script can name, as the
// runtime's own are (#799), and placing onto it is refused the way placing
// onto theirs is: see heldByTheRuntime.

// observerDescriptor is the observer's kqueue, or 0 while there is none.
var observerDescriptor atomic.Int32

// heldByTheObserver reports whether fd is the observer's kqueue.
func heldByTheObserver(fd int) bool {
	held := observerDescriptor.Load()
	return held != 0 && int(held) == fd
}

// profilerGrace is how long after a tick of the profiler's a PROF is still
// taken for one: a tick raised just before the profile stopped can be read
// once the timer no longer says so.
const profilerGrace = time.Second

// observeTheUnforwarded starts the observer for the watch whose channel is ch,
// and returns what stops it, or nil where it could not be started — a shell
// then answers these two as it did before there was one.
//
// Called on the watch's goroutine before it settles, so the registration is
// in place before the shell's first command, which is #7060's guarantee for
// every other signal the watch answers.
func observeTheUnforwarded(ch chan os.Signal) (stop func()) {
	kq, err := syscall.Kqueue()
	if err != nil {
		return nil
	}
	lifted, err := fcntl(kq, syscall.F_DUPFD_CLOEXEC, lowDescriptorCeiling+1)
	_ = syscall.Close(kq)
	if err != nil {
		return nil
	}
	changes := []syscall.Kevent_t{
		signalEvent(syscall.SIGPROF),
		signalEvent(syscall.SIGPIPE),
		{Ident: 0, Filter: syscall.EVFILT_USER, Flags: syscall.EV_ADD | syscall.EV_CLEAR},
	}
	if _, err := syscall.Kevent(lifted, changes, nil, nil); err != nil {
		_ = syscall.Close(lifted)
		return nil
	}
	observerDescriptor.Store(int32(lifted))
	stopped := make(chan struct{})
	go observe(lifted, ch, stopped)
	return func() {
		close(stopped)
		wake := []syscall.Kevent_t{{Ident: 0, Filter: syscall.EVFILT_USER, Fflags: syscall.NOTE_TRIGGER}}
		_, _ = syscall.Kevent(lifted, wake, nil, nil)
	}
}

// signalEvent is the registration for sig's deliveries.
func signalEvent(sig syscall.Signal) syscall.Kevent_t {
	var k syscall.Kevent_t
	syscall.SetKevent(&k, int(sig), syscall.EVFILT_SIGNAL, syscall.EV_ADD|syscall.EV_ENABLE)
	return k
}

// observe waits on kq and passes on what it reports, until it is stopped.
//
// The descriptor is closed here, and only once stopped is closed, so the stop
// never writes to a number that has been given to something else.
func observe(kq int, ch chan os.Signal, stopped chan struct{}) {
	defer func() {
		observerDescriptor.CompareAndSwap(int32(kq), 0)
		_ = syscall.Close(kq)
	}()
	events := make([]syscall.Kevent_t, 4)
	var lastTick time.Time
	for {
		n, err := syscall.Kevent(kq, nil, events, nil)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			<-stopped
			return
		}
		for _, e := range events[:n] {
			switch {
			case e.Filter == syscall.EVFILT_USER:
				return
			case syscall.Signal(e.Ident) == syscall.SIGPROF:
				now := time.Now()
				if profilerRunning() {
					lastTick = now
					continue
				}
				if !lastTick.IsZero() && now.Sub(lastTick) < profilerGrace {
					continue
				}
				// Once however many were folded into the event, as os/signal
				// folds them: a handler runs once for a burst.
				sigbook.Deliver(syscall.SIGPROF)
				toTheWatch(ch, syscall.SIGPROF)
			case syscall.Signal(e.Ident) == syscall.SIGPIPE:
				// One for each the kernel counted, because the shell credits
				// its own broken writes one copy at a time and a copy passed
				// on once for two would leave a credit standing to swallow
				// the next PIPE sent from outside.
				for range min(e.Data, maxPassedOn) {
					sigbook.Deliver(syscall.SIGPIPE)
				}
			}
		}
	}
}

// maxPassedOn bounds how many copies of one event are passed on: the channels
// they go to are buffered, and a burst larger than their buffer would be
// dropped by the send anyway.
const maxPassedOn = 32

// toTheWatch hands sig to the watch whose channel is ch, if that watch is
// still up: under the watch's lock, which is what a take-down closes the
// channel under.
func toTheWatch(ch chan os.Signal, sig os.Signal) {
	fatalWatch.mu.Lock()
	defer fatalWatch.mu.Unlock()
	if fatalWatch.ch != ch {
		return
	}
	select {
	case ch <- sig:
	default:
	}
}

// profToItsDefault and profToTheRuntime are the Linux answer to an untrapped
// PROF. Here the observer answers it, whatever the shell has arranged, so
// there is nothing to change.
func profToItsDefault() {}

func profToTheRuntime() {}
