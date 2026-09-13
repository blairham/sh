// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"os"
	"os/signal"
	"strconv"
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
//
// Derived for the same reason the set above is, and it was the same mistake:
// seven entries written out by hand, so `trap 'x' 5` — TRAP, and in the wild
// on this machine, in /usr/bin/bzless — was refused as not a signal at all
// while `trap 'x' TRAP` worked. Every name the host knows now has both
// spellings, and they refuse alike too: 9 is KILL, so it meets the same
// deliberate refusal the name does rather than a different one.
//
// The numbers are the host's. 10 is BUS on a BSD and USR1 on Linux, and a
// script that says 10 means whichever one it is running on.
var signalNumbers = func() map[string]string {
	m := make(map[string]string, len(knownSignals))
	for _, k := range knownSignals {
		// No two entries share a number, which a test insists on: the aliases
		// that would collide — IOT for ABRT, CLD for CHLD, POLL for IO — are
		// not in the list, and adding one would make this map's contents
		// depend on the order it is written in.
		m[strconv.Itoa(int(k.Sig))] = k.Name
	}
	return m
}()

// signalState is the machinery, shared by every runner in one process.
//
// It is behind a pointer for a reason the race detector found: clone copies a
// Runner by value, and a mutex must not be copied. Sharing is also the honest
// model — signal handlers are installed per *process*, and nothing here forks
// for a subshell.
type signalState struct {
	mu    sync.Mutex
	traps map[string]string
	// ch is where the runtime forwards a signal that came from outside this
	// process. Allocated with the state rather than on the first trap, and
	// buffered so a burst is not lost between commands: a builtin blocked in
	// `wait` selects on it, and a field that only appears part-way through
	// the wait is one the waiter is already past reading.
	ch chan os.Signal
	// wake is a nudge for a builtin blocked on something else — one token per
	// arrival, dropped when one is already outstanding, because the waiter
	// re-reads pending rather than counting tokens.
	wake chan struct{}
	// pending is what the shell already knows has arrived, ahead of the
	// runtime telling it. Only `kill` puts anything here — see selfSignaled.
	pending []string
	// pipeAbsorbed counts the broken pipes answered at the write that caused
	// them, so the kernel's own copies of those SIGPIPEs — forwarded
	// whenever the shell has asked to handle one — are dropped rather than
	// counted again. A count and not a flag, because a script may break
	// several pipes before anything looks at the pending list.
	//
	// A credit nobody spends costs at most one later PIPE from outside the
	// process, which is not a thing a shell's own broken pipe ever is.
	pipeAbsorbed int
	// died and diedSig record a fatal signal a *subshell* aimed at the
	// process. The process is the top-level shell, so the death is the
	// parent's to die: the subshell that sent it carries on — measured,
	// `(kill -INT $$; echo s); echo done` prints s and not done in dash and
	// zsh — and the parent stops when it next looks, which is where a real
	// parent blocked in wait would be ended by the kernel.
	died    string
	diedSig syscall.Signal
	// defaultRestored names the signals `trap -` has explicitly handed back
	// to their default action. It is not the complement of traps: a signal
	// nobody has ever mentioned is absent from both, and the difference
	// between the two matters for the one signal a shell may have been born
	// ignoring — see Semantics.QuitResetRestoresTheDefault.
	defaultRestored map[string]bool
	// borrowed records, for each signal whose *process* disposition this
	// shell has changed, whether the process was ignoring it beforehand. It
	// is what restoreDispositions puts back; see trapSignal for why the
	// snapshot is taken at the first change and never overwritten.
	borrowed map[syscall.Signal]bool
}

// sigs returns the shared state, creating it on first use.
func (r *Runner) sigs() *signalState {
	if r.signals == nil {
		r.signals = &signalState{
			traps: map[string]string{},
			ch:    make(chan os.Signal, 32),
			wake:  make(chan struct{}, 1),
		}
	}
	return r.signals
}

