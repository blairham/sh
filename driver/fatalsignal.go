// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/blairham/sh/interp"
)

// A fatal signal that arrives from outside has to end this shell the way it
// ends every other one, and the Go runtime will not do it.
//
// dieBySignal covers the signal a script sends *itself*: it puts the kernel's
// default action back before raising, so `kill -QUIT $$` kills the shell
// silently. A signal from another process takes a different road and never
// reaches that code at all — the runtime's own handler is still installed, and
// what it does depends on a classification we get no say in. Signals it calls
// killing, HUP and INT and TERM, it forwards to the default action, which is
// what a shell wants and is why those already work. Signals it calls throwing
// — QUIT, ABRT, ILL, TRAP, SYS, SEGV, BUS, FPE — it turns into a goroutine
// dump on standard error and an exit status of 2.
//
// Measured against the panel, with the shell in the foreground so nothing has
// inherited an ignore: bash, dash and zsh all die of every one of those eight,
// print nothing at all, and leave 128 plus the number. This shell printed a
// page of Go and exited 2 for all eight. There is no disagreement to model and
// no dialect answer to ask for; it is one behavior, and it is the last road
// for the leak internal/panicguard exists to close.
//
// So the front end listens. os/signal is what turns the runtime's handler into
// a delivery this program can act on, and acting on it is dieBySignal — the
// same restore-and-raise the self-signal path already uses, so the two roads
// end in the same place.
//
// On every platform, which it was not: darwin had a file of its own saying the
// front end must not listen there, because asking os/signal for a signal makes
// that runtime open a *pipe* to carry it, on descriptors low enough for a
// script to name — and `exec 3>f` then wrote over the runtime's own end of it,
// killing the shell where it was about to become the command. That was #799.
// driver/lowfds_unix.go keeps the runtime above every number a script can
// name, so the pipe is out of reach and this file has no exception left to
// make. Five of the eight arrive on darwin rather than all eight, for a
// reason that is the kernel's and not ours; driver/die_test.go's
// reachesTheFrontEnd has it measured.
//
// # The signals the runtime drops
//
// There is a third class, and it was the quietest: signals whose default
// action ends a process and which the runtime, with nothing subscribed,
// catches and drops. USR1, USR2, ALRM, VTALRM and XCPU sent from another
// process ended nothing here. Measured 2026-10-29 with `/bin/kill -SIG` from
// another process at a shell parked in a builtin `read` on a pipe, in an
// external `sleep` and in a busy loop: bash 5.3, bash 3.2, ksh93, zsh and dash
// each die of every one of them, in all three places, and print nothing. This
// shell carried on in all fifteen cells (#7046). The watch takes them on and
// answers them as it answers the throwing class.
//
// Three more of that class are not taken, each for a reason of its own:
//
//   - PIPE and XFSZ are what the kernel raises at a write of the shell's
//     *own* — into a pipe nobody reads, or past `ulimit -f` — and a pipeline
//     element, a subshell and a substitution body are this process's writes
//     here, where in every reference shell they are a child's. The copy the
//     kernel raises for one of those arrives exactly as one sent by `kill`
//     does, so a watch that died of it would end the whole shell for a
//     subshell's broken pipe. A shell's own broken pipe is answered at the
//     write instead (interp's brokenPipeAbsorbed).
//   - PROF never reaches os/signal at all: the runtime keeps it for its
//     profiler and answers it before anything subscribed could be told.
//
// So those three still end nothing when another process sends them.
//
// # A reset puts the watch back
//
// `trap - SIG` is signal.Reset in a shell that owns its process, and Reset
// takes *every* subscription the process has for the signal, the watch's
// among them. For QUIT that let the runtime's throwing default through, and
// `trap 'x' QUIT; trap - QUIT` followed by a QUIT from another process
// printed a goroutine dump where bash 3.2, ksh93, zsh and dash die at 131
// (#7048). And after `trap '' SIG` it was worse, because Reset does not undo
// an ignore: what took the ignore off was a drain that stays subscribed and
// drops what it is handed, so `trap '' TERM; trap - TERM` left a shell no
// TERM from outside could end, where all five die of it (#7047). So the
// front end's half of a reset subscribes the watch again — see watchAgain —
// and the watch decides as it always has, by the shell's table.

// fatalSignals are the ones the Go runtime treats as throwing: it prints a
// goroutine dump and exits rather than letting the default action run.
//
// KILL is not among them and could not be: nothing catches it, and nothing
// needs to, since the kernel never lets the runtime see it. The killing class
// — HUP, INT, TERM and the rest — is not here either, because the runtime
// already forwards those to the default action, and taking them over would
// mean reimplementing what already works.
//
// The kernel's own additions are in platformThrowingSignals.
var fatalSignals = append([]os.Signal{
	syscall.SIGQUIT, syscall.SIGILL, syscall.SIGTRAP, syscall.SIGABRT,
	syscall.SIGBUS, syscall.SIGFPE, syscall.SIGSEGV, syscall.SIGSYS,
}, platformThrowingSignals...)

