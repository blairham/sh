// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package interp_test

import (
	"sync/atomic"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A signal the runtime has received by the time a child the shell waited for
// has ended is answered as that pipeline ends, however late os/signal's
// goroutine gets to put it on the shell's channel (#7378).
//
// The sender of the issue is `/bin/kill -USR1 $$`, which exits as soon as it
// has sent: the kernel hands the signal to the runtime before the shell reaps
// it, and the runtime's goroutine passes it on when it is next scheduled —
// after the shell had looked, in 174 runs of 1000 on Linux. A real sender
// leaves that to the scheduler, so this one is made exact: the shell's own
// thread raises USR1 on itself the moment the child is reaped, which runs the
// runtime's handler before the raise returns and leaves the arrival in the
// runtime and not yet on the channel. The `if` is what tells the two moments
// apart: answered as its condition ends, the handler writes before `t`; at the
// next statement, after it.
//
// Linux only, because only tgkill aims a signal at one thread of the process:
// elsewhere the kernel picks the thread, and one that has yet to run its
// handler leaves the arrival outside the runtime, where no barrier can see it.
func TestAnArrivalTheRuntimeHasAsAChildEndsIsAnsweredAsThePipelineEnds(t *testing.T) {
	restoreDispositionsAfter(t, []string{"USR1"})
	var raise atomic.Bool
	defer SetAfterAChildIsWaitedFor(func() {
		if raise.Swap(false) {
			if err := syscall.Tgkill(syscall.Getpid(), syscall.Gettid(), syscall.SIGUSR1); err != nil {
				t.Errorf("tgkill: %v", err)
			}
		}
	})()
	const src = "trap 'echo U' USR1\n" +
		"if /bin/true; then echo t; fi\n" +
		"echo b\n"
	late := 0
	for range 50 {
		raise.Store(true)
		out, errs, st := killRun(t, src, killSem(), Diagnostics{})
		if out != "U\nt\nb\n" || errs != "" || st != 0 {
			late++
			if late == 1 {
				t.Errorf("got %q, stderr %q, status %d; want %q", out, errs, st, "U\nt\nb\n")
			}
		}
	}
	if late > 0 {
		t.Errorf("answered after the condition in %d runs of 50", late)
	}
}
