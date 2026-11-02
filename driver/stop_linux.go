// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"runtime"
	"syscall"
)

// raiseStop raises sig on the thread that is running the script, so that the
// shell is stopped before the raise returns.
//
// # Why not the process
//
// A signal aimed at a process is the kernel's to hand to any one of its
// threads, and Linux hands it to the thread-group leader when that thread can
// take it. A Go program's leader is rarely the thread running the script, so
// the raise returns at once and the stop lands later, whenever the leader is
// next scheduled. Meanwhile the script runs on: `kill -TSTP $$; echo alive`
// could write `alive` before stopping, and a script with nothing after the
// kill could exit before the stop landed at all, so the shell that started it
// never saw it stopped. A shell written in C has one thread, so for every shell
// this one is measured against the stop is taken before `kill` returns.
//
// Found by TestAStopAimedAtTheShellStopsTheProcessWithIt, which only passed
// under -race because the race detector holds every exit back by a second, and
// failed in 3 of its 5 rows the day that pause was taken away (#7290). Under
// CPU contention it fails without -race as well, and the SIGSTOP row with it:
// nothing here is particular to the three stops a terminal sends.
//
// # Why this thread
//
// A signal aimed at one thread is that thread's to take, and it takes it on its
// way back from the system call that sent it. For a stop that is the group stop
// of the whole process, entered before this function returns and left only
// when something continues it. Every rule the kernel has about the signal
// still holds, because the kernel is still the one deciding: TSTP, TTIN and
// TTOU are discarded for an orphaned process group, which returns at once, and
// a handler the front end installed answers instead.
//
// Locked to the thread so that the id read here is the id of the thread that
// makes the raise. Without the lock the goroutine could move between the two
// calls and aim the signal at a thread that is not the one running the script,
// which is the bug again.
func raiseStop(sig syscall.Signal) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	return syscall.Tgkill(os.Getpid(), syscall.Gettid(), sig)
}
