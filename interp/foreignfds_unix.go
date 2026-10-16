// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import (
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
)

// Descriptors on a low number that are not in the table being numbered.
//
// A substitution asks the kernel which of the numbers its dialect wants are
// free, and the kernel answers for the one table this process has. Two kinds
// of descriptor sit in that table on a number a forking shell would have free,
// for as long as they live rather than for a moment, and transientFds cannot
// wait them out:
//
//   - **A sibling's substitution.** Its end is parked on the number it
//     published, in the one table, while the body that made it runs. In a
//     forking shell it is in the sibling's own table and nobody else's. See
//     fdlineage.go, which carries the measurement.
//   - **What starting a program leaves behind.** os/exec holds a pipe of its
//     own for each stream of the child's that is not a file — a command
//     substitution's output, a test's buffer — for the life of the child, and
//     on Linux a pidfd as well (#6476). Each takes the lowest free number
//     inside Start, and none of them is any shell's: a forking shell's child
//     writes straight into the pipe its parent made. Measured on Linux in
//     golang:1.26.9 with the claim taken out, `{ echo <(true) <(true)
//     <(true) >&2; } | /bin/true` stepped a substitution over the sibling's
//     pidfd in ten runs of ten.
//
// Both are recorded here as claims: the number, and whose table it is in —
// an owner node, or none. A wish that misses on a claimed number asks whether
// the claim is in the table of the runner making the substitution; where it
// is not, the number is free in the shell being imitated and is published the
// way a private copy's is, with the descriptor parked out of the way.
//
// **A claim must not outlive its descriptor**, or a number something else has
// since taken reads as somebody else's. A substitution's end is closed by the
// shell that made it, which drops the claim — see dropFdClaimMade for the
// order. What a start leaves behind is closed by os/exec in Wait, behind every
// caller's back, so that claim carries what the descriptor *is*, as the
// filesystem identifies it, and a reading that finds something else on the
// number drops it and answers "held", which is what the kernel said.
//
// Read only on a wish that has already missed, which no substitution outside
// a crowded or concurrent table ever reaches.
var foreignFds struct {
	sync.Mutex
	claims map[int]fdClaim
}

// fdClaim is one claimed number.
type fdClaim struct {
	// owner is the node whose table the descriptor is in, or nil for one in
	// nobody's.
	owner *fdLineage
	// stamp is the clock reading when it was claimed. See fdLineage.holds.
	stamp uint64
	// dev and ino are what the descriptor was when it was claimed, where
	// identified says they were taken.
	identified bool
	dev, ino   uint64
}

// fdIdentity is what fd is, as the filesystem identifies it, or false where
// nothing is open on it.
func fdIdentity(fd int) (dev, ino uint64, ok bool) {
	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		return 0, 0, false
	}
	return uint64(st.Dev), uint64(st.Ino), true
}

// claimFd records fd as owner's, or nobody's for a nil owner, and answers the
// claim's stamp, or zero where nothing is open on fd.
//
// identify says the claim carries what the descriptor is, for a claimant that
// will not be there to drop it — os/exec closes what a start left behind in
// Wait. A substitution's end is dropped by whoever closes it (see
// dropFdClaimMade), so it costs its park no fstat.
func claimFd(fd int, owner *fdLineage, identify bool) uint64 {
	var dev, ino uint64
	if identify {
		var ok bool
		if dev, ino, ok = fdIdentity(fd); !ok {
			return 0
		}
	}
	stamp := fdClock.Add(1)
	foreignFds.Lock()
	if foreignFds.claims == nil {
		foreignFds.claims = make(map[int]fdClaim)
	}
	foreignFds.claims[fd] = fdClaim{owner: owner, stamp: stamp, identified: identify, dev: dev, ino: ino}
	foreignFds.Unlock()
	return stamp
}

// dropFdClaim forgets whatever claim is on fd. Called by a park on a number
// it has just put a descriptor of its own on, which no earlier claim can be
// about.
func dropFdClaim(fd int) {
	foreignFds.Lock()
	delete(foreignFds.claims, fd)
	foreignFds.Unlock()
}

// dropFdClaimMade forgets fd's claim if it is still the one stamped stamp.
//
// Called *after* the descriptor is closed, which is the order that keeps a
// sibling from stepping over it: in between, the number reads as the
// claimant's, and a sibling numbering then publishes it as a fork would. The
// other direction is what the stamp and the parks cover. A park that puts a
// descriptor of its own on the number in between replaces the claim or drops
// it — see newProcSubPipe — so it is never told its own descriptor is
// somebody else's; and the stamp keeps this from dropping the claim that park
// made. Anything else that lands there in between is not on a number a script
// can see (see claimStartLeftovers), and the identity outsideTable compares
// catches it besides.
func dropFdClaimMade(fd int, stamp uint64) {
	if stamp == 0 {
		return
	}
	foreignFds.Lock()
	if c, ok := foreignFds.claims[fd]; ok && c.stamp == stamp {
		delete(foreignFds.claims, fd)
	}
	foreignFds.Unlock()
}

