// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver

import (
	"os"
	"os/signal"
	"syscall"
)

// The Go runtime holds descriptors of its own, and a script can name the
// numbers they landed on.
//
// That is the whole of #695, #731 and #799, which are three reports of one
// fault. `exec 5>f; exec cmd` means the replacement has to find that file on
// descriptor 5, so placing the table is a dup2 onto whatever 5 was — and 5 is
// commonly where this process's own startup left the netpoller's epoll
// descriptor. What happens next depends on which structure was overwritten and
// on the platform:
//
//	runtime: epollwait on fd 5 failed with 22    (Linux, the poller)
//	fatal error: netpoll failed
//
//	fatal error: signal_recv: inconsistent state (darwin, the signal pipe)
//
// Both are *fatal* rather than reportable: the shell dies where it was about
// to become the command, and the command never runs.
//
// # Why the exec side could not close it
//
// Two rounds of work went into the window between the placement and the
// execve, and replace.go records them: the collector is stopped over it, and
// everything the exec allocates is built before the first descriptor is
// overwritten, so an strace of the real path is the dup3 and the execve
// adjacent. It was not enough, and could not be. execve on a multithreaded
// process kills the other threads *inside* the call, and until it has they are
// running against a table that has already changed — `sysmon` among them,
// polling on a timer of its own. No ordering written in Go shortens that.
//
// And on darwin there is no window at all. The goroutine reading the signal
// pipe is already blocked on the descriptor when it is overwritten, so it
// notices immediately: one `trap` line ahead of an `exec` killed the shell 25
// times out of 25, measured on main.
//
// # What is closed instead
//
// The collision, rather than the window. It is not luck — this shell's own
// startup arranges it. Opening the script leaves the low numbers briefly free
// and the runtime lands immediately after, which is exactly the range a script
// parks in:
//
//	openat("s.sh", O_RDONLY|O_CLOEXEC)    = 4
//	epoll_create1(EPOLL_CLOEXEC)          = 5
//	eventfd2(0, EFD_CLOEXEC|EFD_NONBLOCK) = 6
//
// So the low numbers are held before the runtime has taken any of them, the
// runtime is made to open everything it opens lazily — which then lands above
// them — and they are given straight back. The hold is over before main runs
// and costs nothing afterwards: what is left behind is the runtime's
// descriptors sitting at 100 and up, where nothing a script writes can reach
// them.
//
// This is #731's option 2, and it answers #799 in the same move, which is why
// it is one change. Every other direction addressed one platform's structure
// and would have been re-found on the other.
//
// # Why an init and not a call from Main
//
// The reservation has to happen before the runtime opens anything, and the
// runtime opens the poller the first time any pipe, socket or terminal is
// registered — which a front end may do long before it reaches Main, and which
// a test binary does before it reaches the first test. Package initialization
// is the earliest point a library can act, and it is early enough: measured,
// no descriptor of the runtime's exists yet when it runs.
//
// It also puts the fix where the fault is. driver is the only package that
// replaces this process — interp is a library and exposes the decision as a
// hook rather than making it — so a program that can hit this is exactly a
// program that imports driver, embedders included. #731 recorded that option 2
// "does not help an embedder"; done here rather than in a main it does.

// lowDescriptorCeiling is 99 because that is where a hand-written descriptor
// number stops, measured three ways:
//
//   - The panel can only *name* single digits. `exec 20>/dev/null` is a
//     redirection in bash alone; dash, ksh93 and zsh all read the number as a
//     command word and answer "20: not found". So 0 through 9 is the whole of
//     what a portable script can ask for, and the rest of the range is
//     headroom against bash.
//   - 424 shell scripts shipped on this machine name descriptors 0 through 5
//     and nothing above.
//   - The numbers above 9 that a script does use are ones it did not choose:
//     `exec {v}>f` allocates 10 in bash and ksh93 and 11 in zsh, and an
//     allocated number cannot collide with the runtime by construction —
//     the kernel hands out a number precisely because nothing holds it.
//
// The cost is the hold itself, which is one fcntl per free number and is paid
// once: measured at 50µs on darwin and 37µs on Linux against a 3ms shell
// startup. A ceiling of 255 would cost twice that and buy nothing any
// measurement here can see.
//
// What is left over is written down rather than claimed away: a bash script
// that names a descriptor above the ceiling by hand can still land on one of
// the runtime's, and nothing in this package can move a number the runtime
// holds. docs/design.md records the limit beside the process model.
const lowDescriptorCeiling = 99

func init() { keepTheRuntimeOffTheLowDescriptors() }

// keepTheRuntimeOffTheLowDescriptors makes the runtime take its descriptors
// from above the range a script can name.
//
// All or nothing, and the failure path is why: a partial hold is a process
// that has consumed most of its open-file limit and *then* asks the runtime to
// open a poller, and a poller that cannot be opened is `throw` rather than an
// error. A shell that could not reserve the range is one that keeps the bug it
// had, which is strictly better than one that cannot start.
func keepTheRuntimeOffTheLowDescriptors() {
	held, complete := holdLowDescriptors(lowDescriptorCeiling)
	if complete {
		before := openInBand()
		openRuntimeDescriptors()
		runtimeDescriptors = appearedInBand(before)
	}
	releaseDescriptors(held)
}

