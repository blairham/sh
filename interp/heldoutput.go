// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io"
	"os"
	"sync"
)

// heldOutputLimit bounds what a hold keeps before it writes some of it out
// anyway — the size of a C library's buffer, roughly, rather than a promise
// that a builtin writing a great deal is held whole.
const heldOutputLimit = 64 << 10

// heldOutput is a builtin's standard output, kept until the builtin returns.
// See Semantics.BuiltinOutputHeldUntilItReturns.
//
// Once released it writes straight through, so a descriptor that copied it
// while it was held — `exec 3>&1` inside an `eval` — goes on working rather
// than filling a buffer nobody will ever flush. A mutex because a subshell or
// a background job started inside the builtin shares the writer.
type heldOutput struct {
	mu sync.Mutex
	// to is the stream the hold replaced, which the Runner gets back; out
	// is the same stream under the lock both streams share, which is where
	// what was held is written — a complaint on the other stream from a job
	// the builtin started is then never torn through the middle of it.
	to, out  io.Writer
	buf      []byte
	released bool
	// guard is the lock wrapper out is, where the stream needs one: kept in
	// the hold rather than made beside it, so a hold is one allocation.
	guard lockedWriter
}

func (h *heldOutput) Write(p []byte) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.released {
		return h.out.Write(p)
	}
	h.buf = append(h.buf, p...)
	if len(h.buf) > heldOutputLimit {
		if err := h.flushLocked(); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (h *heldOutput) flushLocked() error {
	if len(h.buf) == 0 {
		return nil
	}
	n, err := h.out.Write(h.buf)
	h.buf = h.buf[:0]
	return failedPartWay(n, err)
}

// flush writes out what is held so far and goes on holding.
func (h *heldOutput) flush() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.flushLocked()
}

// release writes out what is held and stops holding.
func (h *heldOutput) release() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.released = true
	return h.flushLocked()
}

// unheld is the writer a child process is handed in place of a held one: what
// the builtin wrote first goes out first, and the child writes to the
// descriptor itself rather than through a pipe this shell would have to copy.
func unheld(w io.Writer) io.Writer {
	for {
		h, ok := w.(*heldOutput)
		if !ok {
			return w
		}
		_ = h.flush()
		w = h.to
	}
}

// builtinHold is what holdBuiltinOutput started, for endBuiltinHold to end:
// nothing, a hold of this builtin's own, or the hold of a builtin it runs
// inside, which is flushed rather than let go of. A value rather than a
// closure, so starting one costs only the hold itself (#6373).
type builtinHold struct {
	h      *heldOutput
	nested bool
}

// holdBuiltinOutput starts holding the running builtin's standard output where
// the dialect does, and answers what endBuiltinHold ends — or nothing, where
// there is nothing to hold.
//
// Nested, the hold already in place is flushed at the inner builtin's end
// rather than stacked: each builtin's output reaches the
// descriptor when that builtin returns, so `eval 'print a; print -u2 b; print
// c'` is a, b and c in order, as it is in the shell being modeled.
//
// Read rather than asked. The disagreement shows only where one builtin writes
// to both streams, which nothing can know before it runs, and asking on every
// builtin would refuse every builtin in a vector that has not answered — so an
// unanswered vector holds nothing, which is the zero value and four of the six
// columns' reading of a single write.
func (r *Runner) holdBuiltinOutput() builtinHold {
	if r.sem().BuiltinOutputHeldUntilItReturns != Yes || r.Stdout == nil {
		return builtinHold{}
	}
	if h, ok := r.Stdout.(*heldOutput); ok {
		return builtinHold{h: h, nested: true}
	}
	if r.stdoutIsACharDevice() {
		// A terminal, and the shell being modeled writes a line to one as it
		// is finished; `/dev/null` is one too, and holding what is thrown
		// away would change nothing.
		return builtinHold{}
	}
	h := &heldOutput{to: r.Stdout, out: r.Stdout}
	// lockWriter's answer, with the wrapper it would have made kept in the
	// hold: the same stream under the same lock either way.
	if locks := r.streamLocks(); needsLockWrapper(locks, r.Stdout) {
		h.guard = lockedWriter{locks: locks, w: r.Stdout}
		h.out = &h.guard
	}
	r.Stdout = h
	return builtinHold{h: h}
}

// endBuiltinHold ends what holdBuiltinOutput started: a nested hold is
// flushed and goes on holding, and the builtin's own is let go of and the
// stream it replaced put back. A failed write is the builtin's to report.
func (r *Runner) endBuiltinHold(b builtinHold) {
	if b.h == nil {
		return
	}
	if b.nested {
		if err := b.h.flush(); err != nil && r.writeFailed == nil {
			r.writeFailed = err
		}
		return
	}
	if r.Stdout == b.h {
		r.Stdout = b.h.to
	}
	if err := b.h.release(); err != nil && r.writeFailed == nil {
		r.writeFailed = err
	}
}

// stdoutIsACharDevice reports whether standard output is a character device,
// asked of the file once and remembered for as long as it is the same file.
func (r *Runner) stdoutIsACharDevice() bool {
	f, ok := r.Stdout.(*os.File)
	if !ok {
		return false
	}
	if r.charDevFile == f {
		return r.charDev
	}
	fi, err := f.Stat()
	r.charDevFile, r.charDev = f, err == nil && fi.Mode()&os.ModeCharDevice != 0
	return r.charDev
}