// outsideTable reports whether fd is open only on a claim that is not in the
// table of the runner whose node is at: a sibling's, a later one of an
// ancestor's, or nobody's. gone says nothing is open on fd any more — the
// park asking found it taken a moment ago, and what took it has let go.
//
// **The claim is read before the descriptor is looked at**, and the order is
// the whole of the answer's being right. A claim is dropped after its
// descriptor is closed (see dropFdClaimMade), so a claim missing at the
// reading was closed before it, and the look that follows sees the number
// as it is now; a claim present was open at the reading, and the look says
// whether it still is. Looked at first, a sibling's end could be open at the
// look and closed and unclaimed at the reading, and the number read as held
// by nobody in particular: about one run in a thousand of two bodies
// numbering at once, on Linux.
func outsideTable(fd int, at *fdLineage) (outside, gone bool) {
	foreignFds.Lock()
	c, ok := foreignFds.claims[fd]
	foreignFds.Unlock()
	if betweenClaimAndLook != nil {
		betweenClaimAndLook(fd)
	}
	dev, ino, open := fdIdentity(fd)
	if !open {
		return false, true
	}
	if !ok {
		return false, false
	}
	if c.identified && (dev != c.dev || ino != c.ino) {
		foreignFds.Lock()
		if cur, still := foreignFds.claims[fd]; still && cur.stamp == c.stamp {
			delete(foreignFds.claims, fd)
		}
		foreignFds.Unlock()
		return false, false
	}
	return !at.holds(c.owner, c.stamp), false
}

// betweenClaimAndLook is called between outsideTable's reading of a claim
// and its look at the descriptor, and is nil outside a test: the moment the
// order matters, which only a test can hold open.
var betweenClaimAndLook func(fd int)

// startLeftoverWindow is how many of the lowest free numbers a start is
// watched on. A start holds at most one pipe end per stream, the pipe it
// hears the exec through and a pidfd at once — nine numbers — and a
// concurrent start's own can be among them, so the window is wider than
// that.
const startLeftoverWindow = 16

// claimStartLeftovers runs start and claims, as nobody's, what it left open
// on the low numbers.
//
// The numbers are looked at before the start and again after it, and what
// is claimed is what is new: a number that was free and is open, or one whose
// descriptor is not the one that was there. The second is not a tidiness. A
// sibling's substitution can be let go of while the start runs — closes are
// not in the window — and the start's pipe then lands on the number it gave
// up: measured on Linux, about one run in eight of `cat <(echo <(true))
// <(echo <(true))` in a test's buffered streams, before this looked at the
// numbers that had been open as well as the ones that had been free.
//
// A concurrent start's leftovers can be among what is new and are claimed
// too, which is right: they are the same kind of descriptor. So can a
// concurrent goroutine's own open — a redirection's file in a sibling body —
// and that is right as well, because a redirection's descriptor is never on
// the number the script sees it at (see childFiles), so its real number is in
// nobody's table. What cannot be among them is a substitution's end: a park
// holds transientFds for writing, so none happens while a start holds it.
//
// Only below kernelFirstTable: above it no dialect's wish list reaches far
// enough for a leftover to land in its way.
//
// cmd is the command being started, or nil where the caller opens something
// of its own in the window and wants it claimed too. A start that will leave
// nothing is not watched at all — every stream a file, and no pidfd on this
// platform — which is the common case on darwin and costs it nothing; the
// watch is a few dozen fcntl and fstat calls, a few microseconds against a
// start's several hundred.
func claimStartLeftovers(cmd *exec.Cmd, start func() error) error {
	if cmd != nil && !startLeavesDescriptors(cmd) {
		return start()
	}
	type seen struct {
		fd       int
		open     bool
		dev, ino uint64
	}
	var before [kernelFirstTable]seen
	n, free := 0, 0
	for fd := 3; fd < kernelFirstTable && free < startLeftoverWindow; fd++ {
		dev, ino, open := fdIdentity(fd)
		before[n] = seen{fd: fd, open: open, dev: dev, ino: ino}
		n++
		if !open {
			free++
		}
	}
	if err := start(); err != nil {
		return err
	}
	for _, b := range before[:n] {
		dev, ino, open := fdIdentity(b.fd)
		if open && (!b.open || dev != b.dev || ino != b.ino) {
			claimFd(b.fd, nil, true)
		}
	}
	return nil
}

// startLeavesDescriptors is whether starting cmd leaves a descriptor in this
// process for the child's life: a pidfd, which os/exec keeps on Linux only,
// or a pipe for a stream that is not a file.
func startLeavesDescriptors(cmd *exec.Cmd) bool {
	if runtime.GOOS == "linux" {
		return true
	}
	return !fileOrNothing(cmd.Stdin) || !fileOrNothing(cmd.Stdout) || !fileOrNothing(cmd.Stderr)
}

// fileOrNothing is whether os/exec hands v to a child without a pipe of its
// own: nothing, which it opens the null device for and closes, or a file.
func fileOrNothing(v any) bool {
	if v == nil {
		return true
	}
	_, ok := v.(*os.File)
	return ok
}
