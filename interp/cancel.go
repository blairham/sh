// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"sync/atomic"
	"syscall"
)

// Canceling a run from outside it.
//
// [Runner.Run] has always taken a [context.Context] and, until this file, no
// loop ever asked it anything: one `ctx.Err()` in the package, in `read -t`'s
// own timer. A caller with a runaway script — an embedder running untrusted
// input, a test bounding a case, a front end that wants ^C to work through the
// library rather than through the terminal — had no way to stop it. That was
// #1075, and it is why deadline_test.go's bound can *name* a hang and not end
// one.
//
// # The decision, and what it costs
//
// The context is honored. A parameter in a public signature that does nothing
// is a promise the package does not keep, and the cost of keeping it turns out
// to be a nil comparison.
//
// Not `ctx.Err()` per round, which is what makes this cheap enough to have.
// `Err` on a cancelable context takes a lock, and `Done` on one allocates the
// channel on first call; both are real work to repeat inside a loop of
// builtins. The channel is read **once**, in RunPart, and what each command
// does is compare a field against nil and — only when a caller actually
// supplied a cancelable context — take the default arm of a select. Measured
// with `for ((i=0;i<200000;i++)); do :; done`: see TestBenchmarks in
// cancel_test.go for the figures this was decided on.
//
// One door rather than four. `command` is where an interrupt is already
// noticed, for the reason its comment gives — for a loop of the shell's own
// commands there is no other place, because nothing in `while :; do echo tick;
// done` blocks, waits or returns anywhere else. A caller canceling is the
// library's ^C and is noticed at the same door, so `for ((;;)); do :; done`,
// `while :; do :; done` and a runaway list are all covered by one check
// instead of by three loop drivers and a list walker that would each have to
// stay in step.
//
// # What a canceled run reports
//
// A library question rather than a shell one: no shell in the panel has a
// caller to be canceled by, so there is nothing to measure and nothing for a
// dialect to answer. Two things are decided here and both are conservative.
//
// It unwinds as **controlExit carrying abandonRequested** — a request to stop
// rather than an error. That is what keeps a canceled run from being caught:
// GiveUpTheFile and GiveUpTheLine deliberately let a request to stop through,
// so a `.`, an `eval`, a startup file or an interactive prompt cannot swallow
// a cancellation and carry on. A caller that asked for the run to end gets the
// run ended.
//
// The status is the one a shell reports when it is stopped from outside, which
// is 128 plus SIGINT under every dialect that answers the signal question.
// `diedOfSig` is deliberately **not** set: nothing died of a signal, and a
// pipeline reading that field would otherwise report a death that did not
// happen.
//
// And [Runner.Run] returns the context's own error, which is the Go
// convention and the only way a caller can tell a run it stopped from a run
// that ended. `errors.Is(err, context.Canceled)` and
// `errors.Is(err, context.DeadlineExceeded)` separate it from the
// "not implemented yet" errors that are the other thing Run's error can be.

// cancelWatch is what a run is watched for, shared by every clone of the
// Runner that started it.
//
// A pointer on the Runner rather than a bool, and the reason is a subshell:
// `( while :; do :; done )` runs on a *copy* of the runner, so a copy is where
// the cancellation is noticed, and a flag copied by value would have stopped
// the loop and left the shell that started it reporting a clean finish. One
// shared record, written by whichever runner saw it first, is what makes the
// answer the run's rather than one copy's.
//
// Allocated only when the caller supplied a cancelable context. A nil field
// is the common case — context.Background() has no channel — and is what keeps
// the per-command check to a comparison against nil.
type cancelWatch struct {
	done <-chan struct{}
	hit  atomic.Bool
}

// watch records what this chunk is watched for.
//
// Called once per chunk, from RunPart, rather than once per command: reading
// ctx.Done() is what allocates a cancelable context's channel, and Err takes
// its lock, so both belong outside the loop.
func (r *Runner) watch(ctx context.Context) {
	if ctx == nil {
		// A nil context reaches here: Runner.Expand runs a command
		// substitution for a caller that asked for one word to be expanded
		// and had no context to give. Nothing was ever asked of the
		// argument before this, so nothing had to notice; a nil channel is
		// the honest reading of "no caller is watching".
		r.cancelWatch = nil
		return
	}
	r.canceledChunk = false
	if done := ctx.Done(); done != nil {
		r.cancelWatch = &cancelWatch{done: done}
		return
	}
	r.cancelWatch = nil
}

// canceled reports whether the caller has asked this run to stop, and stops
// it if so.
func (r *Runner) canceled() bool {
	w := r.cancelWatch
	if w == nil {
		return false
	}
	select {
	case <-w.done:
	default:
		return false
	}
	w.hit.Store(true)
	if r.ctl != controlExit {
		r.status = r.signalDeathStatus(syscall.SIGINT)
		r.ctl, r.abandon = controlExit, abandonRequested
	}
	return true
}

// stoppedByTheCaller reports whether this chunk, or any copy of the runner
// running it, stopped because the context was canceled.
func (r *Runner) stoppedByTheCaller() bool {
	return r.cancelWatch != nil && r.cancelWatch.hit.Load()
}

// releaseCancellation puts the shell back into ordinary flow at the end of the
// chunk a cancellation stopped.
//
// **The unit of a cancellation is the chunk the context was given to**, and
// this is what makes that true. A front end that feeds a Runner one typed line
// at a time hands each line a context of its own, and a line whose context was
// canceled costs that line: the session is still there, its variables and
// functions are still there, and the next line runs. Leaving the shell
// unwinding would have made one canceled input close the session, which is
// the shape driver's own test pins.
//
// That is not the boundary GiveUpTheLine is, and the difference is the whole
// reason the unwinding carries abandonRequested. *Inside* the chunk nothing
// may catch a cancellation — a `.`, an `eval` or a prompt that gave up one
// file and carried on would be running commands the caller has asked it to
// stop running. It is only here, where the chunk is over and the caller has
// its error back, that the shell is usable again.
//
// The watch is dropped with it, so the next chunk is watched by whatever
// context that chunk is given rather than by this one's ghost.
func (r *Runner) releaseCancellation() {
	if !r.stoppedByTheCaller() {
		return
	}
	if r.ctl == controlExit && r.abandon == abandonRequested {
		r.ctl = controlNone
	}
	r.cancelWatch = nil
	r.canceledChunk = true
}
