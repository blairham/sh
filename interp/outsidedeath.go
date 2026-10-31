// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
)

// A fatal signal from another process, handed to the shell to die of once its
// EXIT trap has run.
//
// `trap cleanup EXIT` is how a script removes its temporary files when it is
// killed, and a shell that dies of a signal from outside where it lands never
// gets to run it. Measured 2026-10-29 with `/bin/kill -SIG` from another
// process, each shell in a process group of its own and parked in an external
// `sleep`, a builtin `read` on a pipe nobody writes, a busy loop, a function
// waiting on a `sleep`, a `( … )` and a pipeline, under
// `trap 'echo "bye $?"' EXIT`:
//
//	                      bash 5.3, bash 3.2, ksh93   zsh, dash
//	TERM USR1 USR2 ALRM   bye, then killed by it      killed, nothing written
//	VTALRM XCPU ABRT …    bye, then killed by it      killed, nothing written
//	HUP                   bye, then killed by it      zsh: bye 1, exit 1
//
// which is ExitTrapRunsOnSignalDeath reached from outside rather than a new
// question: the same three run the trap for a signal the script sends itself,
// and the same two do not. Every one of them ran it at once, while the `sleep`
// was still running and the `read` still waiting, and left the `sleep` behind
// when it died. This shell ran it nowhere (#7054).
//
// `$?` in the trap is what the shell held when the signal landed: `(exit 7);
// sleep 3` shows 7 in all three, in a function as at the top — ksh93 in a
// function apart, which shows 256 plus the number: see
// Semantics.DeathInAFunctionEndsTheCallAsKilled — and a `read` the signal
// gave up shows the status from before the `read`.
//
// # The route
//
// The front end's watch hears the signal on a goroutine of its own, and the
// EXIT trap is the script's, so the watch cannot run it. It hands the death
// over instead, through the closure TakesAFatalSignal returns, and the shell
// takes it at its next command as it takes one a subshell aimed at `$$` —
// signalDeath, then Finish, whose EXIT trap asks ExitTrapRunsOnSignalDeath and
// whose Runner.DieBySignal ends the process by the same signal. So the two
// roads end in the same place, as the watch's own comment says the throwing
// class's did.
//
// What makes "at its next command" soon is that the waits a shell spends its
// time in are given up for it: a program it waits for in the foreground (the
// program is left running, as every reference leaves it), a `read` of a pipe,
// and `wait`. A copy of the shell that the shell is waiting for — a `( … )`, a
// pipeline element, a substitution's body — stops at its own next command, so
// the shell above it gets its turn. A background job is not one of those and
// carries on, as a reference's child would.
//
// Only where something would run. Without an EXIT trap, or in a dialect that
// runs none on a death, the watch ends the process where the signal lands, as
// it did before; nothing here slows that down. A HUP in the dialect that ends
// on one as an exit is the exception that has two reasons to be handed over:
// the trap, or the dialect's exit hook, which that ending runs too (#7066).
// And never at a prompt, where the shell is waiting on a person rather than on
// a command.
//
// A HUP taken inside a function call in that dialect is a stop that ends the
// shell where it stands, so the call loses the EXIT trap it was holding for its
// caller: the call's own trap runs and the top level's does not, as for one
// the shell sent itself. See Semantics.StopInsideACallLosesTheHeldExitTrap.
//
// A wait nobody gives up — a write into a full pipe, say — is the watch's to
// answer: a death the shell has not taken within its grace is taken back, and
// the process ends without the trap, as it did before.

// The states of a death handed over: nothing yet, handed over and waiting to
// be taken, taken by the shell, and taken back by the front end.
const (
	outsideNone int32 = iota
	outsidePosted
	outsideClaimed
	outsideRevoked
)

// What the shell at the top has told the front end's watch it would run on
// its way out of a fatal signal from outside: nothing, the dialect's exit hook
// alone — which runs only on a HUP the dialect ends on as an exit, since a
// shell killed by a signal runs none (Runner.fireExitHook) — or an EXIT trap.
const (
	armedForNothing int32 = iota
	armedForAHangup
	armedForADeath
)

