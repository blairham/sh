// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import (
	"os"
	"slices"
	"strconv"
	"sync"
	"syscall"
)

// A substitution's pipe, and the descriptor number its path is made of.
//
// `<(cmd)` and `>(cmd)` expand to a path, and what the path has to be is the
// only thing a shell here has any choice about. Every shell in the panel that
// has the construct hands the command `/dev/fd/N` — measured 2026-09-15, bash
// 5.3, ksh93u+ and zsh 5.9.2 all do, on all three invocation routes — and this
// one handed over a named pipe in a directory of its own under `$TMPDIR`
// instead (#2893). Both are pipes and both read correctly, so what differed
// was the name: a path that leaks the temporary directory into anything that
// echoes its arguments, does not repeat between runs, exists in the filesystem
// for something to have to remove, and answers a second `open` differently.
//
// # Why /dev/fd works here and did not before
//
// The rejection this replaces was real and is worth stating exactly, because
// the reasoning was right about the mechanism it had in mind. A `/dev/fd/N`
// path is only openable by a process that holds N, so the descriptor has to
// reach the command — and everything Go opens is close-on-exec, so the way to
// make it reach anything looked like clearing that flag. Clearing it hands the
// descriptor to **every** command the shell runs afterwards, and for `>(cmd)`
// that is fatal rather than untidy: a later command holding the writing end
// open means the body never reads end-of-file, so
//
//	echo x | tee >(tr a-z A-Z); sleep 0.4
//
// produced nothing at all, because `sleep` was holding the pipe. A C shell
// clears the flag around one fork and sets it again; Go's os/exec takes the
// fork lock itself, so there is no window a caller can hold.
//
// The flag never had to be cleared. A descriptor reaches a child **by number**
// through the table childFiles rebuilds — that is what `exec 3>out3; cmd`
// already runs on, and it places a file on a chosen number in that child and
// in no other. So the end the command opens is parked on a number here, stays
// close-on-exec, and is put into that table for the commands of the one shell
// that named it. Nothing else inherits it, which is stronger than the C shells
// manage: a substitution's own body, and every command the body runs, is a
// clone whose procSubs list is empty, so `tee >(cat)` cannot hand the writing
// end to the `cat` that is reading the other side of it.
//
// # And it is a pipe rather than a FIFO
//
// Everything a named pipe needed and an anonymous one does not goes with it.
// A FIFO's two ends have to *meet*: opening one blocks until the other is
// opened, which for a path the command may never open at all is a wait that
// may never end, and the pipe itself exists only while somebody holds it —
// so a shell that opened, wrote and closed inside the window a reader was
// still in `open(2)` ran a whole pipe's life cycle beside a reader attached
// to none of it (#2733), and a last-writer close could be delivered to
// nobody (#1079). An `os.Pipe` has both ends from the moment it is made and
// cannot be torn down while either is held, so there is no rendezvous, no
// bounded poll for a peer, no placeholder end to keep the pipe alive and no
// close to repeat.
//
// What replaces all of it is one descriptor: the end the command opens, held
// by the shell from the moment the word expands until removeProcSubs, which
// is the same lifetime the FIFO's name had. It keeps the pipe alive while the
// command may still open the path, and closing it is what delivers
// end-of-file to a `>(cmd)`'s body and EPIPE to a `<(cmd)`'s.

// procSubEnds is the two halves of one substitution's pipe.
//
// shell is the end this shell reads or writes through — the body's output for
// `<(cmd)`, the body's input for `>(cmd)`. child is the other end, parked on a
// descriptor number.
//
// fd is the number path is made of, and it is *published* rather than raw:
// the number the child's table is built at, which is child's own number in
// every shape but one. See substFdView for the shape where the two part
// company, and real for the path that names the raw one.
type procSubEnds struct {
	shell *os.File
	child *os.File
	fd    int
	path  string
	// real names child's actual descriptor where that is not fd. Empty when
	// the two agree, which is every substitution outside another one's body.
	real string
}

