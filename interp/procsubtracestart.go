// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "sync"

// When a **reading** process substitution's body runs its first command, which
// is the one thing about it a script can observe: the order its `xtrace` lines
// arrive in.
//
// `: <(print A); sleep 0` writes `@ : /dev/fd/11`, `@ print A`, `@ sleep 0` in
// the reference and wrote the body's line *after* `@ sleep 0` here — the body
// ran in a goroutine nothing had yielded to, so it reached its first command
// only once the parent had gone on (#5088).
//
// **The reference's order is a race**, measured rather than assumed: over five
// runs of each of twenty-five shapes, zsh disagreed with *itself* on four, and
// where it settles the answer is decided by what the next statement costs —
// `sleep 0` forks and execs, so the child wins, where a builtin does not. So
// there is no rule to copy. What this engine can do is be deterministic and
// land on the order the reference is observed to produce on the shapes that
// settle.
//
// **A head start alone cannot do that**, which was the first attempt and is
// worth recording: releasing the body early is a *lower bound* — it has traced
// by a point — and nothing in it stops the body tracing *earlier* than the
// reference does. That version flaked about one run in a few, and the mutant
// that removed half of it survived, which is the same gap seen from both sides.
//
// So each body is **held as well as released**. It waits before its first
// command until the parent says so, and the parent then waits for it to have
// traced. Two release points, which is what the measured shapes ask for:
//
//   - **forking the next body releases the previous one**, so every body but
//     the last has traced before the command's own line;
//   - **the command's own trace line releases the last one**, so that one
//     traces after it and before the next statement.
//
// which reproduces one body, two, three and four exactly:
//
//	: <(A)              	`: FD`, A
//	: <(A) <(B)         	A, `: FD FD`, B
//	: <(A) <(B) <(C)    	A, B, `: FD FD FD`, C
//	: <(A) <(B) <(C) <(D)	A, B, C, `: FD … FD`, D
//
// **The release point is before the redirections are applied**, which is what
// makes the holding safe. The earlier reading of this said no such point
// existed and that the only one available — removeProcSubs — is *after* the
// command, where `cat <(print A)` would have the command reading a pipe the
// held body has not written. That is true of removeProcSubs and not of the
// command's own trace line, which is written several statements before
// anything opens or reads anything.
//
// **Inert unless the shell is tracing**, and only for a body whose lines can be
// seen: a `>(…)` body and a redirection target's body are never held, the first
// because it reads what the command is about to write and the second because
// its trace goes nowhere at all (#5089). Nothing is created when `xtrace` is
// off, so an untraced script takes no wait and no hold.
type bodyTraceStart struct {
	release   chan struct{}
	releasing sync.Once
	began     chan struct{}
	beginning sync.Once
}

// newBodyTraceStart is the pair of signals for one body, or nil where nothing
// can observe its order — which is every run with tracing off, every writing
// spelling, and every body whose trace is discarded.
func (r *Runner) newBodyTraceStart(reading bool) *bodyTraceStart {
	if !reading || !r.tracing() || r.bodyTraceIsDiscarded() {
		return nil
	}
	return &bodyTraceStart{release: make(chan struct{}), began: make(chan struct{})}
}

// wait is what the body does before its first command: nothing until the parent
// has reached the point that lets it go.
func (b *bodyTraceStart) wait() {
	if b != nil {
		<-b.release
	}
}

// let releases the body. Idempotent, because the two release points can both
// reach one body — a command with a single substitution has no next fork, and
// one that fails before its trace line is let go by the backstop instead.
func (b *bodyTraceStart) let() {
	if b != nil {
		b.releasing.Do(func() { close(b.release) })
	}
}

// mark records that the body has reached a command, and is also what its *end*
// reports: an empty body traces nothing and must not be waited for.
func (b *bodyTraceStart) mark() {
	if b != nil {
		b.beginning.Do(func() { close(b.began) })
	}
}

// await waits for the body to have begun, bounded by that body's own first
// trace line or its end and never by its work.
func (b *bodyTraceStart) await() {
	if b != nil {
		<-b.began
	}
}

// letGoAndAwait is the two halves together, which is what every release point
// does: let the body run, then wait for it to have written its line.
func (b *bodyTraceStart) letGoAndAwait() {
	b.let()
	b.await()
}

// releasePreviousBodyTrace is the first release point: forking the next body
// lets the previous one go. The bodies of one command are in r.procSubs in the
// order they were written, so the one to release is the last of them.
func (r *Runner) releasePreviousBodyTrace() {
	if n := len(r.procSubs); n > 0 {
		r.procSubs[n-1].began.letGoAndAwait()
	}
}

// releaseHeldBodyTraces is the second release point: the command's own trace
// line has been written, so the body forked last may run. Before the
// redirections are applied and so before anything reads the pipe, which is what
// makes holding a body safe at all.
//
// Every pending body rather than the last alone, because a release already
// taken costs nothing and a command that reached here by a road with no fork
// after it would otherwise leave one held.
func (r *Runner) releaseHeldBodyTraces() {
	for i := range r.procSubs {
		r.procSubs[i].began.letGoAndAwait()
	}
}
