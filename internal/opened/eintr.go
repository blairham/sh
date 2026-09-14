// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"errors"
	"syscall"
)

// The walk opens with raw system calls, and a raw system call in a Go program
// can come back interrupted.
//
// This is the one thing `os.OpenFile` does for its callers that is easy to
// forget when leaving it behind, and leaving it behind is not optional here:
// the walk needs `openat` against a pinned parent descriptor, and O_CREAT has
// to be asked about between the walk and the open. What comes with that is
// `ignoringEINTR`, which os hides — and which nothing replaced.
//
// The runtime preempts goroutines by signaling the thread they are on, so a
// program that installs no handler of its own is still signaled, frequently,
// and a loaded machine is when it is. `SA_RESTART` covers most of what that
// interrupts and does not cover all of it: opening a FIFO waits for a peer,
// and that wait is interruptible on this family of kernels.
//
// The window is not theoretical and is not narrow. Every `cat < <(cmd)` has a
// blocking open in it, waiting for the substitution's writer to be let go by
// its poll — up to the five milliseconds that poll's backoff reaches. Measured
// by the failure that found this: `cat < <(true)` on the macOS runner, with
// the script told
//
//	sh: cannot open /…/sh-procsub731729813/sub2: Interrupted system call
//
// which is not an answer any shell gives for a redirection (#2751).
//
// It is the same defect as #1909, one layer down. That one was a raw
// `syscall.Open` in a *test helper*, and the helper was given a retry loop and
// a comment saying why. The shell's own path was left as it was, so the
// identical failure stayed reachable from a script rather than only from a
// test.
//
// Retrying is safe for every flag the walk passes, including the creating
// ones: EINTR means the call did not take effect, so an O_CREAT|O_EXCL that
// answers it has made nothing and the retry is the first attempt again. The
// one call that is *not* retried is the one that cannot block — see openRoot.

// retrying calls do until it answers something other than an interruption.
//
// A function rather than three loops, because the loop is the whole of the
// fix and three copies of it is three places for one of them to be dropped —
// which is how this package came to have none. It is also the only part of
// this that can be tested: see TestAnInterruptedCallIsMadeAgain, and the
// paragraph under it for why the interruption itself cannot be.
func retrying[T any](do func() (T, error)) (T, error) {
	for {
		v, err := do()
		if !errors.Is(err, syscall.EINTR) {
			return v, err
		}
	}
}

// openat opens name relative to dirfd, waiting through an interruption.
func openat(dirfd int, name string, flags int, perm uint32) (int, error) {
	return retrying(func() (int, error) { return openatOnce(dirfd, name, flags, perm) })
}

// readlinkat reads the link at name relative to dirfd, on the same terms.
//
// A readlink does not wait for anything, so this is the weaker of the two
// claims: it is here because "any raw system call can be interrupted" is the
// rule, and a call that is exempt by reasoning rather than by a guarantee is
// the kind that stops being exempt when somebody changes what it is called on.
// os.Readlink retries for the same reason.
func readlinkat(dirfd int, name string, buf []byte) (int, error) {
	return retrying(func() (int, error) { return readlinkatOnce(dirfd, name, buf) })
}

// openRoot opens `/`, where the walk starts.
//
// Retried like the others rather than exempted for being a directory: the
// exemption would be an argument about which opens can block, and the point of
// the rule above is not to have one.
func openRoot(flags int) (int, error) {
	return retrying(func() (int, error) { return syscall.Open("/", flags, 0) })
}
