// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import (
	"os"
	"syscall"
)

// dupFile opens a second descriptor onto the same open file, which is the
// half of a fork that the descriptor table needs — see ownDescriptors.
//
// Through SyscallConn rather than Fd, and that is the whole of why this is a
// function rather than two lines at the call site. Fd hands out a raw number
// and keeps no reference, so a close on another goroutine between reading it
// and duplicating it leaves this duplicating whatever has since taken the
// number — which is the silent half of the bug this exists to end. Control
// holds the file open for as long as it runs. It also has no business putting
// a stream back into blocking mode, which is Fd's other effect and is not
// asked for here.
//
// floor is the lowest number the duplicate may take, or 0 for wherever the
// kernel puts it. **A copy the shell keeps for its own plumbing is not
// supposed to be in a published answer**, and before this it was: `syscall.Dup`
// answers with the lowest free number, so a substitution body's private copy
// of the shell's own streams landed in the low region — measured 2026-09-21,
// one descriptor per body at 4 and then 6, which is what left ksh93's
// `3 4 5` reading `3 5 7` here after the pipe's own ends were moved out.
// See Runner.privateFdFloor and raiseShellEnd, which is the same rule about
// the other half of the same pipe.
//
// A floor the kernel refuses falls back to the plain duplicate rather than
// failing. Under a low `ulimit -n` the floor is past the limit, and a failure
// here would put back the *sharing* this function exists to end (#2116) over
// a question of tidiness.
//
// Close-on-exec, because everything Go opens is: a descriptor reaches a child
// through the table childFiles rebuilds, by number, and one that leaked
// through the kernel behind that table's back would be open in *every* command
// the shell runs. That is the rule a process substitution's `/dev/fd/N` end
// follows too — parked close-on-exec and put in the table — which is why it
// reaches the command that named the path and nothing else; see
// newProcSubPipe. The fork lock is held across the pair so that no other
// goroutine's fork can happen between the duplicate existing and the flag
// being set.
func dupFile(f *os.File, floor int) (*os.File, error) {
	conn, err := f.SyscallConn()
	if err != nil {
		return nil, err
	}
	var dup int
	var duperr error
	if err := conn.Control(func(fd uintptr) {
		syscall.ForkLock.RLock()
		defer syscall.ForkLock.RUnlock()
		if floor > 0 {
			// F_DUPFD_CLOEXEC carries the flag itself, so there is no pair to
			// hold the fork lock across on this road — it is held anyway,
			// because the fallback below has one.
			if got, ferr := fcntlInt(int(fd), syscall.F_DUPFD_CLOEXEC, floor); ferr == nil {
				dup = got
				return
			}
		}
		dup, duperr = syscall.Dup(int(fd))
		if duperr != nil {
			return
		}
		syscall.CloseOnExec(dup)
	}); err != nil {
		return nil, err
	}
	if duperr != nil {
		return nil, duperr
	}
	// The name travels with the duplicate, because the name is what the file
	// was opened by and a copy of a descriptor is a copy of that. One reader
	// depends on it: holdsDescriptorOnto asks whether any descriptor still
	// points at a substitution's pipe before the path is taken away.
	return os.NewFile(uintptr(dup), f.Name()), nil
}