// handover is one fatal signal from outside handed to the shell: the death
// itself, or the signal that ends the EXIT trap it runs.
type handover struct {
	// state is one of the outside* constants. It moves forward only.
	state atomic.Int32
	// name and sig are the signal, written once before state reads posted
	// and read only after.
	name string
	sig  syscall.Signal
	// dying is closed when the signal is handed over, and is what a wait
	// that gives itself up for one selects on.
	dying chan struct{}
	// claimed is closed when the shell takes it.
	claimed chan struct{}
	// abandoned says a wait was given up for it, so the command that was
	// running when it landed never finished. See Runner.takeOutsideDeath.
	abandoned atomic.Bool
}

// post hands sig over, and reports what TakesAFatalSignal's closure reports.
// Called with the signal state's lock held.
func (h *handover) post(name string, sig syscall.Signal) (<-chan struct{}, func() bool, bool) {
	h.name, h.sig = name, sig
	h.state.Store(outsidePosted)
	close(h.dying)
	return h.claimed, func() bool { return h.state.CompareAndSwap(outsidePosted, outsideRevoked) }, true
}

// outsideDeath is one process's record of a fatal signal handed over from
// outside, part of signalState and shared by every copy of the shell.
type outsideDeath struct {
	// The death.
	handover
	// trapEnd is a fatal signal that lands while the shell at the top runs
	// its EXIT trap, in the dialect where that ends the trap and not the
	// shell, and trapRunning says the trap is running there. See
	// Runner.publishTheTrapRunning.
	trapEnd     handover
	trapRunning atomic.Bool
	// armed is one of the armedFor* constants: what the shell at the top
	// has to run on its way out, where a front end has asked to hand it a
	// death. See Runner.publishExitTrap.
	armed atomic.Int32
	// heard says a front end took TakesAFatalSignal, which is the only way a
	// death is ever handed over: a Runner nobody listens for is never armed,
	// and its waits keep the plainer forms.
	heard atomic.Bool
}

// errAbandonedForADeath is a wait given up because the shell has been handed
// a fatal signal to die of. Nothing reports it; the caller leaves the command
// as it was and the shell takes the death at its next command.
var errAbandonedForADeath = errors.New("abandoned for a fatal signal from outside")

