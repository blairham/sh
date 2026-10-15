// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "sync"

// transientFds orders the moments this process briefly holds low descriptors
// a fork would not have, against a substitution's choice of a number.
//
// **Starting a program opens descriptors in this process for the length of
// the start.** os/exec opens the null device for a stream it was given none
// for, and the fork underneath it makes a pipe to hear whether the exec
// worked, holding its reading end until the child has exec'd or failed. All of
// them take this process's lowest free numbers, and all of them are gone again
// by the time Start returns. The shell's own plumbing has the same shape in a
// smaller way: `os.Pipe` and an open land low and are moved into the private
// band a moment later — see raiseShellEnd and newHeldDirectory.
//
// A shell that forks its pipeline elements never sees any of that from inside
// an element: the sibling's start happens in the sibling's process. Here the
// elements are goroutines over one table, so a substitution numbered while a
// sibling was starting a program stepped over that start's descriptors and
// published whatever was left. Measured 2026-10-15 on darwin, ksh dialect,
// with the table dumped at each park: `{ echo <(true) <(true) <(true) >&2; }
// | /bin/cat` found a pipe pair on 3 and 4 that belonged to nobody in the
// script, and answered `3 4 6`, `3 5 6` or `4 5 6` by the scheduler's choice,
// where ksh93u+ answers `3 4 5` every time. A builtin sibling — `{ :; }`, a
// `read` loop — answered `3 4 5` forty times out of forty, which is the
// control that names the start rather than the pipeline (#6454).
//
// So each of those moments holds this for reading, and newProcSubPipe holds
// it for writing from the pipe it makes to the number it publishes. A park
// waits for any start already in flight to finish, which is one fork and exec
// at most, and a start waits for a park in flight, which is a handful of fcntl
// calls. The cost where there is no substitution is one uncontended read lock
// per program started — tens of nanoseconds against a fork.
//
// Nothing may be started, opened or waited for while holding it for writing:
// the park does nothing but system calls on descriptors it already has, and
// that is what keeps the two sides from waiting on each other.
var transientFds sync.RWMutex

// withTransientFds runs f inside the window a park waits out.
func withTransientFds(f func() error) error {
	transientFds.RLock()
	defer transientFds.RUnlock()
	return f()
}
