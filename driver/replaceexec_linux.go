// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package driver

import (
	"runtime"
	"syscall"
	"unsafe"
)

// readyExec is an execve with everything it needs already built, so that
// nothing between the descriptor placement and the replacement itself has to
// allocate.
//
// # Why this exists at all
//
// `syscall.Exec` takes strings and converts them — the path, every argument
// and every environment entry — into C strings on the way in. Those
// conversions run *after* placeFiles, which is to say inside the one window
// where this process's descriptor table says something the Go runtime does not
// expect. `sysmon` polls the netpoller on a timer of its own from another
// thread, and if that lands while the epoll descriptor has been replaced by a
// script's file the process dies where it stood:
//
//	runtime: epollwait on fd 5 failed with 22
//	fatal error: runtime: netpoll failed
//
// Stopping the collector (#695) removed the half of that window a *collection*
// caused and could not touch the half a timer causes, because the timer does
// not care what started the work — only that the window is long enough to be
// caught in. So the window is emptied instead of shortened: everything that
// allocates happens before the first descriptor is overwritten, and what is
// left between the placement and the execve is system calls.
//
// Measured on Linux, `-race`, twenty-five runs of the test that reports this,
// with the environment padded so the conversions take a length worth seeing:
// 8000 entries fails 25 of 25 with the conversions inside the window and 0 of
// 25 with them outside it. 2000 entries is 3 of 25 and 0 of 25. Nothing else
// about the run changes, and the crash is CI's own.
//
// # Why unsafe is warranted, and what keeps it correct
//
// There is no allocation-free way to reach execve through the `syscall`
// package's own front door: `Exec` is the front door and the conversions are
// what it is for. What is left is the call `Exec` itself makes, which is a
// RawSyscall taking three pointers.
//
// The invariant is one sentence, and it is the whole of what a later reader
// has to keep: **nothing may allocate between the placement and the execve.**
// Everything else follows from it —
//
//   - The pointer conversions sit inside the RawSyscall call expression, which
//     is the one shape the unsafe.Pointer rules allow a uintptr to be made in.
//     Hoisting one into a variable would be a bug even though it would compile.
//   - The collector is stopped over the same span, so nothing can move or free
//     what those pointers name even if something did allocate.
//   - runtime.KeepAlive holds the three conversions live across the call, for
//     the failure path where there is still a program to hold them for.
//
// This is Linux only, deliberately. Go's own `syscall.Exec` reaches execve
// through libc rather than RawSyscall on darwin, ios, openbsd, solaris,
// illumos and aix — a raw system call is not the supported way in on those —
// and the platforms in between are ones this change cannot be tested on. Every
// other platform keeps `syscall.Exec`, which is also why the darwin instance
// of this bug is not fixed here; see replaceexec_other.go.
type readyExec struct {
	argv0 *byte
	argv  []*byte
	env   []*byte

	// raised is the open-file limit as this process was running under, kept
	// so a failed exec can be given it back. See lowerOpenFileLimit.
	raised   syscall.Rlimit
	lowered  bool
	prepared bool
}

// prepareExec does everything the exec is going to allocate.
func prepareExec(path string, argv, env []string) (readyExec, error) {
	var r readyExec
	var err error
	if r.argv0, err = syscall.BytePtrFromString(path); err != nil {
		return r, err
	}
	if r.argv, err = syscall.SlicePtrFromStrings(argv); err != nil {
		return r, err
	}
	if r.env, err = syscall.SlicePtrFromStrings(env); err != nil {
		return r, err
	}
	r.lowerOpenFileLimit()
	r.prepared = true
	return r, nil
}

// lowerOpenFileLimit puts RLIMIT_NOFILE back to what this process was started
// with, which is what `syscall.Exec` does and what going around it would
// otherwise lose.
//
// The Go runtime raises the soft limit to just under the hard one before any
// code here runs — measured in a container started at 1024/524288, the process
// runs at 524287 — and `syscall.Exec` puts the original back before execve so
// that a command does not inherit the raised one. It matters: a soft limit
// exists to keep programs that use `select` inside their descriptor set, and
// handing one 524287 where bash hands it 1024 breaks it for a reason nobody
// would look for in a shell. `driver/rlimit.go` records the same asymmetry
// from the reading side.
//
// The original is not readable from outside the syscall package, so it is
// obtained the only way it can be: by letting `syscall.Exec` apply it, on an
// exec that cannot succeed. An empty path is ENOENT everywhere, and the
// restore happens before the execve is attempted — after the call the limit
// *is* the original and Getrlimit says so.
//
// It checks rather than assumes. If the limit did not move — nothing to
// restore, or a future runtime that stopped doing this — nothing is recorded
// and the exec goes ahead exactly as it would have. The end-to-end assertion
// is what would notice: a replacement's `ulimit -n` has to agree with an
// ordinary child's, and TestAReplacementInheritsTheOpenFileLimitAChildGets
// says so from the outside, where the mechanism cannot hide.
func (r *readyExec) lowerOpenFileLimit() {
	var before syscall.Rlimit
	if syscall.Getrlimit(syscall.RLIMIT_NOFILE, &before) != nil {
		return
	}
	// Cannot succeed: execve("") is ENOENT. What is wanted is the work it
	// does on the way there.
	if err := syscall.Exec("", nil, nil); err == nil {
		return
	}
	var after syscall.Rlimit
	if syscall.Getrlimit(syscall.RLIMIT_NOFILE, &after) != nil {
		return
	}
	if after.Cur != before.Cur {
		r.raised, r.lowered = before, true
	}
}

// undo puts back what the exec would have consumed, for the path where there
// is still a shell to put it back for. Only the limit: the descriptors are
// gone, which is what a failed `exec` means everywhere.
func (r readyExec) undo() {
	if r.lowered {
		_ = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &r.raised)
	}
}

// execve replaces the process, and must not allocate. It returns only on
// failure.
func (r readyExec) execve() error {
	if !r.prepared {
		return syscall.EINVAL
	}
	reachedPlacement("execve")
	_, _, errno := syscall.RawSyscall(syscall.SYS_EXECVE,
		uintptr(unsafe.Pointer(r.argv0)),
		uintptr(unsafe.Pointer(&r.argv[0])),
		uintptr(unsafe.Pointer(&r.env[0])))
	// Reached only when the exec failed, so the pointers have to have stayed
	// live until here and there is a program left to hold them for.
	runtime.KeepAlive(r.argv0)
	runtime.KeepAlive(r.argv)
	runtime.KeepAlive(r.env)
	return errno
}
