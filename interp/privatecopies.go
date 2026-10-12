// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"
	"sync"
)

// kernelFirstTable is how many descriptors a process's table starts with on
// Linux, and so the first number whose use makes the kernel grow it.
//
// Growing a table that is shared between threads — and every Go process's is
// — waits out an RCU grace period before the call that needed the room
// returns. Measured for #6396 in the golang:1.26.9 image: the one F_DUPFD that
// landed on 64 took 8 to 13 ms of wall time, and the runtime's idle threads
// kept waking for as long as it was blocked. driver keeps its own start inside
// this table for that reason; see driver/lowfds_linux.go.
const kernelFirstTable = 64

// privateCopyFloor is where a concurrent body's private copies of the table
// are asked for first: the middle of the kernel's first table.
//
// They used to start at Runner.privateFdFloor, which is above every number the
// dialect could publish — and that is 64 or more in every dialect, because the
// wish list offers sixty-four numbers. So the first pipeline in a run grew the
// table: `true | true` left FDSize at 128 where `true` leaves 64, and cost
// +0.41 ms of CPU and +11 ms of wall per run in that image (#6402).
//
// The middle, because both ends are where answers come from. bash publishes
// down from 63 and every other rule counts up from 3, 10 or 11, so a copy here
// is reached only by a script holding twenty-odd substitutions or descriptors
// at once. And where one *is* reached, the answer does not move: the copy is
// registered in privateCopies, and parkDescriptor publishes a number held by
// one exactly as it publishes a number a fork would have freed. So the floor
// decides how often that happens, never what the script sees.
//
// A band that is full — a copy that would land at kernelFirstTable or above —
// goes to the old floor, which is what every copy did before this.
const privateCopyFloor = 32

// privateCopies are the descriptor numbers this process holds as a body's
// private copies of the table, which no fork would hold.
//
// A real shell's concurrent body is a fork, so the copies a goroutine needs
// in its place are not in *anybody's* table there: not the parent's, not a
// sibling's and not the body's own. A substitution that wishes for one of
// these numbers is therefore wishing for a number that is free in every
// shell this one imitates, and it gets it — published, with its descriptor
// parked elsewhere, the way substFdView's released numbers are.
//
// The shell's own end of a substitution pipe is one too (#6406). In a real
// shell that end belongs to the substitution's body, which is a fork, so the
// parent's table does not hold it either. See raiseShellEnd.
//
// Process-wide rather than per Runner for that reason: the kernel's table is
// one table, and a sibling's copy is in the way exactly as much as one's own.
// A slice and a lock, because it holds one entry per live copy — a handful —
// and is read only when a wish has already missed.
var privateCopies struct {
	sync.Mutex
	fds []int
}

// holdPrivateCopy records fd as a private copy.
func holdPrivateCopy(fd int) {
	privateCopies.Lock()
	privateCopies.fds = append(privateCopies.fds, fd)
	privateCopies.Unlock()
}

// releasePrivateCopy forgets fd. Called *before* the descriptor is closed, so
// that a number the kernel may already have handed to something else is never
// taken for a copy: in the window between the two the number reads as held,
// which is what it is.
func releasePrivateCopy(fd int) {
	privateCopies.Lock()
	if i := slices.Index(privateCopies.fds, fd); i >= 0 {
		privateCopies.fds = slices.Delete(privateCopies.fds, i, i+1)
	}
	privateCopies.Unlock()
}

// isPrivateCopy reports whether fd is held as a private copy.
func isPrivateCopy(fd int) bool {
	privateCopies.Lock()
	defer privateCopies.Unlock()
	return slices.Contains(privateCopies.fds, fd)
}