// discardedSignals are the ones whose default action ends a process and
// which the runtime catches and drops when nothing has subscribed: a shell
// sent one from outside carried on as though nothing had happened.
//
// PIPE, XFSZ and PROF belong to the class and are left out of it on purpose;
// see the file comment above. The kernel's own additions are in
// platformDiscardedSignals.
var discardedSignals = append([]os.Signal{
	syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGALRM, syscall.SIGVTALRM,
	syscall.SIGXCPU,
}, platformDiscardedSignals...)

// killingSignals are the ones the runtime itself ends the process with, which
// is why the watch does not subscribe to them at the start. A reset puts the
// watch on them all the same — see watchAgain — because after an ignore the
// runtime's default is no longer what answers them.
var killingSignals = []syscall.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM}

// fatalWatch is this process's own handling of those signals.
//
// One for the process, and that is the honest model rather than a shortcut: a
// signal disposition belongs to the process, there is one process, and a
// second registration would be a second goroutine racing the first to end it.
// The shell it asks about is a pointer that moves, because a front end builds
// a Runner per invocation and a test binary builds hundreds — what matters is
// that the question reaches whichever shell is running now.
var fatalWatch struct {
	once sync.Once
	// traps answers whether the current shell has a trap for a signal. Nil
	// until a shell that owns the process has been built.
	traps atomic.Pointer[func(syscall.Signal) bool]
	// held answers whether the current shell is holding an interrupt while
	// a foreground program runs. See interp.Runner.InterruptHeld.
	held atomic.Pointer[func() bool]
	// ignores answers whether the current shell, with no trap for a signal,
	// does nothing at all with it. See interp.Runner.IgnoresUntrapped.
	ignores atomic.Pointer[func(syscall.Signal) bool]
	// hangup answers whether the current shell, with no trap for SIGHUP,
	// ends the way `exit` does rather than by the signal, and with which
	// status. See interp.Runner.UntrappedHangupExit.
	hangup atomic.Pointer[func() (int, bool)]
	// bornIgnoring is what the process was ignoring when the watch was set
	// up, for the two signals the runtime lets an inherited ignore stand for.
	// Written once, before the watch starts, and only read after.
	bornIgnoring map[syscall.Signal]bool
	// holdOnce subscribes the watch to SIGINT the first time a shell holds
	// one, so a shell that never does keeps the runtime's default.
	holdOnce sync.Once
	// ch is the watch's subscription, kept so a start that had to ignore
	// the keyboard signals can hand them back to it. See
	// startIgnoringInterrupts.
	ch chan os.Signal
}

// watchFatalSignals arranges for an untrapped fatal signal from outside to end
// this process the way it ends any other shell.
//
// Called only where the front end owns the process — the same condition as
// ReplaceProcess and DieBySignal, and for the same reason. A Runner embedded in
// another program must not take that program's signal handling away from it on
// the say-so of a shell it happens to be running.
func watchFatalSignals(r *interp.Runner) {
	traps := r.TrapsSignal()
	fatalWatch.traps.Store(&traps)
	held := r.InterruptHeld()
	fatalWatch.held.Store(&held)
	hangup := r.UntrappedHangupExit()
	fatalWatch.hangup.Store(&hangup)
	watchWhatTheShellIgnores(r)
	r.HoldInterrupts = holdInterrupts
	fatalWatch.once.Do(func() {
		// Read before anything here subscribes, because a subscription is
		// what would take an inherited ignore away. `nohup` is the shape:
		// a HUP ignored at the start stays ignored, and a reset does not
		// hand it to the watch either. See watchAgain.
		fatalWatch.bornIgnoring = map[syscall.Signal]bool{}
		for _, sig := range killingSignals {
			fatalWatch.bornIgnoring[sig] = signal.Ignored(sig)
		}
		// Buffered, because os/signal drops an arrival rather than blocking
		// and there are a dozen and more of these. Nothing here is slow, but
		// a shell being killed is not the moment to depend on that.
		ch := make(chan os.Signal, len(fatalSignals)+len(discardedSignals)+len(killingSignals))
		fatalWatch.ch = ch
		signal.Notify(ch, fatalSignals...)
		signal.Notify(ch, discardedSignals...)
		if !fatalWatch.bornIgnoring[syscall.SIGHUP] {
			// HUP is the runtime's to end the process with, and it does —
			// by the signal. One shell in the panel ends on it the way
			// `exit 1` ends it instead, from outside as from a `kill`
			// the script sent itself, so the watch has to be the one to
			// answer it. See interp.Semantics.HangupIsAnOrderlyExit.
			signal.Notify(ch, syscall.SIGHUP)
		}
		go func() {
			for s := range ch {
				sig, ok := s.(syscall.Signal)
				if !ok {
					continue
				}
				if traps := fatalWatch.traps.Load(); traps != nil && (*traps)(sig) {
					// The script has claimed it, and interp's own
					// registration has been handed the same arrival. Two
					// channels are told; only one of them may act.
					continue
				}
				if ignores := fatalWatch.ignores.Load(); ignores != nil && (*ignores)(sig) {
					// Ignored by the shell itself, which is not the same as
					// trapped: no handler runs, and nothing dies. See
					// interp.Runner.IgnoresUntrapped.
					continue
				}
				if held := fatalWatch.held.Load(); sig == syscall.SIGINT && held != nil && (*held)() {
					// Held until the program the shell waits for is done,
					// which decides whether the shell dies of it. See
					// interp.Semantics.InterruptWaitsForTheProgram.
					continue
				}
				if hangup := fatalWatch.hangup.Load(); sig == syscall.SIGHUP && hangup != nil {
					if status, exits := (*hangup)(); exits {
						// Not a death in this dialect: measured 2026-10-29,
						// zsh 5.9.2 leaves 1 for a HUP from another process
						// in a `read`, an external `sleep` and a busy loop,
						// where bash 5.3, bash 3.2, ksh93 and dash die of it
						// at 129 (#7046).
						os.Exit(status)
					}
				}
				// Untrapped, so the shell dies of it — silently, with the
				// status a shell killed by a signal leaves, which is what
				// dieBySignal is already for. It does not return.
				_ = dieBySignal(sig)
			}
		}()
	})
}

