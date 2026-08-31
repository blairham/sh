// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

// dieBySignal ends this process with the signal the script sent it.
//
// It lives in driver rather than in interp for the reason replaceProcess does:
// a Runner embedded in another program must not be talked into killing that
// program by the text it was handed. A binary that *is* a shell is the one
// place this is the right answer, and this is that place.
//
// The default action has to be put back first, because the shell arranged to
// hear about these signals rather than die from them. After that the process
// is going to end, and the only question is when — the thread the kernel picks
// to run the default action is not necessarily this one, so there is nothing
// useful left to do here. Parking rather than returning is the whole point: a
// shell that carried on would run the next command in a process it has already
// asked the kernel to end, which is exactly the ordering this all exists to
// get right.
func dieBySignal(sig syscall.Signal) error {
	signal.Reset(sig)
	if err := syscall.Kill(os.Getpid(), sig); err != nil {
		return err
	}
	for {
		// Sleeping rather than blocking forever on a channel: the runtime
		// calls that a deadlock and panics, which would print a Go stack where
		// a shell should print nothing at all.
		time.Sleep(time.Second)
	}
}
