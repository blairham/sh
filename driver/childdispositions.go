// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"os/signal"
	"syscall"
)

// startWithDispositions is interp.Runner.StartWithDispositions for a shell
// that owns the process: start runs with every signal in ignore ignored and
// every signal in reset at its default, so that what it starts — or the
// program `exec` replaces the shell with — inherits them, and the process's
// own handling goes back afterwards. See interp/childdispositions.go for the
// rule and the measurements (#5414, #7049, #7050, #7051).
//
// An ignore goes on with signal.Ignore, which is the only call that leaves a
// signal ignored across exec, and it takes every subscription the process has
// for the signal with it. What goes back afterwards is this watch's for a
// signal it answers, since the watch dies of an untrapped arrival exactly as
// the runtime's default would have, and the runtime's own handling for the
// rest. interp puts its own subscriptions back beside those once this
// returns.
//
// A reset is a subscription for the length of the start, because a handled
// signal is what exec resets to its default; the ignore goes back on after,
// and an ignored signal has no subscriptions to lose.
func startWithDispositions(ignore, reset []syscall.Signal, start func() error) error {
	awaitTheWatch()
	var ignored []syscall.Signal
	var defaulted []os.Signal
	for _, sig := range ignore {
		if passedToAChild(sig) && !signal.Ignored(sig) {
			ignored = append(ignored, sig)
		}
	}
	for _, sig := range reset {
		if passedToAChild(sig) && signal.Ignored(sig) && !fatalWatch.bornIgnoring[sig] {
			defaulted = append(defaulted, sig)
		}
	}
	for _, sig := range ignored {
		signal.Ignore(sig)
	}
	if len(defaulted) > 0 {
		signal.Notify(make(chan os.Signal, 1), defaulted...)
	}
	err := start()
	if len(defaulted) > 0 {
		signal.Ignore(defaulted...)
	}
	for _, sig := range ignored {
		handBack(sig)
	}
	return err
}

// handBack gives the process back its handling of a signal a window ignored.
//
// The watch for a signal it answers. Anything else goes back to the runtime
// by a subscription taken and dropped again, which leaves the runtime's own
// handler in place — except for HUP and INT, whose ignore at the runtime's
// start is one it leaves standing when the last subscription goes, and
// after a window the runtime cannot tell that ignore from one it was born
// with. Both are signals the watch answers, so that branch is never theirs.
func handBack(sig syscall.Signal) {
	if ch := fatalWatch.ch; ch != nil && watched(sig) {
		signal.Notify(ch, sig)
		return
	}
	c := make(chan os.Signal, 1)
	signal.Notify(c, sig)
	signal.Stop(c)
}

// passedToAChild reports whether a window may put sig on the process at all.
// Each signal it leaves out keeps whatever the process has, and a child
// inherits that, which is what every command inherited before there was a
// window.
//
//   - TSTP, TTIN, TTOU and CONT are the runtime's to leave alone until a
//     program asks for them, and once one has, the runtime keeps a handler
//     of its own that ignores them — so after a window the shell itself
//     would no longer stop at ^Z.
//   - CHLD ignored on the process has the kernel reap a child the moment it
//     ends, and this shell waits for its children by asking for exactly that
//     status. A job that ended inside the window would have none to give.
//   - URG is the runtime's own preemption signal, and PROF its profiler's.
//   - The faults — ILL, TRAP, FPE, BUS, SEGV, SYS and the platform's own —
//     are the runtime's to answer, and os/signal's Ignore changes nothing
//     the kernel sees for them.
//
// KILL and STOP never arrive here: interp asks only for signals a handler
// can be set for.
func passedToAChild(sig syscall.Signal) bool {
	switch sig {
	case syscall.SIGKILL, syscall.SIGSTOP,
		syscall.SIGTSTP, syscall.SIGTTIN, syscall.SIGTTOU, syscall.SIGCONT,
		syscall.SIGCHLD, syscall.SIGURG, syscall.SIGPROF,
		syscall.SIGILL, syscall.SIGTRAP, syscall.SIGFPE, syscall.SIGBUS,
		syscall.SIGSEGV, syscall.SIGSYS:
		return false
	}
	return !platformKeptSignal(sig)
}
