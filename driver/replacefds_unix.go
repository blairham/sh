// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver

import (
	"os"
	"syscall"
)

// placeFiles puts the shell's descriptor table on the numbers a replacement
// will find it under, and leaves it there for the exec to carry across.
//
// This is the third and last half of one boundary, and the only one where the
// numbers have to be *made* rather than handed over. An external child is
// given the table by os/exec, which renumbers it during the fork; a
// replacement has no fork to do it in, so the process's own table is what the
// command inherits and it has to say the right thing before execve.
//
// Two things are in the way, and they are not the same thing:
//
//   - The Go runtime opens every descriptor of its own close-on-exec, so
//     nothing the shell holds would survive the exec at all.
//   - A Runner's descriptor 3 is not the process's descriptor 3. `exec 3>h`
//     opens a file that lands on whatever number was free, and the script's
//     number exists only in the interpreter's table.
//
// So clearing the flag is not enough on its own; each file has to be
// duplicated onto its number, which clears the flag as a side effect.
//
// Entry i is descriptor i, named streams included, and they are placed here
// rather than left to the process because there is nothing else left to place
// them: `exec >log` puts a file on the interpreter's standard output and the
// process's own 1 still points at the terminal, so the replacement wrote there
// until this reached down to zero.
//
// A nil entry is a number that must not be open in the replacement, and it is
// closed only where it would otherwise survive. Everything this process opened
// is close-on-exec and goes on its own; what is left is a descriptor the
// process was *started* with, whose original is still open here precisely
// because it is not close-on-exec — so `exec 3<&-; exec cmd` would hand 3 to
// the command through the kernel, behind the table's back, where every shell
// in the panel finds it closed. Asking the flag first is what keeps this from
// closing the runtime's own files on the way past, and 0, 1 and 2 are
// deliberately the case it does *not* keep it from: they are the process's own
// and are not close-on-exec, so a nil there closes them, which is what `exec
// >&-; exec cmd` means in every shell measured.
//
// The one number that is left alone is one the Go runtime holds. That is not
// a descriptor this process may hand back, and overwriting it is fatal rather
// than reportable — see runtimeDescriptors, and lowfds_unix.go for why there
// is almost never one in reach.
//
// Errors are dropped rather than reported. Every one of these calls fails only
// for a descriptor that is not there to place, and the caller execs regardless
// — a command missing one number of its table is a smaller failure than a
// shell that refused to run it.
func placeFiles(files []*os.File) {
	if len(files) == 0 {
		return
	}
	last := len(files) - 1

	// Where each entry's file currently is, with anything sitting on a number
	// this is about to write to moved out of the way first. Without that step
	// the placement can destroy its own input: with the file for descriptor 7
	// currently on 3, placing 3 first would overwrite it, and 7 would end up
	// aiming at whatever 3 became. A file already on its own number is left
	// where it is, because that is the one case a placement need not move.
	sources := make([]int, len(files))
	for fd, f := range files {
		if f == nil {
			sources[fd] = -1
			continue
		}
		// A closed file answers with an invalid descriptor rather than an
		// error, and there is nothing to place for one.
		src := int(f.Fd())
		if src < 0 {
			sources[fd] = -1
			continue
		}
		if src != fd && src <= last {
			// Close-on-exec, because this copy exists only for the moment
			// between here and the placement: it must not reach the command
			// on a number of its own.
			if moved, err := fcntl(src, syscall.F_DUPFD_CLOEXEC, last+1); err == nil {
				src = moved
			}
		}
		sources[fd] = src
	}

	for fd, src := range sources {
		if heldByTheRuntime(fd) {
			// A number the Go runtime answered for at startup, and the one
			// thing this must not do is write over it: the runtime finding
			// its poller or its signal pipe replaced by a script's file is a
			// fatal error rather than a reportable one, and the shell dies
			// where it was about to become the command. The command loses the
			// descriptor instead, which is what every other unplaceable
			// number costs it. lowfds_unix.go is why this is rare enough to
			// be worth a line rather than a design.
			continue
		}
		switch {
		case src < 0:
			if !closeOnExec(fd) {
				_ = syscall.Close(fd)
			}
		case src == fd:
			// Already on its number: only the flag stands between it and the
			// command. F_SETFD takes the whole flag word, and close-on-exec
			// is the only flag there is.
			_, _ = fcntl(fd, syscall.F_SETFD, 0)
		default:
			// The duplicate is not close-on-exec however the original was,
			// which is the whole of what this has to leave behind.
			_ = dup2(src, fd)
		}
	}
}
