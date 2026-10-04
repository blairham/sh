// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"runtime"
	"syscall"
	"unsafe"
)

// WithSignalsHeld makes do's blocking open uninterruptible, so that a FIFO's
// rendezvous with its peer cannot be lost to a signal.
//
// Retrying an interrupted open is not enough on this kernel, which is what
// eintr.go assumed. A FIFO open that is waiting for its peer counts itself as
// a reader (or a writer) while it sleeps, so the peer's own open sees it and
// does not wait. A signal that lands on the sleeping thread wakes it to be
// interrupted — and if, before that thread runs again, the peer has opened,
// written and closed, the interrupted open takes its count back and the retry
// (the kernel's own restart under SA_RESTART, or the loop in retrying) waits
// for a peer that has already been and gone. The data it wrote went with it,
// and the open waits for good.
//
// A real shell does not reach this, because the open is made in a forked
// child the signal is not addressed to. Here a `&` job is a goroutine in the
// shell's own process, so every SIGCHLD the shell is sent — a job of its
// stopping, or ending — can land on the thread that is waiting in the open.
// Measured on the corpus row `jobs/wait-for-a-stopped-job-under-the-monitor`
// under CPU load: 10 of 50 runs hung with the job's `read x <f` still in
// open(2) after the parent's `echo >f` had completed, its `kill -STOP` having
// raised exactly that SIGCHLD (#5763). A standalone program that opens a FIFO
// from a goroutine while a child is stopped and continued in a loop lost the
// rendezvous 8 times in 600; the same program with the open made here, 0 in
// 400.
//
// Darwin only, because that is where it was measured: the row has not been
// seen to hang on Linux, and the macOS runner is where it failed.
//
// The signals are held on this thread only, for the length of the call. They
// are not lost: a signal addressed to the process goes to a thread that is
// not holding it, and one addressed to this thread waits for the mask to
// come off.
func WithSignalsHeld[T any](do func() (T, error)) (T, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var old sigset
	if err := threadSigmask(sigBlock, &allSignals, &old); err != nil {
		// Not held, and no worse off than before this existed.
		return do()
	}
	defer func() { _ = threadSigmask(sigSetmask, &old, nil) }()
	return do()
}

// sigset is Darwin's sigset_t: one bit per signal, 1 through 32.
type sigset uint32

var allSignals = ^sigset(0)

const (
	sigBlock   = 1 // SIG_BLOCK
	sigSetmask = 3 // SIG_SETMASK
)

// threadSigmask is pthread_sigmask(3), by the system call behind it: the
// standard library does not expose it on Darwin, and sigprocmask(2) is not
// the same call here — the per-thread one is __pthread_sigmask.
func threadSigmask(how int, set, old *sigset) error {
	_, _, errno := syscall.RawSyscall(syscall.SYS___PTHREAD_SIGMASK,
		uintptr(how), uintptr(unsafe.Pointer(set)), uintptr(unsafe.Pointer(old)))
	if errno != 0 {
		return errno
	}
	return nil
}