// runtimeDescriptors are the numbers the Go runtime took while the low range
// was held, and the numbers placeFiles will not write over.
//
// The ceiling moves the runtime out of reach; this is what happens if a script
// reaches anyway. A bash script may name any descriptor it likes — `exec
// 100>f` is a redirection there and a command word everywhere else — so the
// numbers immediately above the ceiling are still nameable by hand, and
// measured: with the ceiling in place, `exec 100>f; exec cmd` on darwin lands
// on the runtime's own signal pipe and kills the shell exactly as descriptor 3
// did before it.
//
// Knowing the numbers is what turns that from a death into a missing
// descriptor. The command runs and finds that one number closed, which is what
// placeFiles already does with every descriptor it could not place — and the
// alternative is not "the command gets its file", it is "the shell dies before
// the command runs".
//
// Empty where the hold could not be completed, which is honest rather than
// convenient: nothing was forced, so the runtime has not opened its
// descriptors yet and there is nothing to know. Such a shell has the bug it
// had before this file existed.
//
// A slice rather than a map because it has three entries at most — a poller
// and an eventfd on Linux, a kqueue and a signal pipe on darwin — and is
// scanned once per `exec cmd`.
var runtimeDescriptors []int

// heldByTheRuntime reports whether a descriptor number is one the runtime
// answered for at initialization.
func heldByTheRuntime(fd int) bool {
	for _, held := range runtimeDescriptors {
		if fd == held {
			return true
		}
	}
	return false
}

// runtimeDescriptorBand is how far above the ceiling the census looks.
//
// The runtime takes three descriptors at most and takes them from the lowest
// free number, which is the ceiling's neighbor while the range below is held —
// so a band of sixteen is five times what any measurement has produced and
// still sixteen fcntl calls rather than a scan of the process's open-file
// limit.
const runtimeDescriptorBand = 16

// openInBand lists which numbers just above the ceiling are open.
//
// Taken before and after the runtime is made to open its own, because the
// difference is the answer: a descriptor already there is one the *caller*
// handed this process, and a shell that mistook one of those for the runtime's
// would refuse to place a descriptor a script is entitled to.
func openInBand() []int {
	var open []int
	for fd := lowDescriptorCeiling + 1; fd <= lowDescriptorCeiling+runtimeDescriptorBand; fd++ {
		if _, err := fcntl(fd, syscall.F_GETFD, 0); err == nil {
			open = append(open, fd)
		}
	}
	return open
}

// appearedInBand is openInBand again, minus what was already there.
func appearedInBand(before []int) []int {
	var appeared []int
	for _, fd := range openInBand() {
		found := false
		for _, was := range before {
			if was == fd {
				found = true
				break
			}
		}
		if !found {
			appeared = append(appeared, fd)
		}
	}
	return appeared
}

// holdLowDescriptors occupies every free number up to the ceiling, reporting
// whether it reached it.
//
// One file is opened and duplicated rather than opening one per number: a dup
// is a number and a path is a lookup, and the difference is measured at 4.7µs
// against 0.35µs per descriptor on darwin. F_DUPFD_CLOEXEC is also the only
// shape of this that is safe — dup2 onto a chosen number would silently
// *close* whatever was there, and what is there on a low number may be a
// descriptor the caller handed this process, which inheritedfds is about to
// publish to the script. The kernel choosing the lowest free number cannot
// take anything away.
//
// Close-on-exec on every one of them: they are gone before main, but a program
// that forked in the middle of a package initializer would otherwise hand a
// child a hundred copies of /dev/null.
func holdLowDescriptors(ceiling int) (held []int, complete bool) {
	fd, err := syscall.Open(os.DevNull, syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, false
	}
	held = append(held, fd)
	for fd < ceiling {
		if fd, err = fcntl(held[0], syscall.F_DUPFD_CLOEXEC, firstExtraFd); err != nil {
			// Out of descriptors, which is the one case worth backing out of
			// rather than pressing on with.
			return held, false
		}
		held = append(held, fd)
	}
	return held, true
}

// releaseDescriptors gives the held numbers back.
//
// Errors are dropped: every one of these is a number this function opened, and
// a close that fails leaves a descriptor held for the life of the process,
// which is the same outcome as not trying.
func releaseDescriptors(held []int) {
	for _, fd := range held {
		_ = syscall.Close(fd)
	}
}

// openRuntimeDescriptors makes the Go runtime open every descriptor it would
// otherwise open lazily, so that it opens them here — above the numbers being
// held — instead of later, below them.
//
// There are exactly two, measured on both platforms by taking a census of the
// process's open descriptors around every operation a shell performs: sleeping
// and waking a timer, reading random bytes, running a child, opening a file,
// starting goroutines. Nothing else in the runtime opens anything that
// outlives the call.
//
//   - **The netpoller.** Registering any pipe, socket or terminal creates it,
//     once for the process, and it is never closed. A pipe made and closed
//     here leaves epoll_create1 and its eventfd behind on Linux and a kqueue
//     on darwin — which is the point.
//   - **os/signal's delivery.** On darwin the runtime cannot hand a signal to
//     a channel from a handler, so it carries the arrival over a *pipe*, two
//     descriptors, opened the first time anything asks os/signal for anything
//     and never closed. That is #799 exactly: a shell with a trap has one, and
//     `exec 3>f` writes over it. Linux opens nothing here — its notes are
//     futexes — and the call is made anyway, both because a future runtime
//     may change that and because the platforms in between cannot be measured
//     from here.
//
// SIGWINCH is the signal asked for because it is the one whose default action
// is to be ignored: the disposition is put back immediately, and a signal
// arriving inside those few microseconds is one the process would have thrown
// away regardless. The channel is dropped rather than drained for the same
// reason — nothing was going to read it.
func openRuntimeDescriptors() {
	if r, w, err := os.Pipe(); err == nil {
		_ = r.Close()
		_ = w.Close()
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	signal.Stop(ch)
}
