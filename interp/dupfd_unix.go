// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"syscall"

	"github.com/blairham/sh/internal/opened"
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
		if got, ok := intoTheBand(int(fd)); ok {
			moved = got
		}
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

// intoTheBand answers a close-on-exec duplicate of fd in privateCopyFloor's
// band, or false where the band is full or the kernel refused. The original is
// the caller's to close: raiseScriptFile's is an *os.File's, and a Gate's
// walk closes its own (see openGatedRaised).
func intoTheBand(fd int) (int, bool) {
	got, err := fcntlInt(fd, syscall.F_DUPFD_CLOEXEC, privateCopyFloor)
	if err != nil {
		return -1, false
	}
	if got >= kernelFirstTable {
		// The band is full, and a copy above it would grow the kernel's
		// table. See privateCopyFloor.
		_ = syscall.Close(got)
		return -1, false
	}
	return got, true
}

// openRegularRaised opens a path that names a regular file, or nothing yet,
// inside transientFds from the open to the move, as openRaised opens a
// command's own substitution pipe and for the same reason: a file the parent
// has just opened is on the lowest free number until raiseScriptFile moves it,
// and a park in a body already running found it there and stepped over it.
// Measured on darwin with three substitutions in one body beside a parent's
// `> "$tmp/out"`, everything below the base taken: `10 12 11` or `10 12 13`
// in 70 runs of 300, and with a 10 ms hold between the open and the move, in
// 20 of 20; with this, none (#7335).
//
// done says the open was made here, and false leaves it to the caller's
// ordinary road, outside the lock.
//
// **The lock is never held across an open that can wait**, and two things
// keep that true. The path is asked about first, and only a regular file, or
// a name with nothing on it for O_CREAT to make one, is opened here: a FIFO, a
// device and a directory go the ordinary road exactly as before. And the open
// is made non-blocking, which is what answers the name changing between the
// question and the open — a FIFO put there in between is opened at once for
// reading, and refused at once with ENXIO for writing where it has no reader,
// rather than waiting with the lock held for a peer that may be a process
// waiting on the lock to start. Whatever the open reached is asked about
// again, and anything but a regular file is opened a second time the
// ordinary way, outside, with the first closed only after: a writer the first
// let through never meets a pipe with no reader. A regular file ignores the
// flag at the open, and it is cleared from the file before it is handed on.
//
// Asked with a stat of the path's own, not through Runner.stat: this is the
// shell choosing how to open, not a probe the script wrote, and the open that
// follows is recorded as what it is. A runner with a Gate opens through the
// Gate's walk instead, which looks and holds a descriptor at a time rather
// than across the whole open — see openGatedRaised. Not with `set -C`, for
// openRaised's reason; and not in a dialect without process substitution,
// where nothing parks.
func (r *Runner) openRegularRaised(ctx context.Context, a *Action, path string, flags int) (*os.File, bool, error) {
	if flags&os.O_EXCL != 0 || !r.lang().ProcessSubstitution {
		return nil, false, nil
	}
	if r.Gate != nil {
		return r.openGatedRaised(ctx, a, path, flags)
	}
	st, err := opened.AtHeld(path, os.Stat)
	if err == nil && !st.Mode().IsRegular() || err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if betweenAFilesStatAndOpen != nil {
		betweenAFilesStatAndOpen(path)
	}
	f, regular, err := r.openRegularHeld(ctx, a, path, flags)
	if regular {
		return f, true, err
	}
	if err != nil {
		if errors.Is(err, syscall.ENXIO) || errors.Is(err, syscall.EAGAIN) {
			// The answer the flag gave rather than the file: a FIFO with no
			// reader. Asked again the ordinary way, which waits for one.
			return nil, false, nil
		}
		return nil, true, err
	}
	// Something other than a regular file is on the name now.
	defer func() { _ = f.Close() }()
	again, err := r.openGated(ctx, a, path, flags)
	if err != nil {
		return nil, true, err
	}
	return r.raiseScriptFile(again), true, nil
}

// openRegularHeld is openRegularRaised's open and move, inside the lock.
// regular is false, with the file, where what the open reached is not a
// regular file, and false with the error where the open failed.
func (r *Runner) openRegularHeld(ctx context.Context, a *Action, path string, flags int) (*os.File, bool, error) {
	transientFds.RLock()
	defer transientFds.RUnlock()
	f, err := r.openGated(ctx, a, path, flags|syscall.O_NONBLOCK)
	if err != nil {
		return nil, false, err
	}
	if st, serr := f.Stat(); serr != nil || !st.Mode().IsRegular() {
		return f, false, nil
	}
	if conn, cerr := f.SyscallConn(); cerr == nil {
		_ = conn.Control(func(fd uintptr) { _ = syscall.SetNonblock(int(fd), false) })
	}
	if whileAFileIsLow != nil {
		whileAFileIsLow(path, f)
	}
	return r.raiseScriptFile(f), true, nil
}

// openGatedRaised is openRegularRaised for a runner with a Gate, where the
// open is a walk: a descriptor for every directory on the way, each on the
// lowest free number, then the file. A body's park found them there and
// stepped over them, as it stepped over the file openRegularRaised opens —
// `10 21 11` where `10 11 12` was free, as far as eleven numbers past, in 90
// runs of 300 on darwin with a Gate that allows everything (#7341).
//
// The lock openRegularRaised holds from the open to the move cannot be held
// here, because the Gate's policy is asked from inside the walk — before a
// creation, and again after the open when the name led elsewhere — and a
// policy is an embedder's code, which must never run inside something every
// start waits on. So the walk takes the lock a descriptor at a time instead:
// each directory and the file are opened and moved into the band inside it,
// and the policy is asked between, when nothing of the walk's is low. The
// opens made inside are non-blocking, and the file is looked at from the
// directory the walk holds before it is opened, so a FIFO or a device is
// opened outside the lock as it is with no Gate, and one swapped in after the
// look is refused or opened at once rather than waited on. See opened.Raise.
//
// Always done here, so a Gate's open never takes the ordinary road: what the
// walk could not move — the fallback for a link it cannot follow — is moved
// after, as the ordinary road moves everything.
func (r *Runner) openGatedRaised(ctx context.Context, a *Action, path string, flags int) (*os.File, bool, error) {
	f, raised, err := r.openGatedRaising(ctx, a, path, flags, opened.Raise{
		Hold: transientFds.RLocker(),
		Up:   intoTheBand,
	})
	if err != nil {
		return nil, true, err
	}
	if !raised {
		f = r.raiseScriptFile(f)
	}
	return f, true, nil
}

// betweenAFilesStatAndOpen and whileAFileIsLow are nil outside a test: the
// moment the name can change under openRegularRaised, and the window between
// its open and its move, which only a test can hold open.
var (
	betweenAFilesStatAndOpen func(path string)
	whileAFileIsLow          func(path string, f *os.File)
)
