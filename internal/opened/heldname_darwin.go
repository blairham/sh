// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"io/fs"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// sysPthreadFchdir is __pthread_fchdir, the call behind pthread_fchdir_np:
// set the *calling thread's* working directory to a descriptor, or with -1
// put the thread back on the process's. By number for walkat_darwin.go's
// reason — the standard library does not expose it — and stable since 10.5.
//
// Measured 2026-10-15 on Darwin 27.0.0 with a scratch Go program: after the
// call, a relative os.Stat and os.ReadDir from that thread answered from the
// deleted directory the descriptor held, and a /bin/sh started by os/exec
// from the same locked thread listed two entries (`.` and `..`) and could not
// create a file, where the process's own cwd held several; the process's
// cwd was unchanged afterwards. So a fork inherits the thread's directory,
// which is what HeldStart needs.
const sysPthreadFchdir = 349

const devFdPrefix = "/dev/fd/"

// heldFds is every descriptor HeldName has answered for and ForgetHeld has
// not withdrawn. Only these are recognized: a script that writes `/dev/fd/3/x`
// itself gets the kernel's own answer for it, ENOTDIR.
var heldFds sync.Map // int -> struct{}

func heldName(fd int) string {
	heldFds.Store(fd, struct{}{})
	return devFdPrefix + strconv.Itoa(fd)
}

func forgetHeld(fd int) { heldFds.Delete(fd) }

// splitHeld recognizes a HeldName, optionally followed by a path under it,
// and answers with the descriptor and the remainder relative to it ("." for
// the directory itself).
func splitHeld(path string) (int, string, bool) {
	if !strings.HasPrefix(path, devFdPrefix) {
		return 0, "", false
	}
	num, rest, _ := strings.Cut(path[len(devFdPrefix):], "/")
	fd, err := strconv.Atoi(num)
	if err != nil {
		return 0, "", false
	}
	if _, ok := heldFds.Load(fd); !ok {
		return 0, "", false
	}
	// `/dev/fd/N//f` is `f` under N, not `/f`.
	rest = strings.TrimLeft(rest, "/")
	if rest == "" {
		rest = "."
	}
	return fd, rest, true
}

func pthreadFchdir(fd int) syscall.Errno {
	_, _, errno := syscall.RawSyscall(sysPthreadFchdir, uintptr(fd), 0, 0)
	return errno
}

// fromHeldThread runs do on a thread whose working directory is fd's.
//
// The goroutine is locked to its thread for the length of it, so nothing else
// runs there while the thread is somewhere other than the process. If the
// thread cannot be put back, it stays locked, and the runtime ends a thread
// whose goroutine exits locked — so the stray directory goes with it rather
// than being handed to the next goroutine scheduled there.
func fromHeldThread(fd int, do func()) error {
	runtime.LockOSThread()
	if errno := pthreadFchdir(fd); errno != 0 {
		runtime.UnlockOSThread()
		return &fs.PathError{Op: "fchdir", Path: devFdPrefix + strconv.Itoa(fd), Err: errno}
	}
	do()
	if pthreadFchdir(-1) == 0 {
		runtime.UnlockOSThread()
	}
	return nil
}

func heldChdir(fd int) error { return syscall.Fchdir(fd) }
