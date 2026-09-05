// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver

import (
	"errors"
	"syscall"
	"time"

	"github.com/blairham/sh/interp"
)

// waitForCommand waits the way a shell has to.
//
// WUNTRACED is the whole of it. An ordinary wait returns when a child *ends*,
// and a child that ^Z stopped has not ended — so a shell without this does not
// so much hang as wait forever for something that is never going to finish.
// That was the behavior before this existed: ^Z at the prompt left the shell
// blocked and the job stopped, with nothing able to reach either.
//
// It lives in driver for the reason the other process hooks do. Reaping is
// process-wide: a Runner embedded in a program that handles its own children
// must not have a library calling wait4 behind its back, so interp asks and
// the binary answers.
func waitForCommand(pid int) (interp.Wait, error) {
	w, _, err := wait(pid, syscall.WUNTRACED)
	return w, err
}

// pollCommand is waitForCommand without the waiting: it says whether the
// command has changed state and does not block if it has not.
//
// It is what finishes a job `bg` resumed. Nothing else is waiting on such a
// job — the wait that returned when ^Z stopped it is over — so unless the
// shell asks, the process stays in the table as "running" for the rest of the
// session and is never reaped.
//
// WNOHANG is the whole of the difference, and it is a second function rather
// than a flag on the first because the answer has a third shape: nothing
// happened. A blocking wait has no such case.
func pollCommand(pid int) (interp.Wait, bool, error) {
	return wait(pid, syscall.WUNTRACED|syscall.WNOHANG)
}

// wait is the one call both of them make. changed is false only for the
// non-blocking form, where it means the child is still doing what it was.
func wait(pid, flags int) (interp.Wait, bool, error) {
	for {
		var ws syscall.WaitStatus
		var ru syscall.Rusage
		reaped, err := syscall.Wait4(pid, &ws, flags, &ru)
		if errors.Is(err, syscall.EINTR) {
			// A signal arrived while waiting — the shell's own SIGTSTP or
			// SIGINT handler, most often. That is not an answer about the
			// child, so ask again.
			continue
		}
		if err != nil {
			return interp.Wait{}, false, err
		}
		if reaped == 0 {
			// WNOHANG, and the child has not stopped, been signaled or
			// exited since it was last asked about.
			return interp.Wait{}, false, nil
		}
		switch {
		case ws.Stopped():
			// Still there: nothing has been reaped, so there is no usage to
			// report yet.
			return interp.Wait{Signal: ws.StopSignal(), Stopped: true}, true, nil
		case ws.Signaled():
			return interp.Wait{
				Signal: ws.Signal(), Killed: true,
				User: time.Duration(ru.Utime.Nano()), System: time.Duration(ru.Stime.Nano()),
			}, true, nil
		default:
			return interp.Wait{
				Status: ws.ExitStatus(),
				User:   time.Duration(ru.Utime.Nano()), System: time.Duration(ru.Stime.Nano()),
			}, true, nil
		}
	}
}