// substFdView is how the shell making a substitution sees the descriptor
// table, in the two places the kernel cannot answer for it.
//
// released are numbers this process holds that a *fork* would have closed:
// the parked ends of the substitutions this body is running inside. bash runs
// a substitution's body in a fork and closes the outer end there, so the
// number comes free and a nested substitution takes it — `cat <(echo
// <(true))` is `/dev/fd/63` twice over in bash 5.3.20, the outer's number
// re-used inside. A body here is a clone rather than a fork and the outer end
// is still open in the one descriptor table there is, so asking the kernel
// answers about a descriptor the body is not supposed to be able to see.
//
// taken are numbers already published in this view whose descriptor is *not*
// on them — the other half of the same split. Once a nested substitution has
// published a number it borrowed from released, the kernel will hand that
// number out again, and the next substitution in the same body would publish
// it a second time.
//
// Both are short — one entry per enclosing substitution — so they are walked
// rather than hashed.
type substFdView struct {
	released []int
	taken    []int
}

func (v substFdView) isReleased(fd int) bool { return slices.Contains(v.released, fd) }
func (v substFdView) isTaken(fd int) bool    { return slices.Contains(v.taken, fd) }

// firstProcSubFd is the floor the search falls back to when every number the
// dialect asked for is taken.
//
// Above the nine a script names by hand, which is not tidiness: the number is
// where the table childFiles builds will place the descriptor in the command,
// and a script's own `exec 3>out` is an entry in that same table. Two things
// on one number is one of them lost, and which one would depend on map
// iteration order.
//
// It used to be where the search *began*, in every dialect, which is what made
// every process substitution in this shell `/dev/fd/10` whatever it was
// imitating. Where it begins is the dialect's now — see
// Semantics.SubstitutionEndPlacement and Runner.substEndCandidates — and one
// of the four answers, ksh93's, is deliberately *below* this number. That is
// not a hole in the reasoning above: substEndCandidates skips what the
// runner's own table holds, so the collision this constant was chosen to
// avoid is ruled out by the wish list rather than by the floor.
const firstProcSubFd = 10

// devFdDir and procFdDir are the two directories a substitution's path can be
// named after. Both are the process's own descriptor table under another
// name, and where both exist the first is a symlink to the second.
const (
	devFdDir  = "/dev/fd"
	procFdDir = "/proc/self/fd"
)

// procFdDirExists is whether /proc/self/fd is there on this machine.
//
// Asked once: it is a property of the kernel the process is running on, and a
// shell that stat'd it per substitution would be asking the same question
// thousands of times to get the same answer. A read of the filesystem rather
// than a build tag, because a Linux kernel without /proc mounted is a real
// configuration and GOOS cannot see it.
var procFdDirExists = sync.OnceValue(func() bool {
	info, err := os.Stat(procFdDir)
	return err == nil && info.IsDir()
})

// procSubFdDir is the directory this shell names its substitutions after.
//
// Semantics.SubstitutionPathPrefersProcSelfFd carries the measurement and the
// reason the directory is looked for rather than assumed. /dev/fd is the
// fallback every preset takes where /proc is not, which is every preset on
// this machine's own platform.
func (r *Runner) procSubFdDir() string {
	if r.sem().SubstitutionPathPrefersProcSelfFd == Yes && procFdDirExists() {
		return procFdDir
	}
	return devFdDir
}

