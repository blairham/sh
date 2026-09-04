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
	for {
		var ws syscall.WaitStatus
		var ru syscall.Rusage
		_, err := syscall.Wait4(pid, &ws, syscall.WUNTRACED, &ru)
		if errors.Is(err, syscall.EINTR) {
			// A signal arrived while waiting — the shell's own SIGTSTP or
			// SIGINT handler, most often. That is not an answer about the
			// child, so ask again.
			continue
		}
		if err != nil {
			return interp.Wait{}, err
		}
		switch {
		case ws.Stopped():
			// Still there: nothing has been reaped, so there is no usage to
			// report yet.
			return interp.Wait{Signal: ws.StopSignal(), Stopped: true}, nil
		case ws.Signaled():
			return interp.Wait{
				Signal: ws.Signal(), Killed: true,
				User: time.Duration(ru.Utime.Nano()), System: time.Duration(ru.Stime.Nano()),
			}, nil
		default:
			return interp.Wait{
				Status: ws.ExitStatus(),
				User:   time.Duration(ru.Utime.Nano()), System: time.Duration(ru.Stime.Nano()),
			}, nil
		}
	}
}
