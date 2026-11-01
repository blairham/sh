// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"io"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

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
//     subshell's broken pipe. A shell's own broken pipe, and its own write
//     past the limit, are answered at the write instead (interp's
//     writeSignalAbsorbed and filesizedeath.go).
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
//
// # The start does not wait for the dropped class; the first command does
//
// os/signal enables a signal nothing has asked for before by a synchronous
// round trip to the runtime's signal-mask thread, a goroutine locked to an OS
// thread of its own, so each one is about four thread handoffs. The throwing
// class is eight or nine of those. The dropped class is five more on darwin
// and thirty-seven on Linux, where the real-time range is most of it, and with
// HUP that put about 1 ms of CPU on every start on Linux and 1.3 to 1.6 ms of
// wall on the ubuntu runner, in every dialect (#7263, after #7060).
//
// So the throwing class is taken where the watch is set up, as before, and the
// watch's own goroutine takes the dropped class and HUP before it starts
// reading, while the start goes on to the prelude and everything else that
// comes before a script. awaitTheWatch is the join, and nothing that runs a
// command gets past it: a signal sent once the shell has done anything another
// process could see finds every subscription in place, exactly as it did when
// the start waited. Until then an arrival of the dropped class is dropped,
// which is what the runtime already did with it for the whole of the start
// before the watch existed — and nothing outside can have seen the shell
// ready, because nothing has run.
//
// Every hook that changes a disposition joins too, because a subscription
// still in flight would take an ignore it raced away.
//
// # A shell with an EXIT trap takes the death over
//
// Ending the process from this goroutine is right for a shell with nothing to
// run on its way out, and wrong for one with an EXIT trap in a dialect that
// runs one on a death: bash 5.3, bash 3.2 and ksh93 run `trap cleanup EXIT`
// for a TERM, a HUP or a USR1 from another process, at once, wherever they are
// standing, and then die of it. This shell never did (#7054). So before it
// dies the watch offers the signal to the shell — interp.Runner's
// TakesAFatalSignal, which takes it only where there is a trap to run — and
// the shell runs the trap and ends the process itself, through the same
// DieBySignal as a signal it sent itself. TERM and INT, which the runtime
// would end the process with before the watch heard of them, come to the
// watch once the shell first has such a trap: see listenForFatalSignals.
//
// A shell that does not take the death within handOverGrace — one standing
// somewhere nothing gives up for it — has it taken back and dies here, as
// before. A second signal while the trap runs ends the process at once, as it
// ends bash — except in the dialect where it ends only the trap, where it is
// offered as that and the shell leaves as it was leaving (#7064). See
// interp.Semantics.FatalSignalInTheExitTrapEndsOnlyTheTrap.

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
	// takes hands the current shell a death it has an EXIT trap to run
	// first. See interp.Runner.TakesAFatalSignal.
	takes atomic.Pointer[takesAFatalSignal]
	// alarm answers whether the current shell, with no trap for SIGALRM,
	// ends on it as a timeout, and says what to write where. See
	// interp.Runner.TimesOutOnAnAlarm.
	alarm atomic.Pointer[alarmTimeout]
	// bornIgnoring is what the process was ignoring when the watch was set
	// up, for the two signals the runtime lets an inherited ignore stand for.
	// Written once, before the watch starts, and only read after.
	bornIgnoring map[syscall.Signal]bool
	// holdOnce subscribes the watch to SIGINT the first time a shell holds
	// one, so a shell that never does keeps the runtime's default.
	holdOnce sync.Once
	// ch is the watch's subscription, kept so a start that had to ignore
	// a signal can hand it back. See startWithDispositions.
	ch chan os.Signal
	// settled is closed once the watch holds every subscription it takes
	// at the start. Nil until the watch is set up. See awaitTheWatch.
	settled chan struct{}
}