// TakesAFatalSignal hands a front end that owns the process the way to give
// this shell an untrapped fatal signal from another process to die of, rather
// than ending the process where the signal lands.
//
// The closure reports taken where the shell has something to run first — an
// EXIT trap, in a dialect that runs one on a death or, for HUP, in the one
// that ends on it as an exit, where the dialect's exit hook is reason enough
// too (#7066) — and has been handed the signal. claimed is
// closed once the shell has taken it, after which the shell runs its EXIT
// trap and ends the process itself through Runner.DieBySignal. revoke takes
// back a death the shell has not taken yet and reports whether it did, for a
// front end that will not wait any longer: the shell then never takes it.
//
// Not taken where the shell would run nothing, is at a prompt, or has already
// been handed a death — a second signal while the first one's EXIT trap runs
// is the front end's to end the process with, as it ends a reference: measured
// 2026-10-29, a second TERM, USR1 or HUP during a `sleep` in the trap ends
// bash 5.3, bash 3.2 and ksh93 at once. Except in the dialect where any fatal
// signal while the EXIT trap runs ends only the trap, which ksh93's does at
// once and then leaves by the first: there it is taken, as the signal that
// ends the trap. See Semantics.FatalSignalInTheExitTrapEndsOnlyTheTrap.
//
// Shaped like TrapsSignal and for its reason: asked from the front end's own
// goroutine, it closes over the shared signal state and over the answers as
// they stood when it was taken, never over the Runner's fields. Take it again
// once Interactive is set. The axes are read and not asked, as
// IgnoresUntrapped's are: an unanswered one is no, which leaves the signal to
// end the process as it did before.
func (r *Runner) TakesAFatalSignal() func(syscall.Signal) (claimed <-chan struct{}, revoke func() bool, taken bool) {
	s, interactive := r.sigs(), r.Interactive
	s.outside.heard.Store(true)
	runsOnADeath := r.sem().ExitTrapRunsOnSignalDeath == Yes
	hangupExits := r.sem().HangupIsAnOrderlyExit == Yes
	endsTheTrap := r.sem().FatalSignalInTheExitTrapEndsOnlyTheTrap == Yes
	segvKills := r.sem().UntrappedSegvEndsTheShellWhereItLands == Yes
	return func(sig syscall.Signal) (<-chan struct{}, func() bool, bool) {
		o := &s.outside
		if interactive {
			return nil, nil, false
		}
		// A signal with no name runs no EXIT trap in any column, which is
		// core and not the axis: see Runner.runExitTrap. signalName spells
		// one of those as its number, which is not a name the table holds.
		name, ok := signalName(sig)
		if _, named := trappableSignals[name]; !ok || !named {
			return nil, nil, false
		}
		// Nor an untrapped SEGV, in the dialect that lets one kill the
		// shell where it lands: neither the EXIT trap nor the end of a
		// running one is the shell's to run first. See
		// Semantics.UntrappedSegvEndsTheShellWhereItLands.
		if name == segvName && segvKills {
			return nil, nil, false
		}
		if endsTheTrap && o.trapRunning.Load() {
			return o.endTheTrap(&s.mu, name, sig)
		}
		hangup := name == "HUP" && hangupExits
		switch o.armed.Load() {
		case armedForNothing:
			return nil, nil, false
		case armedForAHangup:
			// The exit hook alone, which a death would not run.
			if !hangup {
				return nil, nil, false
			}
		default:
			if !runsOnADeath && !hangup {
				return nil, nil, false
			}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if o.state.Load() != outsideNone {
			return nil, nil, false
		}
		return o.post(name, sig)
	}
}

// endTheTrap hands over a fatal signal that landed while the shell's EXIT
// trap runs, in the dialect where it ends the trap rather than the shell. See
// Semantics.FatalSignalInTheExitTrapEndsOnlyTheTrap.
//
// One ends it, and a later one while it is still being ended is taken as the
// same: the shell is leaving as it was already leaving, and the watch waits
// for that rather than ending the process by the later signal. One that came
// after the watch took the first back is the watch's, as before.
func (o *outsideDeath) endTheTrap(
	mu *sync.Mutex, name string, sig syscall.Signal,
) (<-chan struct{}, func() bool, bool) {
	mu.Lock()
	defer mu.Unlock()
	e := &o.trapEnd
	switch e.state.Load() {
	case outsideNone:
		return e.post(name, sig)
	case outsideRevoked:
		return nil, nil, false
	}
	return e.claimed, func() bool { return e.state.CompareAndSwap(outsidePosted, outsideRevoked) }, true
}

// publishExitTrap tells the front end's watch what the shell at the top would
// run on its way out — an EXIT trap, or the dialect's exit hook where a HUP
// ends the shell as an exit — which is the one thing a death from outside is
// handed over for, and has the front end listen for the signals it would
// otherwise leave to end the process where they land.
//
// The hook as well as the trap, because the dialect that ends on a HUP as an
// exit runs its hook for one from outside as for one it sent itself: measured
// 2026-10-30 on zsh 5.9.2, `/bin/kill -HUP` from another process at
// `zshexit() { echo hook; }; /bin/sleep 3` writes `hook` and exits 1, and so
// does the same with the function named in `zshexit_functions` instead. This
// shell armed for the trap alone, so the watch ended the process with nothing
// run (#7066).
//
// Asked between statements, which is where a `trap` has taken effect, and
// written only when the answer moves. The shell at the top alone: a copy's
// EXIT trap is its own and a reference's child would not have heard the
// signal at all. Never at a prompt, and never where no front end listens,
// since no death would be handed over there for the waits to be given up for.
func (r *Runner) publishExitTrap() {
	s := r.signals
	if s == nil {
		return
	}
	armed := armedForNothing
	// And only until one has been handed over. Taken, it is what the shell is
	// leaving by, and the EXIT trap and the hook it runs on the way out wait
	// as any command does: armed, a wait of theirs would select on a death
	// that has already landed and be given up at once — `/bin/sleep 2` in
	// `zshexit` returned at once where zsh 5.9.2 waits it out.
	if !r.Interactive && s.outside.heard.Load() && s.outside.state.Load() == outsideNone {
		switch {
		case r.exitTrap != nil:
			armed = armedForADeath
		case r.exitHookStandsForAHangup():
			armed = armedForAHangup
		}
	}
	if armed == r.exitTrapPublished {
		return
	}
	r.exitTrapPublished = armed
	s.outside.armed.Store(armed)
	if armed == armedForADeath && r.ListenForFatalSignals != nil && r.sem().ExitTrapRunsOnSignalDeath == Yes {
		r.ListenForFatalSignals()
	}
}

// exitHookStandsForAHangup reports whether the dialect's exit hook has a
// function to run — its own name, or one its list names — in the dialect that
// ends on a HUP as an exit, which is the only ending from outside a hook runs
// for. A name with nothing defined behind it runs nothing, as Runner.FireChain
// passes it over.
//
// Asked between every two statements where there is no EXIT trap, so it
// builds nothing: the list's name is kept once it has been spelled.
func (r *Runner) exitHookStandsForAHangup() bool {
	sem := r.sem()
	name := sem.ExitHook
	if name == "" || sem.HangupIsAnOrderlyExit != Yes {
		return false
	}
	if _, ok := r.funcs[name]; ok {
		return true
	}
	if sem.HookListSuffix == "" || r.arraysLen() == 0 {
		return false
	}
	list := r.exitHookList
	if len(list) != len(name)+len(sem.HookListSuffix) ||
		!strings.HasPrefix(list, name) || !strings.HasSuffix(list, sem.HookListSuffix) {
		list = name + sem.HookListSuffix
		r.exitHookList = list
	}
	a, ok := r.arraysLookup(list)
	if !ok {
		return false
	}
	for _, fn := range r.readArray(a) {
		if _, ok := r.funcs[fn]; ok {
			return true
		}
	}
	return false
}

// takeOutsideDeath is the shell acting on a fatal signal from outside that
// has been handed to it, and reports whether this runner has stopped for one.
//
// The shell at the top takes it and dies of it as of one it sent itself —
// signalDeath, then the EXIT trap and the raise in Finish. A copy the shell is
// waiting for stops where it stands, so the shell above it gets its turn; it
// runs nothing of an EXIT trap of its own, because in a reference it is
// another process the signal never reached — measured, bash 5.3 prints the
// shell's `bye` alone for `( trap 'echo in' EXIT; sleep 3 )`. A background
// job carries on.
//
// The status the EXIT trap is shown is the one the shell held when the
// signal landed: what the interrupted command began with where a wait was
// given up for it, since that command never finished, and otherwise the last
// command's. Read before the command door records the next command's.
func (r *Runner) takeOutsideDeath() bool {
	// One load at every command, and a call only once there is something to
	// act on, so the door stays the price it was.
	s := r.signals
	if s == nil {
		return false
	}
	if s.outside.state.Load() != outsidePosted &&
		(!r.inExitTrap || s.outside.trapEnd.state.Load() != outsidePosted) {
		return false
	}
	return r.actOnOutsideDeath()
}

// actOnOutsideDeath is takeOutsideDeath once a death is waiting, or a signal
// that ends the EXIT trap.
func (r *Runner) actOnOutsideDeath() bool {
	s := r.signals
	if r.inSubshell {
		if r.bg != nil {
			return false
		}
		// Stopped as `exit` stops it, and its EXIT trap — and zsh's exit
		// hook — start and stop again at their first command, which comes
		// to this same door while the death is still the shell's to take.
		r.ctl, r.abandon, r.errexitStopped = controlExit, abandonRequested, false
		return true
	}
	o := &s.outside
	if e := &o.trapEnd; r.inExitTrap && e.state.Load() == outsidePosted {
		if !e.state.CompareAndSwap(outsidePosted, outsideClaimed) {
			return false
		}
		close(e.claimed)
		// Which ends the trap, as the same signal sent by the trap itself
		// would. See Runner.fatalSignalEndsOnlyTheExitTrap.
		r.signalDeath(e.name, e.sig)
		return true
	}
	if !o.state.CompareAndSwap(outsidePosted, outsideClaimed) {
		// Taken back: the front end is ending the process itself.
		return false
	}
	close(o.claimed)
	shown := r.status
	if o.abandoned.Load() {
		shown = r.statusWhenTheCommandBegan
	}
	r.signalDeath(o.name, o.sig)
	r.showTheExitTrap(shown, o.name, o.sig)
	return true
}

// abandonsForADeath is what a wait this runner makes selects on to be given
// up for a fatal signal from outside: closed once one has been handed to the
// shell, and nil where none could be — nothing for the shell at the top to
// run on its way out, or a background job.
//
// That includes the EXIT trap's own waits once the shell has taken a death,
// and the exit hook's: the body's first statement publishes the shell as
// unarmed once a death has been handed over, so nothing either waits for is
// given up. See Runner.publishExitTrap.
//
// Except in the dialect where a fatal signal ends the EXIT trap and not the
// shell: there the trap's waits are given up for that signal instead. See
// Runner.publishTheTrapRunning.
func (r *Runner) abandonsForADeath() <-chan struct{} {
	s := r.signals
	if s == nil || r.bg != nil {
		return nil
	}
	if r.inExitTrap && s.outside.trapRunning.Load() {
		return s.outside.trapEnd.dying
	}
	if s.outside.armed.Load() == armedForNothing {
		return nil
	}
	return s.outside.dying
}

// waitAbandonedForADeath records that a wait was given up for the signal
// handed over, and hands back its signal.
func (r *Runner) waitAbandonedForADeath() syscall.Signal {
	o := &r.signals.outside
	h := &o.handover
	if r.inExitTrap && o.trapRunning.Load() {
		h = &o.trapEnd
	}
	h.abandoned.Store(true)
	return h.sig
}

// outputsDetachedOnADeath puts a program's output streams that os/exec copies
// on a goroutine of its own — anything that is not a file, which is a command
// substitution's buffer above all — behind a writer that can be cut off, and
// hands back what cuts them off, for a wait a death from outside may give up.
//
// The copying goes on for as long as the program writes, and a program whose
// wait was given up is still running: the substitution reading its buffer and
// the goroutine still filling it would be a race on the buffer, which the race
// detector reported on the first run of the substitution row (#7054). Cut off,
// what the program writes afterwards is dropped, as a reference's output is
// that nobody is left to read. Where nothing could be given up the streams are
// left exactly as they were.
func (r *Runner) outputsDetachedOnADeath(cmd *exec.Cmd) (detach func()) {
	if r.abandonsForADeath() == nil {
		return func() {}
	}
	var cut []*detachableWriter
	wrap := func(w io.Writer) io.Writer {
		if _, isFile := w.(*os.File); w == nil || isFile {
			return w
		}
		d := &detachableWriter{w: w}
		cut = append(cut, d)
		return d
	}
	// One writer for both where os/exec was handed one for both, which is
	// what keeps it to one pipe and the two streams in order.
	if sameWriter(cmd.Stdout, cmd.Stderr) {
		cmd.Stdout = wrap(cmd.Stdout)
		cmd.Stderr = cmd.Stdout
	} else {
		cmd.Stdout, cmd.Stderr = wrap(cmd.Stdout), wrap(cmd.Stderr)
	}
	return func() {
		for _, d := range cut {
			d.detach()
		}
	}
}

// sameWriter reports whether two writers are one, as os/exec asks it: where
// the type can be compared at all.
func sameWriter(a, b io.Writer) (same bool) {
	defer func() {
		if recover() != nil {
			same = false
		}
	}()
	return a != nil && a == b
}

// detachableWriter is a writer that can be cut off from the one it writes to.
type detachableWriter struct {
	mu       sync.Mutex
	w        io.Writer
	detached bool
}

func (d *detachableWriter) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.detached {
		return len(p), nil
	}
	return d.w.Write(p)
}

func (d *detachableWriter) detach() {
	d.mu.Lock()
	d.detached = true
	d.mu.Unlock()
}
