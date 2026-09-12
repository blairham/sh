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
// This is decoded here rather than taken from syscall.WaitStatus.Stopped
// because that call answers it differently on the two platforms this runs on,
// and one of the two answers is no use to a shell. syscall_linux.go has
//
//	func (w WaitStatus) Stopped() bool { return w&0xFF == stopped }
//
// and syscall_bsd.go — macOS included — has
//
//	func (w WaitStatus) Stopped() bool { return w&mask == stopped && Signal(w>>shift) != SIGSTOP }
//
// The BSD exclusion is deliberate on that side: those kernels report a
// *continued* child through the same encoding, so the package reads a status
// carrying SIGSTOP as Continued rather than as Stopped. For a shell it is the
// wrong answer, because `kill -STOP` is how a script stops a job — and ^Z
// sends SIGTSTP, which is why this shell only ever saw the stop it was not
// going to be asked about.
//
// What it cost is #2227, on macOS. A foreground command a script stopped came
// back as neither stopped nor exited nor signaled, so the wait fell through to
// "it ended", os/exec's own Wait was called on a process that was still there,
// and the shell sat in it for as long as something outside took to resume or
// kill the job. A `wait` for a background job in the same state waited just as
// long. Neither is a loop and neither leaves a frame of ours on a CPU, which
// is why the instrument that found it recorded a shell idle in
// __wait4_nocancel with nothing running.
//
// The test is the C macro's — WIFSTOPPED is `(status & 0xff) == 0x7f` — which
// is what Linux already does and what the BSDs do apart from the one signal.
// It is unambiguous here because this package never asks for WCONTINUED: a
// continued child is only ever reported to a wait that requested it, and
// neither of the two calls above does.
func stoppedBy(ws syscall.WaitStatus) (syscall.Signal, bool) {
	const (
		stopped   = 0x7f
		continued = 0xffff
	)
	// Before the standard library's own question and not after it, which is
	// the order the test found: `Stopped` reads 0xffff as a stop by signal
	// 255 on both platforms, so a guard placed behind it never runs. It
	// cannot arrive either way — neither call above asks for WCONTINUED, and
	// a continued child is only ever reported to a wait that did — so this is
	// the belt to that reasoning's braces, and it has to be first to be one.
	if ws == continued {
		return 0, false
	}
	if ws.Stopped() {
		return ws.StopSignal(), true
	}
	if ws&stopped != stopped {
		return 0, false
	}
	return syscall.Signal(ws>>8) & 0xff, true
}