// holdTheRestOfTheWatch, when set, runs on the watch's goroutine before it
// takes the dropped class. Nil outside tests: it is how one holds that half
// back for longer than a start takes, to show nothing runs before it lands.
var holdTheRestOfTheWatch func()

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
		settled := make(chan struct{})
		fatalWatch.settled = settled
		hangup := !fatalWatch.bornIgnoring[syscall.SIGHUP]
		go func() {
			// Here rather than before the `go`, so the start does not wait
			// a round trip per signal for them; see the file comment.
			if holdTheRestOfTheWatch != nil {
				holdTheRestOfTheWatch()
			}
			signal.Notify(ch, discardedSignals...)
			if hangup {
				// HUP is the runtime's to end the process with, and it
				// does — by the signal. One shell in the panel ends on it
				// the way `exit 1` ends it instead, from outside as from a
				// `kill` the script sent itself, so the watch has to be
				// the one to answer it. See
				// interp.Semantics.HangupIsAnOrderlyExit.
				signal.Notify(ch, syscall.SIGHUP)
			}
			close(settled)
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
				if takes := fatalWatch.takes.Load(); takes != nil && handedOver(*takes, sig) {
					// The shell has an EXIT trap to run first and has taken
					// the death; it ends the process itself once the trap
					// has run. See interp/outsidedeath.go.
					continue
				}
				if alarm := fatalWatch.alarm.Load(); sig == syscall.SIGALRM && alarm != nil {
					if say, status, timesOut := alarm.answer(); timesOut {
						// Not a death in this dialect's interactive shell:
						// measured 2026-10-30, zsh 5.9.2 at a prompt writes
						// `zsh: timeout` and exits 14 for an ALRM from
						// another process, where bash, dash and ash die of
						// it and ksh93 carries on (#7058).
						_, _ = io.WriteString(alarm.stderr, say)
						os.Exit(status)
					}
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

// awaitTheWatch waits until the watch holds every subscription it takes at the
// start: the join for the half its goroutine takes. Called before the first
// command a shell runs and by every hook that changes a disposition, and
// immediate once the watch has settled or where there is no watch at all.
func awaitTheWatch() {
	if settled := fatalWatch.settled; settled != nil {
		<-settled
	}
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
	// And whether it takes a death over, which is never at a prompt: taken
	// again here for the same reason. See interp.Runner.TakesAFatalSignal.
	takes := takesAFatalSignal(r.TakesAFatalSignal())
	fatalWatch.takes.Store(&takes)
	// And whether an alarm ends it as a timeout, which only an interactive
	// shell does: taken again here for the same reason.
	alarm := alarmTimeout{answer: r.TimesOutOnAnAlarm(), stderr: r.Stderr}
	if alarm.stderr == nil {
		alarm.stderr = io.Discard
	}
	fatalWatch.alarm.Store(&alarm)
	if r.Interactive && fatalWatch.ch != nil && !signal.Ignored(syscall.SIGTERM) {
		signal.Notify(fatalWatch.ch, syscall.SIGTERM)
	}
}

// holdInterrupts is interp.Runner.HoldInterrupts for a shell that owns the
// process: SIGINT comes to the watch, which asks the shell before dying of
// it, rather than ending the process where the runtime's default would.
func holdInterrupts() {
	awaitTheWatch()
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
	awaitTheWatch()
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

// alarmTimeout is interp.Runner.TimesOutOnAnAlarm's answer and the stream the
// shell writes its diagnostics to, where the watch says what the shell would.
//
// Ended by the watch rather than handed over, so it runs no EXIT trap and the
// location it writes is the shell's name alone. That is zsh's answer at a
// prompt, which is where the signal comes from in practice — a TMOUT is a
// prompt's timer — and it is short of zsh's under `-i -c` while a command
// runs, which writes `zsh:1: timeout` and runs the trap.
type alarmTimeout struct {
	answer func() (say string, status int, timesOut bool)
	stderr io.Writer
}

// takesAFatalSignal is interp.Runner.TakesAFatalSignal's answer.
type takesAFatalSignal func(sig syscall.Signal) (claimed <-chan struct{}, revoke func() bool, taken bool)

// handOverGrace is how long the watch waits for a shell it has handed a death
// to take it, before taking it back and ending the process without the EXIT
// trap, as it did before the shell could take one at all.
//
// The shell takes it at its next command, and the waits it spends its time in
// are given up for it, so a taken death is taken in microseconds; the grace
// is for a wait nothing gives up — a write into a pipe nobody reads, say —
// where the alternative is a shell that the signal does not end at all. It is
// generous against a loaded machine for that reason: running out early costs
// the script its cleanup, and running out late costs a shell nothing could
// hand the death to this long, once.
const handOverGrace = 2 * time.Second

// handedOver offers sig to the shell as a death to take, and reports whether
// it took it: false where it had nothing to run first, and false where it did
// not take it within handOverGrace, which the watch then answers as it
// answers any other untrapped fatal signal.
func handedOver(takes takesAFatalSignal, sig syscall.Signal) bool {
	claimed, revoke, taken := takes(sig)
	if !taken {
		return false
	}
	grace := time.NewTimer(handOverGrace)
	defer grace.Stop()
	select {
	case <-claimed:
		return true
	case <-grace.C:
		// Taken back unless the shell took it in the meantime, in which case
		// the shell is the one ending the process.
		return !revoke()
	}
}

// listenForFatalSignals is interp.Runner.ListenForFatalSignals for a shell that
// owns the process: TERM and INT come to the watch, which offers them to the
// shell before dying of them, rather than ending the process where the
// runtime's default would.
//
// Not a signal the process was born ignoring, nor one the shell ignores now:
// a subscription takes an ignore away, from every child as well. A reset
// subscribes the watch again — see watchAgain.
func listenForFatalSignals() {
	awaitTheWatch()
	ch := fatalWatch.ch
	if ch == nil {
		return
	}
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGINT} {
		if !fatalWatch.bornIgnoring[sig] && !signal.Ignored(sig) {
			signal.Notify(ch, sig)
		}
	}
}
