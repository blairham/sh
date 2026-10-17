// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package testenv

import (
	"os"
	"strings"
	"syscall"
)

// highestInheritedDescriptor bounds the scan. A host that hands a test binary
// descriptors hands it low ones — they are whatever its own table had free —
// and 1024 fcntl calls is microseconds, paid once per suite.
const highestInheritedDescriptor = 1024

// closeInheritedDescriptors closes every descriptor above stdio that this
// process was handed by whoever started it, so the suite's descriptor table is
// assembled from nothing in the same way its environment is.
//
// # Why a suite needs it
//
// The shells these suites run are in-process, and a shell's descriptor table
// is the process's. A test that asks what `>&8`, `exec 9</dev/null; … <&9` or
// `print -u3` does is therefore asking about numbers this process holds, and
// every one of them assumes the numbers it did not open are closed. That is
// true on a developer's machine and was false on the macOS runner (#6944):
//
//	echo hi >&08     write error: Invalid argument   (want Bad file descriptor)
//	cat <&10         cat: stdin: Resource temporarily unavailable
//	print -u3 a 3<f  bad mode on fd 5                (want fd 4)
//
// The descriptors were the go command's. When `go test` has to download a
// module it resolves the proxy's name through the system resolver, which on
// darwin opens `com.apple.netsrc` kernel-control sockets without
// close-on-exec, and every test binary the command starts afterwards inherits
// them on whatever low numbers they landed on. `lsof` in a `go test -exec`
// wrapper shows one (`6u systm … [ctl com.apple.netsrc]`) with a cold module
// cache and none with a warm one. A write to such a socket is EINVAL and a
// read is EAGAIN, which is each message above. CI's cache is keyed on go.sum
// and only saved by a green job, so the first change that added a module made
// every macOS run download, fail, and never save the cache that would have
// stopped the download.
//
// # Why closing is safe here
//
// Everything this process opened itself is close-on-exec — the Go runtime
// opens nothing without it, and neither does os — so a descriptor that lacks
// the flag when TestMain runs is one that survived the exec that started this
// binary: the host's. A test that wants a child to inherit something passes it
// explicitly through ExtraFiles to a re-executed copy of this binary, and that
// copy takes the Assembled branch and never reaches Run.
//
// A fuzz worker is the one other process that is handed descriptors on
// purpose — the coordinator's pipes — and runs TestMain on the way to using
// them, so it is left alone.
func closeInheritedDescriptors() {
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-test.fuzzworker") {
			return
		}
	}
	for fd := 3; fd <= highestInheritedDescriptor; fd++ {
		flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(syscall.F_GETFD), 0)
		if errno != 0 || flags&syscall.FD_CLOEXEC != 0 {
			continue
		}
		_ = syscall.Close(fd)
	}
}