// newProcSubPipe makes a substitution's pipe and parks the end the command
// will open.
//
// childWrites says which way round the ends go, and it is the only difference
// between the two spellings: `>(cmd)` hands the command the writing end and
// keeps the reading one, `<(cmd)` the other way about. dir is the directory
// the path is named after — see Runner.procSubFdDir.
func newProcSubPipe(childWrites bool, dir string, want []int, view substFdView) (procSubEnds, error) {
	rd, wr, err := os.Pipe()
	if err != nil {
		return procSubEnds{}, err
	}
	shell, child := wr, rd
	if childWrites {
		shell, child = rd, wr
	}
	// The shell's own end is moved out of the way first — see raiseShellEnd.
	// Before the park, because the number it gives up is one of the numbers
	// the park is about to ask for.
	shell = raiseShellEnd(shell, want)
	parked, pub, err := parkDescriptor(child, dir, want, view)
	// The original is closed either way: on success the parked duplicate is
	// the one the path names, and on failure there is nothing to hand over.
	_ = child.Close()
	if err != nil {
		_ = shell.Close()
		return procSubEnds{}, err
	}
	if pub == int(parked.Fd()) {
		// And now that both of this pipe's own originals are gone, the number
		// the dialect actually wanted may have come free — see
		// reparkPreferred. Only where the published number *is* the
		// descriptor's: a borrowed number was never the kernel's to give, so
		// there is nothing to ask it again about.
		parked = reparkPreferred(parked, dir, want, view)
		pub = int(parked.Fd())
	}
	ends := procSubEnds{shell: shell, child: parked, fd: pub, path: dir + "/" + strconv.Itoa(pub)}
	if raw := int(parked.Fd()); raw != pub {
		ends.real = dir + "/" + strconv.Itoa(raw)
	}
	return ends, nil
}

// raiseShellEnd moves this shell's end of the pipe above every number the
// dialect could publish, and closes the original.
//
// **The shell's half of the plumbing is not supposed to be in the answer.**
// `os.Pipe` takes the two lowest free numbers, so before this the shell's own
// end sat *inside* the region the published number comes from and the next
// substitution's number had to step over it. That is the whole of ksh93's
// `6 9 12` where the shell answers `3 4 5`: the rule was right and the floor
// was this shell's own pipes. Measured 2026-09-21 by dumping the table at each
// park — `3:P 4:P` on the first, `3:s 4:P 5:P 6:P 7:P` on the second — so each
// turn cost two or three numbers, which is exactly the gap.
//
// It is what ksh93 does too, and the reason it can answer `3 4 5` at all:
// a shell that keeps its own descriptors high leaves the low ones for the
// numbers it publishes.
//
// A best effort, and the failure is not one. Under a low `ulimit -n` the floor
// is past the limit and the duplicate is refused, which leaves the end where
// `os.Pipe` put it — the arrangement every release before this one shipped.
// A substitution that refused to run because its *private* descriptor could
// not be tidied would be a construct lost to a cosmetic.
func raiseShellEnd(f *os.File, want []int) *os.File {
	floor := shellEndFloor(want)
	if floor == 0 {
		return f
	}
	conn, err := f.SyscallConn()
	if err != nil {
		return f
	}
	moved := -1
	if cerr := conn.Control(func(fd uintptr) {
		// **Any answer at or above the floor will do**, which is the
		// difference from parkDescriptor and the reason these are two walks
		// rather than one. A published number has to be the number the
		// dialect asked for; a number nothing can name only has to be out of
		// the way, and F_DUPFD answers at or above what it was given. Taking
		// the exact number instead cost the third substitution its place —
		// the first two shell ends were already on the floor and its
		// neighbor, so the wish was refused and the end stayed low, and
		// ksh93's `3 4 5` read `3 4 6`.
		if got, err := fcntlInt(int(fd), syscall.F_DUPFD_CLOEXEC, floor); err == nil {
			moved = got
		}
	}); cerr != nil || moved < 0 {
		return f
	}
	// And the blocking mode is put back, for parkDescriptor's reason applied
	// to the other end of the same pipe. Go opens a pipe non-blocking and
	// runs it behind its own poller, and that mode belongs to the *os.File
	// Go made — a duplicate wrapped by os.NewFile is a file Go did not make,
	// so the mode stops being the poller's business and starts being the
	// description's. What reads this end is the substitution's body, which is
	// usually an external command handed the descriptor, and a command on a
	// non-blocking pipe meets EAGAIN on an empty one: measured, `cat` in
	// `>(cat)` answered `stdin: Resource temporarily unavailable`.
	if err := syscall.SetNonblock(moved, false); err != nil {
		_ = syscall.Close(moved)
		return f
	}
	raised := os.NewFile(uintptr(moved), f.Name())
	_ = f.Close()
	return raised
}

