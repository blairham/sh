// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"os"
	"sync"

	"github.com/blairham/sh/interp"
)

// A record lock is the process's, and this shell's subshells are not
// processes. So the system answers "yes" to every body of this shell at once,
// and the question of *which body* holds a file is kept here instead.
//
// Measured against zsh 5.9.2, where every one of these is a fork:
//
//	written                                        zsh 5.9.2
//	zsystem flock f; zsystem flock -t 0 f          0, the same process
//	zsystem flock f; ( zsystem flock -t 0 f )      1, failed to lock file
//	zsystem flock f; x=$(zsystem flock -t 0 f)     1
//	zsystem flock f; zsystem flock -t 0 f | cat    1, and the last element
//	                                               of a pipeline is 0
//	( zsystem flock f; … ) &  then  -t 0 f         1 while the job holds it
//	( zsystem flock -r f; … ) &  then  -r -t 0 f   0, and a write lock is 1
//	zsystem flock -r f; ( zsystem flock -t 0 f )   1, and -r there is 0
//
// and a blocking `zsystem flock f` behind a job holding it waits until the
// job has ended. The ending is what releases it, so each holder registers
// its own release with the process it belongs to — see interp.Process.
//
// A holder is live only while its descriptor is open. That is what the
// system would have said too — a lock goes with the last descriptor on the
// file in its process — and it keeps a holder whose descriptor a script
// closed by hand from excluding anybody forever.
type systemLockHolder struct {
	info os.FileInfo
	file *os.File
	// sys is the system's number for file, which is how `-u` finds it:
	// the script names a descriptor of this shell's, and that is what
	// the runner translates it to.
	sys  int
	proc *interp.Process
	read bool
}

var systemLockHolders struct {
	mu   sync.Mutex
	list []systemLockHolder
}

// live reports whether the holder's descriptor is still open. Asked through
// the file's own reference count rather than by reading its number, because
// the body that holds it may be closing it on another goroutine right now.
func (h systemLockHolder) live() bool {
	conn, err := h.file.SyscallConn()
	if err != nil {
		return false
	}
	return conn.Control(func(uintptr) {}) == nil
}

// systemLockClaim records that proc holds the file f is open on, unless a
// body of this shell in another process holds it in a way that excludes this
// one: any write lock, or a read lock when this is a write. Answered and
// recorded under one lock, so two bodies asking at once cannot both win.
func systemLockClaim(f *os.File, proc *interp.Process, read bool) bool {
	info, err := f.Stat()
	if err != nil {
		// Nothing to compare by, so this process's answer stands: the
		// system has already said yes.
		return true
	}
	systemLockHolders.mu.Lock()
	defer systemLockHolders.mu.Unlock()
	kept := systemLockHolders.list[:0]
	for _, h := range systemLockHolders.list {
		if h.live() {
			kept = append(kept, h)
		}
	}
	systemLockHolders.list = kept
	for _, h := range kept {
		if h.proc != proc && os.SameFile(h.info, info) && !(h.read && read) {
			return false
		}
	}
	systemLockHolders.list = append(systemLockHolders.list,
		systemLockHolder{info: info, file: f, sys: int(f.Fd()), proc: proc, read: read})
	proc.AtExit(func() { systemLockDrop(proc, -1) })
	return true
}

// systemLockDrop forgets what proc holds: on the file open at the system
// descriptor sys, or every file when sys is negative, which is the process
// ending. A process's locks on one file go together, as they do for the
// system, where closing any descriptor on the file gives up all of them.
func systemLockDrop(proc *interp.Process, sys int) {
	systemLockHolders.mu.Lock()
	defer systemLockHolders.mu.Unlock()
	var info os.FileInfo
	if sys >= 0 {
		for _, h := range systemLockHolders.list {
			if h.proc == proc && h.sys == sys {
				info = h.info
				break
			}
		}
		if info == nil {
			return
		}
	}
	kept := systemLockHolders.list[:0]
	for _, h := range systemLockHolders.list {
		if h.proc == proc && (info == nil || os.SameFile(h.info, info)) {
			continue
		}
		kept = append(kept, h)
	}
	systemLockHolders.list = kept
}