// watchWhatTheShellIgnores tells the watch what the shell ignores untrapped,
// and subscribes it to SIGTERM where an interactive shell ignores that too.
//
// Called again once the invocation has said whether the shell is
// interactive, because the answer is taken as the shell stood — see
// interp.Runner.IgnoresUntrapped. TERM is the runtime's to end the process
// with until then, which is right for every shell that is not interactive;
// an interactive one ignores it in every column of the panel, measured
// 2026-10-16 with `sh -c 'kill -TERM $PPID'` at a pseudo-terminal (#6718).
//
// Not where the process was born ignoring TERM: subscribing would take that
// ignore away from every child, and the shell ignores it anyway.
func watchWhatTheShellIgnores(r *interp.Runner) {
	ignores := r.IgnoresUntrapped()
	fatalWatch.ignores.Store(&ignores)
	if r.Interactive && fatalWatch.ch != nil && !signal.Ignored(syscall.SIGTERM) {
		signal.Notify(fatalWatch.ch, syscall.SIGTERM)
	}
}

// startIgnoringInterrupts starts a program of an asynchronous list with
// SIGINT and SIGQUIT ignored, so that it inherits them that way, and hands
// the two back afterwards. See interp/asyncinterrupts.go for the rule and the
// measurement (#5414).
//
// signal.Ignore undoes every subscription the process had for the two, and
// signal.Reset cannot undo an ignore — see interp's restoreDispositions — so
// what goes back is a subscription: this watch's, which dies of an untrapped
// arrival exactly as the runtime's default would have. interp puts its own
// trap subscriptions back beside it. A signal the script itself had ignored
// is left ignored.
func startIgnoringInterrupts(start func() error) error {
	keyboard := []os.Signal{syscall.SIGINT, syscall.SIGQUIT}
	var handBack []os.Signal
	for _, sig := range keyboard {
		if !signal.Ignored(sig) {
			handBack = append(handBack, sig)
		}
	}
	signal.Ignore(keyboard...)
	err := start()
	if ch := fatalWatch.ch; ch != nil && len(handBack) > 0 {
		signal.Notify(ch, handBack...)
	}
	return err
}

// holdInterrupts is interp.Runner.HoldInterrupts for a shell that owns the
// process: SIGINT comes to the watch, which asks the shell before dying of
// it, rather than ending the process where the runtime's default would.
func holdInterrupts() {
	fatalWatch.holdOnce.Do(func() {
		if fatalWatch.ch != nil && !signal.Ignored(syscall.SIGINT) {
			signal.Notify(fatalWatch.ch, syscall.SIGINT)
		}
	})
}

// watchAgain puts the watch back on a signal a reset took it off, so that an
// untrapped arrival from outside ends the shell again. See the file comment
// for the two ways a reset left one that did not.
//
// Only a signal the watch answers, and not one the process was born ignoring:
// a subscription takes an ignore away, and the ignore `nohup` hands a shell
// is one every column but zsh keeps through `trap - HUP`. Measured
// 2026-10-29, `trap "" HUP; exec <shell> -c 'trap - HUP; …'` and then a HUP
// from outside: bash 5.3, bash 3.2, ksh93 and dash carry on. zsh's reset is
// the default action, which interp asks about as
// Semantics.QuitResetRestoresTheDefault for the two signals it is measured on.
//
// What the watch then does with an arrival is what it always does: the
// shell's trap table, its untrapped ignores and a held interrupt each come
// before the death, so a subscription here is never itself a death.
func watchAgain(sig syscall.Signal) {
	ch := fatalWatch.ch
	if ch == nil || fatalWatch.bornIgnoring[sig] || !watched(sig) {
		return
	}
	signal.Notify(ch, sig)
}

// watched reports whether the watch answers sig at all.
func watched(sig syscall.Signal) bool {
	for _, list := range [][]os.Signal{fatalSignals, discardedSignals} {
		for _, s := range list {
			if s == sig {
				return true
			}
		}
	}
	for _, s := range killingSignals {
		if s == sig {
			return true
		}
	}
	return false
}
