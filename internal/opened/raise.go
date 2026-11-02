// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"io/fs"
	"sync"
	"syscall"
)

// Raise is what a caller that publishes descriptor numbers gives the walk, so
// that nothing running beside an open ever finds one of the open's descriptors
// on a low number.
//
// interp is that caller. A process substitution's body runs beside the command
// that named it, in the same descriptor table, and it numbers its own
// substitutions by asking the kernel which low numbers are free. A forking
// shell's body never holds what its parent opens after the fork, so every
// descriptor the parent makes in the meantime is in the body's way here and in
// nobody's there. The walk makes one per directory it passes through and one
// for the file, each on the lowest free number: measured on darwin with a
// Gate that allows everything, `cat <(echo <(true) <(true) <(true)) > out`
// published a number past one that was free in 90 runs of 300, as far as
// eleven past it, where a run with no Gate published none (#7341).
//
// So each descriptor is moved by Up the moment it is made, and the open and
// the move are made inside Hold, which is what a body's park waits on. Outside
// the hold, the walk's descriptors are out of the way already.
//
// **Nothing that can wait is done inside the hold**, because every start in
// the program waits on a park and a park on the hold. Two things could wait,
// and neither is in it:
//
//   - The caller's check. It is a Gate's policy in interp — an embedder's own
//     code — and it is asked between holds, never inside one: before a
//     creation, with the walk's directories already moved, and after the
//     open, with the file already moved.
//
//   - An open that waits for a peer, which is a FIFO's. The final component is
//     looked at from the directory the walk holds before it is opened, and a
//     name whose open could wait — a FIFO, a device — is opened outside the
//     hold and moved after, exactly as an open with no Gate is. Every open
//     made inside the hold is non-blocking besides, which answers the name
//     changing between the look and the open: a FIFO put there in between is
//     opened at once for reading and refused at once with ENXIO for writing,
//     and either way is opened again outside, with the first closed only after
//     the second, so that a writer the first let through never meets a pipe
//     with no reader.
//
// The zero Raise moves nothing and holds nothing, which is what every caller
// but one passes.
type Raise struct {
	// Hold is taken from each open the walk makes to the move of what it
	// made, and at no other time. Nil makes the opens and moves without it.
	Hold sync.Locker
	// Up answers a duplicate of fd out of the way, or false where there is no
	// room. The walk closes fd once it has the duplicate.
	Up func(fd int) (int, bool)
}

func (r Raise) on() bool { return r.Up != nil }

// held makes one open inside the hold and moves what it made before letting
// go of it.
//
// name is the path the walk will report for the descriptor, which is what a
// test's hold is matched against. file says the descriptor is the one handed
// back to the caller rather than a directory the walk only passes through:
// such a descriptor is asked what it reached, and anything whose open could
// have waited is reported as waits, moved, for the caller to open again
// outside the hold and close after. waits with no descriptor is the
// non-blocking open's own refusal of a FIFO with no reader.
func (w *walk) held(name string, file bool, open func(nonblock int) (int, error)) (fd int, waits bool, err error) {
	if w.raise.Hold != nil {
		w.raise.Hold.Lock()
		defer w.raise.Hold.Unlock()
	}
	fd, err = open(syscall.O_NONBLOCK)
	if err != nil {
		if file && (err == syscall.ENXIO || err == syscall.EAGAIN) {
			return -1, true, nil
		}
		return -1, false, err
	}
	if file {
		var st syscall.Stat_t
		if serr := syscall.Fstat(fd, &st); serr != nil || !cannotWait(uint32(st.Mode)) {
			waits = true
		} else {
			// The caller asked for a blocking descriptor and a regular file
			// or a directory ignored the flag at the open; it comes off the
			// description before the file is handed on. Not from a directory
			// the walk passes through, which is never handed on, and which on
			// Linux is O_PATH and refuses F_SETFL.
			_ = syscall.SetNonblock(fd, false)
		}
	}
	if whileLow != nil {
		whileLow(name, fd)
	}
	return w.lift(fd), waits, nil
}

// lift moves fd out of the way with the caller's Up, closing it once the
// duplicate is in hand, and answers the number to use from now on.
func (w *walk) lift(fd int) int {
	moved, ok := w.raise.Up(fd)
	if !ok {
		return fd
	}
	_ = syscall.Close(fd)
	return moved
}

