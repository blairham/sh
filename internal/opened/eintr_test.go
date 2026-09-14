// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package opened

import (
	"errors"
	"syscall"
	"testing"
)

// An interrupted call is made again, and a call that answers anything else is
// not.
//
// Both halves, because a loop that retried everything would turn a real ENOENT
// into a spin and would pass a test that only checked the first.
func TestAnInterruptedCallIsMadeAgain(t *testing.T) {
	t.Run("an interruption is waited through", func(t *testing.T) {
		calls := 0
		got, err := retrying(func() (int, error) {
			calls++
			if calls < 3 {
				return -1, syscall.EINTR
			}
			return 7, nil
		})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if got != 7 {
			t.Errorf("got %d, want the answer of the call that was not interrupted", got)
		}
		if calls != 3 {
			t.Errorf("the call was made %d times, want 3 — twice interrupted and once not", calls)
		}
	})

	t.Run("an interruption wrapped in something else is still one", func(t *testing.T) {
		// The walk hands back what the platform file produced, and a platform
		// file is free to wrap. errors.Is rather than == is what makes that
		// safe, and this is the case that says so.
		calls := 0
		if _, err := retrying(func() (int, error) {
			calls++
			if calls < 2 {
				return -1, &wrapped{syscall.EINTR}
			}
			return 0, nil
		}); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if calls != 2 {
			t.Errorf("the call was made %d times, want 2", calls)
		}
	})

	t.Run("any other answer is the answer", func(t *testing.T) {
		calls := 0
		_, err := retrying(func() (int, error) {
			calls++
			return -1, syscall.ENOENT
		})
		if !errors.Is(err, syscall.ENOENT) {
			t.Errorf("err = %v, want ENOENT handed straight back", err)
		}
		if calls != 1 {
			t.Errorf("the call was made %d times, want 1 — nothing to retry", calls)
		}
	})
}

type wrapped struct{ err error }

func (w *wrapped) Error() string { return "wrapped: " + w.err.Error() }
func (w *wrapped) Unwrap() error { return w.err }

// There is deliberately no case here that *produces* an interruption, and the
// reason belongs beside the ones that can be produced rather than as an
// absence.
//
// What would have to be built is a signal landing on the thread a raw open is
// parked on, and a Go program cannot arrange it: the runtime installs every
// handler it installs with `SA_RESTART`, so a caught signal restarts the call
// rather than ending it. Measured 2026-09-14 on macOS 26.5.2, a standalone
// program parked in a blocking FIFO open and signaled every 100µs for 150ms —
// SIGUSR1, SIGUSR2, SIGCHLD, SIGWINCH, SIGURG and SIGCONT in turn, each with
// signal.Notify holding a handler — and the open answered `<nil>` every time.
// An attempt at a case this way passed identically with the retry taken out,
// which is a case that watches nothing.
//
// What the interruption is known from is the failure that filed #2751: a
// macOS runner told a script `cannot open …/sub2: Interrupted system call` on
// a redirection onto a process substitution's pipe, where the open waits for
// the substitution's writer. So the state is real and reachable under a load
// this machine does not reproduce on demand — which makes the loop above the
// testable half and the diagnostic the evidence.