// TrapsSignal hands back a question a front end may ask from a goroutine of
// its own: does this shell have something to do with a signal — a `trap` the
// script installed, or an ignore?
//
// There is one caller and it is the reason for the shape. A front end that is
// the process listens for the signals whose default action ends it, because an
// untrapped fatal one has to end the shell rather than print a Go stack —
// that is the same split as DieBySignal, with interp declining to change a
// process-wide disposition and the binary doing it. But os/signal hands an
// arrival to *every* channel registered for it, so when a script has trapped
// the signal both are told, and the front end must not kill a shell whose
// script was about to handle it. This is how it finds out.
//
// A function taken once rather than a method called later, because a signal
// arrives whenever it arrives: a method would be reading a Runner's fields
// from a goroutine that does not own them. What this closes over is the shared
// signal state, which is behind a lock of its own and is the one part of a
// Runner meant to be reached from more than one goroutine.
//
// It answers for the top-level shell, which is where the handlers are. A
// subshell's traps are its own and never reach os/signal at all.
func (r *Runner) TrapsSignal() func(sig syscall.Signal) bool {
	s := r.sigs()
	return func(sig syscall.Signal) bool {
		name, ok := signalNumbers[strconv.Itoa(int(sig))]
		if !ok {
			return false
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		_, has := s.traps[name]
		return has
	}
}

// poke tells a waiter to look again. Never blocks: one outstanding token says
// everything ten would.
func (s *signalState) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// drainForwarded moves what the runtime has handed over onto the pending list,
// with the lock already held.
//
// Appending is what keeps the order the shell recorded itself in front: those
// are already on the list, and a forwarded arrival goes behind them.
func (s *signalState) drainForwarded() {
	for {
		select {
		case sig := <-s.ch:
			name, ok := signalName(sig)
			if !ok {
				continue
			}
			if name == "PIPE" && s.pipeAbsorbed > 0 {
				// The kernel's copy of a broken pipe the shell already
				// answered at the write that caused it. One write is one
				// arrival, and whether the runtime's forwarding goroutine
				// gets to run before the script ends is not something a
				// handler should fire a second time over — measured at one
				// `handled` in some runs and two in others over the same
				// script, which is the shape of a scheduler deciding.
				//
				// It is also how an element's broken pipe is kept out of the
				// shell that started it: the copy is real and the runtime
				// delivers it here whatever descriptor the write was on, and
				// no panel shell runs the outer handler for a signal the
				// outer process never had.
				s.pipeAbsorbed--
				continue
			}
			s.pending = append(s.pending, name)
		default:
			return
		}
	}
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
	if r.traps != nil {
		// A subshell is a pretend process: its trap table is its own, and
		// the real handlers stay the top-level shell's. Nothing here may
		// touch os/signal — a subshell's `trap '' INT` outliving the
		// subshell as a process-wide ignore would be the parent inheriting
		// from the child.
		if body == nil {
			delete(r.traps, name)
		} else {
			r.traps[name] = *body
		}
		// Set on this side of the boundary now, whatever it was before: the
		// dialect that hides an inherited ignore lists one the subshell
		// makes itself.
		delete(r.inheritedIgnored, name)
		return
	}
	s := r.sigs()
	s.mu.Lock()
	defer s.mu.Unlock()
	// Every one of the three branches below changes a disposition that
	// belongs to the *process*, so the shell borrows it rather than owning
	// it. Noted before the change, and only the first time, so what goes
	// back is the state this shell was handed and not the state some earlier
	// line of the same script left. See restoreDispositions.
	s.borrow(sig)
	switch {
	case body == nil:
		delete(s.traps, name)
		if s.defaultRestored == nil {
			s.defaultRestored = map[string]bool{}
		}
		s.defaultRestored[name] = true
		signal.Reset(sig)
		s.dropOwnIgnore(sig)
	case *body == "":
		s.traps[name] = ""
		delete(s.defaultRestored, name)
		signal.Ignore(sig)
	default:
		s.traps[name] = *body
		delete(s.defaultRestored, name)
		signal.Notify(s.ch, sig)
	}
}

// dropOwnIgnore takes back an ignore this shell installed, so that `trap -`
// reaches the children started after it.
//
// Called with the lock held, from the `trap -` branch and only after
// signal.Reset has had its say — which, for an ignore, is nothing at all. The
// asymmetry restoreDispositions documents applies in full here: signal.Reset
// puts back the handler that was in place before the Go runtime's, and after
// signal.Ignore that is the SIG_IGN just installed. So `trap ” INT; trap -
// INT` left the process ignoring SIGINT, and every child started after the
// reset inherited an ignore the script had explicitly dropped, since an ignore
// survives exec and a handled signal does not (#2507).
//
// Unanimous across the panel and so a correction rather than an axis.
// Measured 2026-09-12 with a child that raises the signal at itself after
// `trap ” SIG; trap - SIG`, for INT, TERM, QUIT and USR1: bash 5.3,
// bash-as-`sh`, bash 3.2, zsh and dash all kill the child, and ksh93 kills it
// too and reports the death its own 256-plus-the-number way. The control —
// the same snippet without the reset — leaves the child alive in all six.
//
// # Only an ignore this shell installed
//
// The guard is borrowed, which records what the process was doing before the
// first `trap` of this script touched the signal. A shell *started* with a
// signal ignored is a different question and the panel splits on it: measured
// the same day through `trap ” INT; exec <shell> -c 'trap - INT; …'`, zsh
// drops the inherited ignore and bash 5.3, bash 3.2, ksh93 and dash all keep
// it. That split is already an axis — Semantics.QuitResetRestoresTheDefault,
// which asks the same question of the one signal a shell may be born ignoring
// — and answering it here, in the core, for every signal at once would be
// deciding it rather than asking. Leaving it alone keeps the four-column
// answer this shell already gave, and keeps `nohup` meaning `nohup`.
//
// # Why Notify and not Notify-then-Stop
//
// Notify onto a drain is what clears SIG_IGN, and it has to stay subscribed.
// Measured the same day, per signal, against a child:
//
//	after Ignore, then…     INT    HUP    USR1   TERM   QUIT
//	Reset                   ignored across the board
//	Notify                  default for all five
//	Notify then Stop        ignored ignored default default default
//
// Stop hands the signal back to the handler the runtime saved when Notify
// installed its own — which, at that moment, was the SIG_IGN being undone —
// for exactly the signals the runtime lets an inherited ignore stand for. The
// trap is that signal.Ignored answers **false** afterwards while the process
// is still ignoring the signal, so a probe that asks Go rather than a child
// reports the fix working on the two signals it does not work on.
//
// What the process keeps is therefore a *handled* signal rather than a
// defaulted one — the same trade restoreDispositions makes and for the same
// reason, since Go offers no way back to SIG_DFL once SIG_IGN is installed.
// Children see the default, which is the graded behavior, and the process
// drops an arrival it would otherwise have been ignoring anyway.
func (s *signalState) dropOwnIgnore(sig syscall.Signal) {
	if s.borrowed[sig] {
		return
	}
	if !signal.Ignored(sig) {
		return
	}
	signal.Notify(dispositionSink, sig)
}

// borrow records the disposition a signal had before this shell changed it.
//
// Called with the lock held, once per signal: a script that ignores SIGINT,
// traps it and resets it again has borrowed one thing, and what it owes back
// is what the process had before the first of those.
func (s *signalState) borrow(sig syscall.Signal) {
	if s.borrowed == nil {
		s.borrowed = map[syscall.Signal]bool{}
	}
	if _, ok := s.borrowed[sig]; !ok {
		s.borrowed[sig] = signal.Ignored(sig)
	}
}

// defaultRestored reports whether `trap -` has named this signal, so that a
// shell which would otherwise be ignoring it has been asked to stop.
//
// Read from the shared state rather than from a subshell's own table, which
// is the same boundary sendSignal draws when it asks whether a signal is
// trapped: the disposition belongs to the process, and a subshell here is a
// pretend child of it.
func (r *Runner) defaultRestored(name string) bool {
	s := r.sigs()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.defaultRestored[name]
}

// signalDisposition is what the shell has arranged for a signal, and it is
// three answers rather than two: nothing at all, an ignore, or a handler.
type signalDisposition int

const (
	// signalFatal is the default action, which for the signals that carry
	// one is to end the process.
	signalFatal signalDisposition = iota
	// signalHandledBy is a trap with a body to run.
	signalHandledBy
	// signalIgnored is `trap '' NAME` — an entry with an empty body, which
	// is a different thing from no entry.
	signalIgnored
)

// signalArranged reports what this runner has arranged for the named signal.
//
// It is asked by the code that decides whether a failure the kernel reported
// *was* a signal, which is a question only the disposition can answer. A write
// into a pipe nobody reads raises SIGPIPE and kills the writer — but only
// while SIGPIPE would kill. Ignore it or handle it and the kernel has nothing
// fatal to raise, so it returns EPIPE to a writer that is still there. The
// errno is identical in all three cases and the outcome is not.
//
// The table is this runner's own inside a subshell and the process's at the
// top, which is the right answer at both. An ignore is the one disposition
// POSIX carries across the boundary intact, so a pipeline element clones one
// that was set outside it — see inheritTraps — while a handled signal is back
// at its default there, which is exactly what the panel does with `trap 'x'
// PIPE; { … } | true`: the writer dies as though nothing had been trapped.
func (r *Runner) signalArranged(name string) signalDisposition {
	table := r.traps
	if table == nil {
		s := r.sigs()
		s.mu.Lock()
		defer s.mu.Unlock()
		table = s.traps
	}
	body, ok := table[name]
	switch {
	case !ok:
		return signalFatal
	case body == "":
		return signalIgnored
	default:
		return signalHandledBy
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
	// A background job sending this is the shape the issue was reported as:
	// the arrival has to reach a `wait` that is already blocked, and nothing
	// else here would tell it.
	s.poke()
}

// brokenPipeAbsorbed answers the SIGPIPE a failed write of this shell's own
// just caused: delivered to a handler between commands where deliver says so,
// and taken out of the runtime's hands either way.
//
// Delivered rather than waited for, which is the argument selfSignaled makes
// at length: the shell holds the answer already — the write it just made is
// where the signal came from — and the runtime's copy arrives on a goroutine
// whose scheduling decides nothing here and would decide whether the handler
// ran at all, or twice.
//
// Absorbed even where nothing is delivered, and that is the half a second
// platform found. The kernel raises SIGPIPE for a write on any descriptor, so
// a *pipeline element* breaking its pipe hands the process a signal the shell
// that started it never had — and with a handler installed the runtime
// forwards it, and the outer handler ran. No panel shell does that.
//
// Delivered to two different lists, and that is the whole of what a subshell
// needed. A handler at the top level belongs to the process, so its arrival
// goes on the shared list with every other one. A handler a subshell set for
// itself belongs to the subshell, and its arrival goes on the subshell's own
// list — where the parent cannot see it, which is the same fact the absorbing
// above states from the other side. Recording it on the shared list is what a
// naive fix does, and it makes the parent run its own PIPE handler for a
// broken pipe an element caused: measured, no panel shell does that either.
func (r *Runner) brokenPipeAbsorbed(deliver bool) {
	if deliver && r.traps != nil {
		// A subshell's own handler, for a signal that never left the
		// subshell. Run between the subshell's commands, and once more as
		// its body ends — see runSelfRaisedTraps.
		r.selfPending = append(r.selfPending, "PIPE")
		deliver = false
	}
	s := r.sigs()
	s.mu.Lock()
	defer s.mu.Unlock()
	if deliver {
		s.pending = append(s.pending, "PIPE")
	}
	s.pipeAbsorbed++
	s.poke()
}

// takeSelfPending reports what this subshell raised on itself and has not
// handled yet, and forgets it. No lock; see the field.
func (r *Runner) takeSelfPending() []string {
	got := r.selfPending
	r.selfPending = nil
	return got
}

// runSelfRaisedTraps answers what a subshell raised on itself, at the point
// its body ends.
//
// A handler runs between commands, and the last command of a subshell has no
// command after it — so the element whose *final* write breaks its own pipe
// would set a handler for exactly that and never run it. Measured on `{ trap
// 'echo child >&2' PIPE; echo "$big"; } | true`, with nothing after the failed
// write inside the element: dash, bash 5.3, ksh93 and zsh all print `child`.
//
// It is the subshell's half of what Finish does for the shell at the top, and
// it is called at the end of a *list* rather than where the subshell's body
// was started, because the two are on opposite sides of the body's
// redirections. Measured with the element's standard error sent to a file:
// dash, bash 5.3, ksh93 and zsh all put the handler's output in that file, so
// the handler runs while the redirection is still in force.
func (r *Runner) runSelfRaisedTraps(ctx context.Context) {
	if !r.inSubshell || len(r.selfPending) == 0 {
		return
	}
	r.runPendingTraps(ctx)
}

// recordSharedDeath notes a fatal signal a subshell sent the process, for
// the top-level runner to die of. First one wins: a process killed twice
// died of the first.
func (r *Runner) recordSharedDeath(name string, sig syscall.Signal) {
	s := r.sigs()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.died == "" {
		s.died, s.diedSig = name, sig
	}
}

// takeSharedDeath reports a death a subshell has already caused, once.
func (r *Runner) takeSharedDeath() (string, syscall.Signal, bool) {
	s := r.signals
	if s == nil {
		return "", 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.died == "" {
		return "", 0, false
	}
	name, sig := s.died, s.diedSig
	s.died, s.diedSig = "", 0
	return name, sig, true
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
	s.drainForwarded()
	got := s.pending
	s.pending = nil
	return got
}

// pendingTrap reports an arrival that has a handler waiting to run, and the
// signal it names.
//
// It looks without taking: running a handler belongs between commands, and a
// builtin that came back *because* of an arrival has not run it. The status it
// reports is the only thing it takes from the arrival.
//
// An ignored signal — a trap with an empty body — is not one of these, which
// is measured: with `trap ” USR1` set, every shell in the panel waits the
// background job out and reports 0.
func (r *Runner) pendingTrap() (syscall.Signal, bool) {
	s := r.signals
	if s == nil {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drainForwarded()
	for _, name := range s.pending {
		if body, ok := s.traps[name]; ok && body != "" {
			return trappableSignals[name], true
		}
	}
	return 0, false
}

// awaitOrTrap blocks until done is closed, or until a trapped signal arrives,
// and reports the signal when one did.
//
// This is what makes `wait` interruptible, and `wait` is the only builtin that
// blocks long enough for the difference to be visible. POSIX has a wait cut
// short by a trapped signal report a status above 128 rather than resume, and
// the panel bears that out: with a background job outliving the signal by
// three seconds, every shell measured came back inside the signal's own 200ms
// rather than at the end of the job.
//
// A subshell is left to block. Its traps are its own table and what is in the
// shared state belongs to the shell at the top, so a subshell returning from
// one would be reacting to a signal it is never going to handle.
//
// giveUp is a second way out, for the caller that has one: `wait` in the
// dialect that does not go on waiting for a job it has been told *stopped*
// (#2227). nil is the common case and needs no guard — a receive on a nil
// channel is never ready, so a caller with no second way out cannot take one.
func (r *Runner) awaitOrTrap(done, giveUp <-chan struct{}) (sig syscall.Signal, trapped, gaveUp bool) {
	s := r.signals
	if s == nil || r.inSubshell {
		select {
		case <-done:
			return 0, false, false
		case <-giveUp:
			return 0, false, true
		}
	}
	for {
		if sig, ok := r.pendingTrap(); ok {
			return sig, true, false
		}
		select {
		case <-done:
			// Asked once more rather than returned on, because a select
			// offered both would choose between them at random. The job in
			// the reported shape sends the signal as its last act and ends a
			// moment later, so both are ready by the time anything looks —
			// and every shell in the panel reports the signal rather than the
			// job, which is only reproducible if the arrival wins outright.
			if sig, ok := r.pendingTrap(); ok {
				return sig, true, false
			}
			return 0, false, false
		case <-giveUp:
			// The same rule in the other direction: a job that stopped and
			// then ended is a job that ended, and the two are ready together
			// often enough that a random choice would be a flaky answer.
			select {
			case <-done:
				return 0, false, false
			default:
			}
			return 0, false, true
		case <-s.wake:
		case sig := <-s.ch:
			s.mu.Lock()
			if name, ok := signalName(sig); ok {
				s.pending = append(s.pending, name)
			}
			s.mu.Unlock()
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
	if r.inSubshell {
		// A subshell is a pretend child process: a signal aimed at `$$` is
		// aimed at the shell at the top, so what has *arrived* is the
		// parent's to handle once the subshell is done — measured, `trap
		// 'echo x' USR1; (kill -USR1 $$; echo sub)` prints sub before x in
		// bash, dash and zsh. Draining the shared list here would run the
		// parent's handler in the child, or worse, lose the arrival to a
		// runner about to be discarded.
		//
		// What the subshell raised on *itself* is not one of those, and it
		// has a list of its own for that reason. A broken pipe an element
		// wrote into is the element's signal: the process never had it, and
		// the handler that answers it is the one the element set. Measured
		// on `{ trap 'echo child >&2' PIPE; echo "$big"; echo reached >&2; }
		// | true`, every shell in the panel runs the element's handler and
		// none of them runs the shell's.
		for _, name := range r.takeSelfPending() {
			if body, ok := r.traps[name]; ok && body != "" {
				r.runTrapHandler(ctx, body)
			}
		}
		return
	}
	if name, sig, died := r.takeSharedDeath(); died {
		// A subshell killed the process, and the process is this shell.
		r.signalDeath(name, sig)
		return
	}
	for _, name := range r.takePending() {
		s := r.sigs()
		s.mu.Lock()
		body, ok := s.traps[name]
		s.mu.Unlock()
		if !ok || body == "" {
			continue
		}
		r.runTrapHandler(ctx, body)
	}
}

// runTrapHandler runs one handler body, with what the interrupted script can
// see put back around it.
func (r *Runner) runTrapHandler(ctx context.Context, body string) {
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

// stopSignals releases the handlers a runner installed.
func (r *Runner) stopSignals() {
	s := r.signals
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// The channel stays where it is: it is read without the lock by a waiter
	// selecting on it, so it is allocated once and never replaced. Stopping
	// it is what releases the handlers, and an unsubscribed channel simply
	// never fires again.
	signal.Stop(s.ch)
}

// stopSignalsAndRestore is the pair of things a shell that has finished owes
// the process: its handler subscriptions released, and every disposition it
// borrowed put back. Written as one call because forgetting the second is
// exactly what #2446 was.
func (r *Runner) stopSignalsAndRestore() {
	r.stopSignals()
	r.restoreDispositions()
}

// dispositionSink receives the signals taken over to undo an ignore the
// script asked for, and nothing ever reads it.
//
// The same device internal/oracle uses for the same reason, and for the same
// measured reason it is a drain rather than a handler: os/signal never blocks
// on delivery, so a full channel means the signal is dropped.
var dispositionSink = make(chan os.Signal, 1)

// restoreDispositions hands back the signal dispositions the shell borrowed
// from the process it is running in.
//
// A `trap` is the one thing a shell does that reaches outside its own tables.
// Everything else a script can change process-wide is already the Runner's
// own — the working directory is Dir and `cd` declines to call os.Chdir, the
// environment is Env and nothing calls os.Setenv, and `umask` and `ulimit`
// reach the process only through the SetUmask and SetRlimit hooks an embedder
// has to supply. Dispositions were the exception: `trap ” INT` called
// signal.Ignore for the whole process and nothing ever put it back.
//
// For a shell that *is* the process that costs nothing, which is why it went
// unnoticed: the process exits a moment later. It costs a great deal for a
// shell that is not. One test binary runs every corpus snippet in its own
// process — interp's printed-source round-trip — so a case whose program
// ignores a signal at the top level left it ignored for every test after it
// and for every child those tests exec'd, since an ignore survives exec and
// that is the whole of what `nohup` does. The failures landed in unrelated
// tests long afterwards and read as a load-dependent flake (#2446).
//
// Fixed here rather than in the test that noticed, because that test is not
// the only in-process embedder and a scrub around one loop leaves the next
// one to find this the same way.
//
// # How a disposition is put back, which is not how it looks
//
// signal.Reset is the obvious undo and it does not undo an ignore. Measured
// 2026-09-12 against a child that raises the signal at itself: after
// signal.Ignore the child survives, and after signal.Reset it still survives
// — Reset restores the handler that was in place before the Go runtime's,
// which is the SIG_IGN just installed. signal.Notify is what clears it, since
// exec resets a *handled* signal to its default in the child. So the two
// directions are asymmetric: an ignore that has to go back is signal.Ignore,
// and an ignore that has to come off is signal.Notify onto a drain.
//
// That is the same mechanism and the same measurement as
// internal/oracle.scrubSignalDispositions, and the ineffective
// `t.Cleanup(signal.Reset(…))` this replaces in interp's own tests had been
// standing in for it.
//
// What the process keeps is a *handled* signal rather than a defaulted one,
// which Go offers no way back from once SIG_IGN has been installed. It is the
// difference that matters: children see the default, and the process drops a
// signal it would otherwise have been ignoring anyway.
//
// # What is not touched
//
// A signal found exactly as it was left alone, so a script that only
// *handled* a signal does not go near os/signal here — signal.Stop in
// stopSignals already released that subscription, and, measured in the same
// run, Notify followed by Stop leaves a child seeing the default. That is
// what keeps this from taking an embedder's own signal.Notify away with it.
func (r *Runner) restoreDispositions() {
	if r.inSubshell {
		// A subshell shares this state with the shell it was cloned from and
		// has changed none of it — trapSignal returns before the os/signal
		// call for a clone — so putting anything back here would be a child
		// tidying up after its parent, mid-script.
		return
	}
	s := r.signals
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for sig, wasIgnored := range s.borrowed {
		if sig == r.killedBySig {
			// The shell is about to die of this one and DieBySignal has not
			// run yet. Putting an inherited ignore back would mean the raise
			// does nothing and a shell that reported a signal death does not
			// take one — so the signal the script asked to be killed by keeps
			// the disposition the script gave it.
			continue
		}
		if signal.Ignored(sig) == wasIgnored {
			continue
		}
		if wasIgnored {
			signal.Ignore(sig)
		} else {
			signal.Notify(dispositionSink, sig)
		}
	}
	clear(s.borrowed)
}
