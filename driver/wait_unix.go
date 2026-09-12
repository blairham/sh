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
		if sig, stopped := stoppedBy(ws); stopped {
			// Still there: nothing has been reaped, so there is no usage to
			// report yet.
			return interp.Wait{Signal: sig, Stopped: true}, true, nil
		}
		switch {
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

// stoppedBy is the signal that stopped a child, and whether one did.
//
// This is decoded here rather than taken from syscall.WaitStatus.Stopped,
// which answers it for every stop signal *except the one a script is most
// likely to send*:
//
//	func (w WaitStatus) Stopped() bool { return w&mask == stopped && Signal(w>>shift) != SIGSTOP }
//
// The exclusion is deliberate on the standard library's side — on the BSDs a
// continued child is reported through the same encoding, so the package
// reads a wait status carrying SIGSTOP as Continued and not as Stopped —
// and for a shell it is simply the wrong answer. `kill -STOP` is how a
// script stops a job; ^Z sends SIGTSTP and was therefore the only stop this
// shell ever saw.
//
// What it cost is #2227. A foreground command a script stopped came back as
// neither stopped nor exited nor signaled, so the wait fell through to "it
// ended", os/exec's own Wait was called on a process that was still there,
// and the shell sat in it for as long as something outside took to resume or
// kill the job. A `wait` for a background job in the same state waited just
// as long. Neither is a loop and neither leaves a frame of ours on a CPU,
// which is why the instrument that found it recorded a shell idle in
// __wait4_nocancel with nothing running.
//
// The test is the C macro's — WIFSTOPPED is `(status & 0xff) == 0x7f` — and
// it is unambiguous here because this package never asks for WCONTINUED: a
// continued child is only ever reported to a wait that requested it, and
// neither of the two calls above does. The one encoding that could collide
// is Linux's WIFCONTINUED, 0xFFFF, which is excluded rather than reasoned
// about.
func stoppedBy(ws syscall.WaitStatus) (syscall.Signal, bool) {
	if ws.Stopped() {
		return ws.StopSignal(), true
	}
	const (
		stopped   = 0x7f
		continued = 0xffff
	)
	if ws == continued || ws&stopped != stopped {
		return 0, false
	}
	return syscall.Signal(ws>>8) & 0xff, true
}