// reparkPreferred asks the wish list again, once, now that this pipe's own
// originals are closed.
//
// The park has to run while the original is still open — it is what is being
// duplicated — so the original is itself occupying one of the numbers being
// asked for, and on the dialects that allocate *upward* it is occupying the
// best one. `os.Pipe` hands out 3 and 4, ksh93's list starts at 3, and the
// first answer is therefore 5 however free 3 was a moment later. One more
// pass, after the close, is what turns that into 3.
//
// The list is walked in its own order and the walk stops at the number
// already held: the list is best-first, so reaching the current number
// without a wish landing means nothing better was free. And it is the *list*
// rather than a bare descent from a floor, which is what keeps this from
// reopening the collision firstProcSubFd was set to avoid —
// Runner.substEndCandidates has already taken out every number the script's
// own table holds.
//
// Failure is not one here either: the number in hand is already a working
// descriptor, and every refusal simply leaves it.
func reparkPreferred(parked *os.File, dir string, want []int, view substFdView) *os.File {
	cur := int(parked.Fd())
	for _, n := range want {
		if n == cur {
			return parked
		}
		if view.isTaken(n) {
			// Published already, by a substitution whose descriptor is
			// somewhere else. The kernel would hand this number over.
			continue
		}
		got, err := fcntlInt(cur, syscall.F_DUPFD_CLOEXEC, n)
		if err != nil {
			continue
		}
		if got != n {
			// Taken. A working duplicate at the wrong number is no use here —
			// the one in hand is already that — so it is closed rather than
			// kept.
			_ = syscall.Close(got)
			continue
		}
		// No SetNonblock: the duplicate shares the open file description with
		// the one parkDescriptor already cleared the mode on.
		_ = parked.Close()
		return os.NewFile(uintptr(got), dir+"/"+strconv.Itoa(got))
	}
	return parked
}

