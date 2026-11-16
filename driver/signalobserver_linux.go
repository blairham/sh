// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"sync"
	"syscall"
	"unsafe"
)

// An untrapped PROF from another process ends the shell here by the kernel's
// own default action, because nothing in the process can be told one arrived.
//
// The runtime keeps PROF for its profiler and answers it in its own handler
// before os/signal is consulted, so a PROF from outside ended nothing, where
// bash 5.3, ksh93u+m, zsh 5.9 and dash 0.5.12 all die of it at 155 — measured
// 2026-11-04 in golang:1.26.9, in a `read`, an external `sleep` and a busy
// loop (#7046). darwin's kernel can be asked about deliveries and is (see
// signalobserver_darwin.go); this one cannot be in pure Go: signalfd reads
// only a signal that is blocked on every thread, and the runtime unblocks
// PROF on every thread it starts.
//
// So while the shell does nothing with PROF, its disposition is the kernel's
// default, and the kernel ends the process the way it ends every shell in the
// panel — silently, at 128 plus the number, wherever the shell is standing.
// The runtime's handler is put back the moment it is wanted:
//
//   - by a `trap` for PROF, handled or ignored, so the shell does not die of
//     what it arranged. A handler still never runs for one from outside here,
//     which is the half of #7056 that only a handler written in assembly could
//     reach, and the maintainer decided against one (2026-11-04).
//   - while the CPU profiler runs, whose ticks are PROF: the default is not
//     put on at all then, which is what keeps `make pgo`, profiling shells
//     in-process, alive. A profile *started* while the default is on would
//     end the process at its first tick — the runtime does not put its
//     handler back on a start, believing it is still there — and nothing
//     here starts one.
//   - when the watch comes down, so a process that goes on after its run has
//     the runtime's handling back exactly as it was.
//
// A command the shell starts is unaffected either way: the runtime resets
// every signal it handles to the default between fork and exec.
//
// One consequence is the kernel's and not ours: a death by the default action
// runs no EXIT trap. Measured 2026-11-04 on macOS, bash 5.3, bash 3.2 and dash
// run none for a PROF from outside either; ksh93 does.

var profDefault struct {
	mu sync.Mutex
	// on is set while PROF's disposition is the default by this file's doing.
	on bool
	// runtimes is the disposition it replaced, put back as it was read.
	runtimes [dispositionBytes]byte
}

// profToItsDefault puts PROF at the kernel's default action, unless the
// profiler is running or it already is. Called where the shell does nothing
// with PROF: as the watch is set up, and after `trap - PROF`.
func profToItsDefault() {
	profDefault.mu.Lock()
	defer profDefault.mu.Unlock()
	if profDefault.on || profilerRunning() {
		return
	}
	var old, act [dispositionBytes]byte
	if rtSigaction(syscall.SIGPROF, &act, &old) != nil {
		return
	}
	profDefault.runtimes, profDefault.on = old, true
}

// profToTheRuntime puts back the runtime's handler for PROF, where
// profToItsDefault took it away.
func profToTheRuntime() {
	profDefault.mu.Lock()
	defer profDefault.mu.Unlock()
	if !profDefault.on {
		return
	}
	if rtSigaction(syscall.SIGPROF, &profDefault.runtimes, nil) == nil {
		profDefault.on = false
	}
}

// rtSigaction is rt_sigaction(2) with whole buffers: see die_linux.go for why
// the layout of the structure is never written down here. A nil act or old
// is a nil pointer to the kernel, which reads or writes nothing there.
func rtSigaction(sig syscall.Signal, act, old *[dispositionBytes]byte) error {
	var a, o uintptr
	if act != nil {
		a = uintptr(unsafe.Pointer(act))
	}
	if old != nil {
		o = uintptr(unsafe.Pointer(old))
	}
	if _, _, errno := syscall.Syscall6(syscall.SYS_RT_SIGACTION, uintptr(sig), a, o, sigsetBytes, 0, 0); errno != 0 {
		return errno
	}
	return nil
}

// observeTheUnforwarded is, on this kernel, PROF at its default for the
// watch's length. ch is darwin's to use.
func observeTheUnforwarded(chan os.Signal) (stop func()) {
	profToItsDefault()
	return profToTheRuntime
}

// heldByTheObserver: there is no observer here, and so no descriptor of one.
func heldByTheObserver(int) bool { return false }
