// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"runtime"
	"runtime/debug"
)

// atPlacement is called on the way through a replacement, named for where it
// is standing: "before" with the table still untouched, "after" once it has
// been placed, and "execve" with the replacement one instruction away. Nil in
// a real shell.
//
// A seam rather than a test that waits for the window to bite: what goes wrong
// there is a garbage collection or a poll on a timer, and neither is something
// a test can ask for from the outside. Three points are offered because the
// window has two rules and each needs both of its ends —
//
//   - the collector is stopped before the first descriptor is overwritten and
//     is still stopped when the execve is reached, so a shell that stopped it
//     too late would satisfy one end and not the other;
//   - nothing allocates between "before" and "execve", which is what keeps
//     `sysmon` from finding a window long enough to poll in, and which only
//     shows as the *difference* between two counts.
//
// See replaceProcess for what the window is.
var atPlacement func(where string)

func reachedPlacement(where string) {
	if atPlacement != nil {
		atPlacement(where)
	}
}

// replaceProcess is `exec cmd`: this process stops being a shell and becomes
// the command.
//
// It lives in driver rather than in interp on purpose. interp is a library, and
// a Runner embedded in another program must not be able to replace that
// program with whatever a script named — so interp exposes the decision as a
// hook and provides no implementation. A binary that *is* a shell is the one
// place the call is correct, and this is that place.
//
// It returns only on failure. On success the image is gone and there is nothing
// left to return to, which is also why the caller reports an error from here as
// the exec having failed rather than as the command having exited badly.
//
// argv must include argv[0]; execve takes the name the command sees as part of
// its arguments, which is what makes `exec -a name cmd` expressible at all.
//
// files is the whole descriptor table the replacement is to be given, named
// streams included: entry i is descriptor i, and a nil entry is a number that
// must not be open there. It reaches down to zero where the other halves of
// this boundary start at 3, because those hand the named streams over
// separately and there is no separately here — this process's own 0, 1 and 2
// are what the command will find. Placing them is process work of exactly the
// kind the exec itself is — the process's own descriptor table is being
// rewritten — which is why it happens here and not in the package that decided
// *which* descriptors those are.
//
// A descriptor that could not be placed does not stop the exec. The command
// runs with that number missing, which is what a shell does with every other
// descriptor it could not give it, and refusing to run at all would be a
// larger failure than the one being reported.
//
// The collector is stopped for the window between the placement and the
// execve, and that is not an optimization or a precaution — it is the whole of
// #695, which had been written off as a flaky test.
//
// Placing the table overwrites descriptor numbers this process did not choose,
// and some of them belong to the Go runtime. The netpoller's epoll descriptor
// is the one that bites: `exec 5>f; exec cmd` puts the file on 5, and 5 is
// commonly where the runtime's epoll descriptor already sits. Nothing notices
// until the runtime polls, and then
//
//	runtime: epollwait on fd 5 failed with 22
//	fatal error: runtime: netpoll failed
//
// is a *fatal* error rather than a returned one: the shell dies where it was
// about to become the command. The window is normally microseconds and the
// failure is silent — a script that redirects the replacement's diagnostics
// away, as the tests for this do, sees only an empty file — which is what made
// it look like a test that fails one run in twenty on Linux and never on
// macOS, where the poller is kqueue and the numbers fall differently.
//
// What runs in that window is `syscall.Exec` itself: it converts the path, the
// arguments and the environment into C strings, three allocations, and an
// allocation is where a collection starts. Stopping the world to start one
// ends by starting it again, and starting the world polls. Measured: an
// allocation of four mebibytes placed in this window kills the shell
// twenty-five times out of twenty-five on Linux, and none out of twenty-five
// with the collector stopped.
//
// Stopping it is safe because this function does not return on success — there
// is no process left to collect for — and it is put back if the exec fails,
// where there is.
//
// Stopping the collector closes one half of the window and cannot close the
// other, which is worth stating rather than leaving to be rediscovered:
// `sysmon` polls on a timer of its own from another thread, so a window that
// is *long* is dangerous whatever the collector is doing. That half is closed
// by emptying the window instead of shortening it — everything the exec is
// going to allocate is built by prepareExec, before a single descriptor has
// been overwritten, so what is left between the placement and the execve is
// system calls. replaceexec_linux.go carries the mechanism and the numbers.
//
// The invariant the two halves share, and the one thing to keep: **nothing
// may allocate between the placement and the execve.** A conversion moved
// back below placeFiles would compile, pass every test that runs a shell, and
// put the failure back — which is why `TestNothingAllocatesInTheWindow`
// counts rather than trusts.
//
// # What is left, and why it stays
//
// The window is now empty of everything this repository puts in it. An strace
// of `exec 5>f; exec cmd` is the dup3 and then the execve, adjacent:
//
//	dup3(8, 5, 0) = 5
//	execve("/bin/sh", ...)
//
// It is still not safe, and the remainder is the kernel's rather than ours.
// execve on a *multithreaded* process kills the other threads inside the
// call, and until it has, they are running against a descriptor table that
// has already changed. sysmon is one of them and polls on a timer of its own.
// Nothing written in Go can shorten that interval, and no ordering here can
// avoid it: the script asked for its file on descriptor 5 and that is where
// it has to go.
//
// Which number the runtime holds is not luck, either, and it is the part
// worth knowing. Measured, this shell's own startup creates it: opening the
// script leaves 3 and 4 briefly free, and the poller lands immediately after
// on 5 — the number a script is most likely to park on after 3 and 4.
//
//	openat("s.sh", O_RDONLY|O_CLOEXEC) = 4
//	epoll_create1(EPOLL_CLOEXEC)       = 5
//	eventfd2(0, EFD_CLOEXEC|EFD_NONBLOCK) = 6
//
// So the collision is between two low numbers, both chosen by accident. The
// direction that would actually close it is to stop the runtime taking a
// number a script can reach — reserving the low descriptors until the poller
// exists, which is #731 option 2 and is a change to how the shell starts
// rather than to how it execs. It is not made here.
//
// Until then a replacement that dies this way has not answered the question a
// test asked, and the tests retry it rather than report it — loudly, and
// counting every attempt. See replacementAttempts in replacefds_test.go.
func replaceProcess(path string, argv, env []string, files []*os.File) error {
	prev := debug.SetGCPercent(-1)
	// Before the table is touched, because this is the part that allocates.
	ready, err := prepareExec(path, argv, env)
	if err != nil {
		debug.SetGCPercent(prev)
		return err
	}
	reachedPlacement("before")
	placeFiles(files)
	reachedPlacement("after")
	err = ready.execve()
	// Reached only when the exec failed. The process is a shell again, so it
	// collects again, and gets back whatever the exec had already spent.
	debug.SetGCPercent(prev)
	ready.undo()
	// And here for the sake of the files rather than the error: nothing refers
	// to the slice after placeFiles, so without this the collector may close a
	// descriptor's original in the window between placing it and the exec that
	// was to inherit it.
	runtime.KeepAlive(files)
	return err
}
