// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

// deathGrace is how long the raise is given before this process gives up on
// dying properly and exits with the number instead.
//
// Nothing here expects to use it. The kernel runs the default action as part
// of delivering the signal, so a successful raise ends the process before the
// first sleep returns, and every path below that can fail returns an error
// rather than arriving here. It exists because the failure it guards against
// was a *hang* — a shell that sat in this loop until something else killed it,
// costing ten seconds of harness timeout per case and reading, from the
// outside, as an implementation that never answers. A wrong status is a bug
// report; a wedged process is a morning.
const deathGrace = 5 * time.Second

// deathPoll is how often the parked loop looks up. Only its total matters.
const deathPoll = 100 * time.Millisecond

// dieBySignal ends this process with the signal the script sent it.
//
// It lives in driver rather than in interp for the reason replaceProcess does:
// a Runner embedded in another program must not be talked into killing that
// program by the text it was handed. A binary that *is* a shell is the one
// place this is the right answer, and this is that place.
//
// # The disposition has to be genuinely the default first
//
// The Go runtime installs a handler for nearly every signal at startup and
// decides for itself what to do with one nothing is listening for, and its
// answer depends on a classification we do not get a say in. Signals it marks
// as killing — HUP, INT, TERM — it forwards to the default action, which is
// exactly what a shell wants. Signals it marks as throwing — QUIT, ABRT, and
// the fault signals — it turns into a goroutine dump on standard error and an
// exit status of 2, which is the same leak internal/panicguard exists to stop
// arriving by another road. Everything else it *discards*: measured, USR1,
// USR2, ALRM, PIPE, XCPU, XFSZ, VTALRM and PROF raised at this process with no
// listener did nothing at all, and the loop below then held the shell open
// forever.
//
// signal.Reset does not undo any of that. It undoes a previous Notify or
// Ignore, and an untrapped signal was never passed to either — so there is
// nothing for it to undo and the runtime's own handler stays installed. The
// disposition has to be put back to the default by asking the kernel directly,
// which is what restoreDefaultDisposition does and why it is written once per
// platform.
//
// Reset still comes first: a signal this shell *did* trap at some point is
// registered with os/signal, and leaving that registration in place would let
// the notify path claim the raise before the kernel got to it.
//
// # After the raise there is nothing useful left to do
//
// The thread the kernel picks to run the default action is not necessarily
// this one, so parking rather than returning is the whole point: a shell that
// carried on would run the next command in a process it has already asked the
// kernel to end, which is exactly the ordering this all exists to get right.
func dieBySignal(sig syscall.Signal) error {
	signal.Reset(sig)
	// SIGKILL and SIGSTOP have no disposition to restore — the kernel refuses
	// to let anything catch, block or ignore them, so asking is EINVAL rather
	// than a no-op, and the default action is the only thing they ever do.
	// Found by the corpus: `kill -KILL $$` reported `kill: invalid argument`
	// on its way to dying correctly anyway.
	if sig != syscall.SIGKILL && sig != syscall.SIGSTOP {
		if err := restoreDefaultDisposition(sig); err != nil {
			return err
		}
	}
	if err := syscall.Kill(os.Getpid(), sig); err != nil {
		return err
	}
	for waited := time.Duration(0); waited < deathGrace; waited += deathPoll {
		// Sleeping rather than blocking forever on a channel: the runtime
		// calls that a deadlock and panics, which would print a Go stack where
		// a shell should print nothing at all.
		time.Sleep(deathPoll)
	}
	// Unreachable on a platform whose disposition could be restored, and the
	// honest answer where one could not: 128 plus the number is what a shell
	// reports for a command a signal killed, so a caller reading the status
	// still learns which signal it was.
	os.Exit(signalStatusBase + int(sig))
	return nil
}

// signalStatusBase is the offset a shell adds to a signal number to report a
// death by it as an exit status.
const signalStatusBase = 128
