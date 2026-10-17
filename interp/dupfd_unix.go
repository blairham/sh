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
// The copy is asked for in privateCopyFloor's band first and at floor only
// when the band is full, which is what keeps the first pipeline in a run from
// growing the kernel's descriptor table (#6402). A copy in the band can sit on
// a number the dialect publishes, so the caller registers it — see
// privateCopies, which is how a published answer steps *onto* it rather than
// over it. The number is answered beside the file for that reason: reading it
// back through Fd would put the stream into blocking mode.
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
func dupFile(f *os.File, floor int) (*os.File, int, error) {
	conn, err := f.SyscallConn()
	if err != nil {
		return nil, 0, err
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
			//
			// The band first, and the floor only where the band is full. See
			// privateCopyFloor: a copy at or above the kernel's first table
			// makes the kernel grow the table, and on Linux that waits out an
			// RCU grace period (#6402).
			if floor > privateCopyFloor {
				if got, ferr := fcntlInt(int(fd), syscall.F_DUPFD_CLOEXEC, privateCopyFloor); ferr == nil {
					if got < kernelFirstTable {
						dup = got
						return
					}
					// Past the first table already, so the table has grown
					// and the band is full: the copy goes where it went
					// before there was a band.
					_ = syscall.Close(got)
				}
			}
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
		return nil, 0, err
	}
	if duperr != nil {
		return nil, 0, duperr
	}
	// The name travels with the duplicate, because the name is what the file
	// was opened by and a copy of a descriptor is a copy of that. One reader
	// depends on it: holdsDescriptorOnto asks whether any descriptor still
	// points at a substitution's pipe before the path is taken away.
	return os.NewFile(uintptr(dup), f.Name()), dup, nil
}

// raiseScriptFile moves a file a redirection opened into privateCopyFloor's
// band and closes the original, in a dialect that publishes process
// substitutions, answering the file the script's table holds.
//
// The script's descriptor is in this shell's table at the number the script
// names, and in the kernel's at whatever number the open returned — the
// lowest free one. A substitution asks the kernel which numbers are free, so
// a file left there was counted at its real number, which no shell being
// imitated holds, and the substitution stepped over it: `exec 5</dev/null;
// echo <(true)` was `/dev/fd/4` in the ksh dialect where ksh93u+ answers 3,
// `{ echo <(true); } 2>/dev/null; :` the same, and each file the script
// opened cost one more number (#6662). Up in the band it is out of the way of
// every dialect's first answers, as the shell's own ends of a substitution's
// and a pipeline's pipes are (see raiseShellEnd).
//
// A best effort, as those are: a band that is full, or a refusal, leaves the
// file where the open put it.
func (r *Runner) raiseScriptFile(f *os.File) *os.File {
	if !r.lang().ProcessSubstitution {
		return f
	}
	conn, err := f.SyscallConn()
	if err != nil {
		return f
	}
	moved := -1
	if cerr := conn.Control(func(fd uintptr) {
		got, err := fcntlInt(int(fd), syscall.F_DUPFD_CLOEXEC, privateCopyFloor)
		if err != nil {
			return
		}
		if got >= kernelFirstTable {
			// The band is full, and a copy above it would grow the kernel's
			// table. See privateCopyFloor.
			_ = syscall.Close(got)
			return
		}
		moved = got
	}); cerr != nil || moved < 0 {
		return f
	}
	// And in blocking mode, for raiseShellEnd's reason: the copy shares the
	// open file description, Go opens a pipe or a FIFO non-blocking, and the
	// program the file is handed to meets EAGAIN on an empty one — measured,
	// `head` answered `Resource temporarily unavailable` on Linux. The file
	// answered is one Go did not open, so the mode is the description's.
	if err := syscall.SetNonblock(moved, false); err != nil {
		_ = syscall.Close(moved)
		return f
	}
	raised := os.NewFile(uintptr(moved), f.Name())
	_ = f.Close()
	return raised
}
