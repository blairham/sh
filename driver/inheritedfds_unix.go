// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver

import (
	"os"
	"strconv"
	"sync"
	"syscall"
)

// firstExtraFd and maxInheritedFd are interp's layout for the descriptor
// table beyond the three named streams, repeated here because the two
// packages meet across a slice rather than a shared constant: entry i of
// interp.Runner.InheritedFiles is descriptor 3+i, and nothing above 1024 is
// carried. interp/inheritedfds.go has the reasoning for both numbers.
const (
	firstExtraFd   = 3
	maxInheritedFd = 1024
)

// inheritedFiles are the descriptors this process was started holding beyond
// standard input, output and error, ready to be published to a script.
//
// Once per process, and that is not only an optimization. Each descriptor
// found is *duplicated*, so scanning twice would hold two copies of every one
// of them; and the answer cannot change, because the question is what this
// process was handed before it ran, which nothing that happens later revises.
func inheritedFiles() []*os.File { return scanOnce() }

var scanOnce = sync.OnceValue(scanInheritedFiles)

// scanInheritedFiles finds the descriptors this process inherited and hands
// back a copy of each, laid out the way interp reads them.
//
// **Close-on-exec is the discriminator.** The Go runtime opens every
// descriptor of its own close-on-exec — that is the whole reason a shell
// written in Go has to rebuild the outbound table by hand — so a descriptor
// that *would* survive an exec is one this program did not open. It came from
// whoever started the process, which is exactly the question being asked, and
// it is the only kind a script may be shown. Getting this wrong in the
// permissive direction would hand a script the runtime's own files.
//
// The descriptors are listed rather than probed. /proc/self/fd and /dev/fd
// both name what is open, and one fcntl per *number* would be a scan of the
// process's open-file limit — which is 2560 on a stock macOS, commonly 1048576
// under systemd, and unbounded in principle. That is a poor thing to do on
// every shell's startup path, so the bounded scan is only the fallback for a
// platform that publishes neither directory.
//
// Each one is duplicated, not adopted by number, and the copy is close-on-exec
// where the original was not. Adopting would put a descriptor the *caller*
// owns into a table this shell closes entries from, and would leave the
// program that started us reading through a number this process had handed
// back to the kernel. The copy being close-on-exec is right for the same
// reason the shell's own files are: a child is given the table by number
// through os/exec's ExtraFiles, so nothing needs to leak through an exec to
// get there.
//
// The original is deliberately left open. It is not close-on-exec, which is
// what makes `exec cmd` — the path that replaces this process rather than
// forking a child — still hand the descriptor to its replacement by number,
// as every shell in the panel does. Closing it would take that away.
func scanInheritedFiles() []*os.File {
	var files []*os.File
	for _, fd := range openDescriptors() {
		if fd < firstExtraFd || fd > maxInheritedFd || closeOnExec(fd) {
			continue
		}
		dup, err := fcntl(fd, syscall.F_DUPFD_CLOEXEC, firstExtraFd)
		if err != nil {
			// A descriptor that was listed and is now gone, or one this
			// process may not duplicate: it is not there to publish, and a
			// script asking for it gets what it gets for any other number
			// nothing is open on.
			continue
		}
		for len(files) <= fd-firstExtraFd {
			// Gaps stay nil, which is a number nothing was inherited on:
			// `sh 5<&0 script` leaves 3 and 4 as unopened as they were.
			files = append(files, nil)
		}
		files[fd-firstExtraFd] = os.NewFile(uintptr(dup), "/dev/fd/"+strconv.Itoa(fd))
	}
	return files
}

// openDescriptors lists the descriptors this process holds.
//
// /proc/self/fd on Linux, /dev/fd on the BSDs and macOS, and a bounded probe
// where neither is mounted. The directory is read and closed before anything
// is asked about a number in it, so the descriptor the listing itself used
// falls out on its own — it is gone by the time it would be examined, and
// anything that took its place is close-on-exec because this program opened
// it.
func openDescriptors() []int {
	for _, dir := range []string{"/proc/self/fd", "/dev/fd"} {
		if fds, ok := readFdDir(dir); ok {
			return fds
		}
	}
	// One fcntl per number up to the same ceiling the table is bounded by.
	// A thousand cheap calls once at startup is affordable where a scan of
	// the process limit is not, and a platform that reaches this has already
	// declined to answer the cheaper way.
	fds := make([]int, 0, maxInheritedFd-firstExtraFd)
	for fd := firstExtraFd; fd <= maxInheritedFd; fd++ {
		if _, err := fcntl(fd, syscall.F_GETFD, 0); err == nil {
			fds = append(fds, fd)
		}
	}
	return fds
}

// readFdDir reads one of the directories that names a process's descriptors,
// reporting whether it answered at all.
//
// An empty listing is not an answer: the directory always contains at least
// the descriptor being used to read it, so nothing at all means the path is
// not the thing it was taken for, and the caller should try the next.
func readFdDir(dir string) ([]int, bool) {
	d, err := os.Open(dir)
	if err != nil {
		return nil, false
	}
	names, err := d.Readdirnames(-1)
	// Closed before the numbers are examined, so the listing's own
	// descriptor is not among the ones examined.
	_ = d.Close()
	if err != nil || len(names) == 0 {
		return nil, false
	}
	fds := make([]int, 0, len(names))
	for _, name := range names {
		if fd, err := strconv.Atoi(name); err == nil {
			fds = append(fds, fd)
		}
	}
	return fds, true
}

// closeOnExec reports whether a descriptor would be closed by an exec, which
// is true of everything the Go runtime opened and false of what was inherited.
// A descriptor that is not open at all answers true, because it is not one to
// publish either.
func closeOnExec(fd int) bool {
	flags, err := fcntl(fd, syscall.F_GETFD, 0)
	if err != nil {
		return true
	}
	return flags&syscall.FD_CLOEXEC != 0
}

func fcntl(fd, cmd, arg int) (int, error) {
	r, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(cmd), uintptr(arg))
	if errno != 0 {
		return 0, errno
	}
	return int(r), nil
}