// openRoot is the package's openRoot, made inside the hold where there is a
// Raise. `/` is a directory, whose open waits for nothing.
func (w *walk) openRoot(flags int) (int, error) {
	if !w.raise.on() {
		return openRoot(flags)
	}
	fd, _, err := w.held("/", false, func(nonblock int) (int, error) { return openRoot(flags | nonblock) })
	return fd, err
}

// openThrough opens a directory the walk is passing through. Opened with
// O_DIRECTORY, so a FIFO on the name is refused with ENOTDIR at once rather
// than waited on — measured on Darwin 25.5.0 for both the traverse open and
// the readable one — and the hold's non-blocking open is the belt to that.
func (w *walk) openThrough(comp string, flags int, perm uint32) (int, error) {
	dirfd := w.top()
	if !w.raise.on() {
		return openat(dirfd, comp, flags, perm)
	}
	fd, _, err := w.held(w.wouldReach(comp), false, func(nonblock int) (int, error) {
		return openat(dirfd, comp, flags|nonblock, perm)
	})
	return fd, err
}

// openFinal opens the component the caller asked for, from the directory the
// walk stands in: a file, a directory at the end of the path, or the probe
// and the creation createChecked makes. With a Raise, the name is looked at
// first, and one whose open can wait is opened outside the hold.
func (w *walk) openFinal(comp string, flags int, perm uint32) (int, error) {
	dirfd := w.top()
	if !w.raise.on() {
		return openat(dirfd, comp, flags, perm)
	}
	if !mayWait(dirfd, comp) {
		if betweenTheLookAndTheOpen != nil {
			betweenTheLookAndTheOpen(w.wouldReach(comp))
		}
		first, waits, err := w.held(w.wouldReach(comp), true, func(nonblock int) (int, error) {
			return openat(dirfd, comp, flags|nonblock, perm)
		})
		if !waits {
			return first, err
		}
		if first >= 0 {
			// Something other than what was looked at is on the name now,
			// and it was opened without waiting. Closed only after the open
			// that may wait, below.
			defer func() { _ = syscall.Close(first) }()
		}
	}
	fd, err := openat(dirfd, comp, flags, perm)
	if err != nil {
		return -1, err
	}
	return w.lift(fd), nil
}

// mayWait reports whether the object on name, from the directory dirfd holds,
// is of a kind whose open can wait: anything but a regular file, a directory
// or a symbolic link — which the walk's O_NOFOLLOW refuses at once and then
// follows by name — and a name with nothing on it, which a creation makes a
// regular file of.
//
// It decides only which way the open is made. Where the platform cannot ask,
// every name is taken to be one that cannot wait, and the non-blocking open
// and the look at what it reached are what keep that true.
func mayWait(dirfd int, name string) bool {
	if !statAtSupported {
		return false
	}
	info, err := retrying(func() (fs.FileInfo, error) { return statAtOnce(dirfd, name, false) })
	if err != nil {
		return false
	}
	m := info.Mode()
	return !m.IsRegular() && !m.IsDir() && m&fs.ModeSymlink == 0
}

// cannotWait reports whether a stat's file type is one whose open never waits
// for a peer: a regular file or a directory.
func cannotWait(mode uint32) bool {
	switch mode & syscall.S_IFMT {
	case syscall.S_IFREG, syscall.S_IFDIR:
		return true
	}
	return false
}

// whileLow and betweenTheLookAndTheOpen are nil outside a test: the window
// between a held open and its move, which only a test can hold open, and the
// moment the final component's name can change under the look that chose how
// to open it.
var (
	whileLow                 func(name string, fd int)
	betweenTheLookAndTheOpen func(name string)
)

// WhileLowForTest has f called inside the hold with the name and number of
// each descriptor a raised walk makes, between its open and its move, until
// restore is called. For interp's tests, which hold that window open to show
// that nothing beside it gets in.
func WhileLowForTest(f func(name string, fd int)) (restore func()) {
	whileLow = f
	return func() { whileLow = nil }
}

// BetweenTheLookAndTheOpenForTest has f called with the name between a raised
// walk's look at its final component and the open the look chose, until
// restore is called. For interp's tests, which change what is on the name
// there.
func BetweenTheLookAndTheOpenForTest(f func(name string)) (restore func()) {
	betweenTheLookAndTheOpen = f
	return func() { betweenTheLookAndTheOpen = nil }
}