// parkDescriptor duplicates a file onto one of the numbers want asks for, and
// answers both the descriptor and the number to publish.
//
// The two are one number in every substitution that is not inside another
// one's body, and the second answer exists for the ones that are: a number
// view.released says a fork would have freed is published without being
// taken, and the descriptor goes above the published region instead. See
// substFdView.
//
// want is the dialect's ordered wish list — see Runner.substEndCandidates and
// Semantics.SubstitutionEndPlacement, which is where the four shells' four
// rules are recorded. It is a wish list rather than an instruction because
// only the kernel knows which numbers this process actually has free:
// F_DUPFD answers with the lowest free number *at or above* the one it is
// given, so a number that came back different from the one asked for is a
// number that was taken, and the duplicate is closed and the next wish tried.
// That is also what makes this safe where dup2 would not be — dup2 onto a
// taken number closes whatever was there, silently, and what was there could
// be the shell's own.
//
// The list ends with firstProcSubFd, which is not a wish but a floor: if
// every number the dialect wanted is taken, the substitution still needs a
// descriptor, and the lowest free one at or above the region a script names
// by hand is the answer every shell measured falls back to.
//
// F_DUPFD_CLOEXEC rather than a dup and a flag, because the pair is not atomic
// and a fork on another goroutine between them is a descriptor leaked into a
// child — the same reason driver's own descriptor moves use it. Close-on-exec
// stays *on*: what puts this in a command is the table childFiles builds, by
// number, and a descriptor that also leaked through the kernel behind that
// table's back would be open in every command the shell runs.
//
// The name is the path the command is going to be handed — `/dev/fd/N`, or
// `/proc/self/fd/N` in the dialect and on the platform that prefer it — and
// *os.File carries its name for whoever asks, which is what lets
// holdsDescriptorOnto recognize a descriptor the script has since taken onto
// the same pipe. Both spellings are the same number in the same table, so the
// choice reaches nothing but the string.
//
// # And the blocking mode is put back
//
// Go opens a pipe non-blocking and runs it behind its own poller, which is a
// fact about *this* process and travels further than it looks: on darwin
// `/dev/fd/N` is a duplicate rather than a fresh open, so it shares the open
// file description and its flags. A command handed a path whose description
// is non-blocking meets EAGAIN on an empty pipe and reports it as an error —
// `cat <(…)` answered `Resource temporarily unavailable` and `read < <(…)`
// read nothing at status 0, which is the quieter half of the same fault.
//
// Cleared on the duplicate, which is the end nothing in this process reads or
// writes through: the shell's own end is the pipe's *other* description and
// keeps the mode Go gave it, so the poller is untouched.
func parkDescriptor(f *os.File, dir string, want []int, view substFdView) (*os.File, int, error) {
	conn, err := f.SyscallConn()
	if err != nil {
		return nil, 0, err
	}
	// Where a borrowed number's descriptor goes: above everything this
	// dialect could publish, which is raiseShellEnd's floor and is chosen for
	// the same reason. A raw number nothing can name only has to be out of
	// the way, and leaving it inside the published region would put it in
	// front of the next substitution's wish.
	floor := shellEndFloor(want)
	if floor < firstProcSubFd {
		floor = firstProcSubFd
	}
	var parked, published int
	var parkErr error
	if cerr := conn.Control(func(fd uintptr) {
		for _, n := range want {
			if view.isTaken(n) {
				// Published already by a substitution of this same body,
				// whose descriptor is not on it. The kernel would say the
				// number is free and two paths would name one pipe.
				continue
			}
			if view.isReleased(n) {
				// A number a fork would have freed, so this shell publishes
				// it and parks the descriptor out of the way. The command
				// this substitution is handed to gets it *at* n through the
				// table childFiles builds, which is the only place the
				// number has to be true.
				got, err := fcntlInt(int(fd), syscall.F_DUPFD_CLOEXEC, floor)
				if err != nil {
					continue
				}
				parked, published = got, n
				return
			}
			got, err := fcntlInt(int(fd), syscall.F_DUPFD_CLOEXEC, n)
			if err != nil {
				// A wish the kernel refuses is a wish that missed, not the
				// end of the search. The refusal that matters is EMFILE from
				// asking high under a low `ulimit -n`: bash's own rule takes
				// it as a signal to stop reaching for the top of the table,
				// and a list that gave up here would turn what every shell
				// answers into `too many open files`.
				continue
			}
			if got == n {
				parked, published = got, n
				return
			}
			// The number was taken. What came back is a working duplicate at
			// the wrong number, so it is closed rather than kept: keeping it
			// would leak one descriptor per wish that missed.
			_ = syscall.Close(got)
		}
		parked, parkErr = fcntlInt(int(fd), syscall.F_DUPFD_CLOEXEC, firstProcSubFd)
		published = parked
	}); cerr != nil {
		return nil, 0, cerr
	}
	if parkErr != nil {
		return nil, 0, parkErr
	}
	if err := syscall.SetNonblock(parked, false); err != nil {
		_ = syscall.Close(parked)
		return nil, 0, err
	}
	return os.NewFile(uintptr(parked), dir+"/"+strconv.Itoa(parked)), published, nil
}

// fcntlInt is the one fcntl this package needs, with the errno turned into an
// error.
func fcntlInt(fd, cmd, arg int) (int, error) {
	n, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(cmd), uintptr(arg))
	if errno != 0 {
		return 0, errno
	}
	return int(n), nil
}
