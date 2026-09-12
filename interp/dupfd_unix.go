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
// Close-on-exec, because everything Go opens is: a descriptor reaches a child
// through the table childFiles rebuilds, by number, and one that leaked
// through the kernel behind that table's back would be open in every command
// the shell runs — the leak that /dev/fd process substitution was rejected
// for. The fork lock is held across the pair so that no other goroutine's
// fork can happen between the duplicate existing and the flag being set.
func dupFile(f *os.File) (*os.File, error) {
	conn, err := f.SyscallConn()
	if err != nil {
		return nil, err
	}
	var dup int
	var duperr error
	if err := conn.Control(func(fd uintptr) {
		syscall.ForkLock.RLock()
		defer syscall.ForkLock.RUnlock()
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
