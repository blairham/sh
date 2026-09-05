// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The shell's own goroutines, and what a panic on one costs.
//
// A shell runs four things beside itself, and every one of them is a goroutine
// here where a real shell forks: a background job, each half of a pipeline, a
// coprocess, a process substitution. That is the reconstruction of a process
// boundary this package has to make by hand, and it carries a cost a fork does
// not — a `recover` reaches only the goroutine that panicked, so the guard a
// front end puts around a run cannot see any of them. An interpreter bug hit on
// one of these ended the process however carefully the caller had wrapped its
// call, which for an embedder is the host program dying: the outcome the guard
// exists to prevent, arriving on the road it could not watch.
//
// Three things close that, and none of them works alone.
//
// **The recover goes in the front end**, through GuardConcurrent, because a
// library may not decide to swallow its own broken invariant — the same split
// as ReplaceProcess and DieBySignal, and the one internal/panicguard is already
// built on.
//
// **What the goroutine owes the rest of the shell is handed over whatever
// happens**, and that half is here. The shell blocks until a background job has
// said it started, `wait` blocks until it has said it finished, a pipeline
// element's status is read after the wait for it, and the element downstream
// reads until the pipe is closed. A recover on its own would have turned every
// one of those crashes into a *hang* — or into a pipeline reporting a success
// it never had, since the zero value of a status is 0 — and both are worse than
// the crash they replaced. That is the reason this is a seam rather than a
// `defer recover()` written into four places.
//
// **The handing over comes after the guard, and the guard reports through the
// shell's own error stream.** Both are about the diagnostic being readable. The
// stream, because interp serializes writes to a caller's io.Writer with a lock
// only interp holds — a front end reporting through the raw writer would be a
// second writer with a different lock, which excludes nothing — so the stream
// is handed to the guard rather than assumed by it. The order, because whatever
// was waiting on this goroutine carries straight on the moment it is released:
// a `wait` that returned before the report was written would let the next line
// of the script interleave with it, and a caller reading what the shell wrote
// would find the report missing from what it had just been handed.

// spawn runs work on a goroutine of this shell's own, and hands over what the
// rest of the shell is waiting on when it is done.
//
// handOver runs however work ended — returned, or panicked and was caught, or
// panicked with nobody guarding — and it runs *after* the guard, so nothing
// that was waiting is released until the report is written. It is separate from
// work rather than deferred inside it for exactly that reason: a defer would
// run while the panic was still unwinding, which is before the recover.
//
// The goroutine is started here and the hook is called on it, so a front end can
// say what happens to a panic without being able to say where anything runs. A
// hook that ran the work on the calling goroutine would deadlock the shell it
// was installed to protect, because the shell waits here for a background job
// to report its process. Nil is the default and means a plain `go`: the panic
// ends the process, which is what a library owes a caller whose invariant it
// has just found broken.
//
// The four callers are the four boundaries above. The other goroutines this
// package starts run no script — `wait -n` fans a channel in, and `read -t`
// reads the caller's own stream — so there is no interpreter bug for them to
// hit, and a guard on them could only convert a caller's panic into a shell
// waiting forever for an answer that is no longer coming.
func (r *Runner) spawn(work, handOver func()) {
	guard := r.GuardConcurrent
	if guard == nil {
		go func() {
			defer handOver()
			work()
		}()
		return
	}
	// Taken here, on the goroutine that is spawning, because it reads the
	// runner's streams and the new goroutine's runner is a clone whose
	// fields the caller is still writing.
	errs := r.lockedStderr()
	go func() {
		defer handOver()
		guard(work, errs)
	}()
}

// internalErrorStatus is what one of the shell's own goroutines reports when it
// did not finish.
//
// Nonzero, because nothing about that run succeeded, and 2 because that is what
// a front end already exits with when the *shell* failed rather than a command
// it ran — see internal/panicguard, which is where the number is written down
// for the guard on the calling goroutine. Repeated rather than imported: this
// package does not depend on the front end, and it is the front end that has to
// agree with it.
const internalErrorStatus = 2
