// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"syscall"
	"testing"
)

// TestAnUncatchableSignalBorrowsNoDisposition is the half of #2919 that
// nothing outside this package can see.
//
// `trap … KILL` is accepted because every shell on the panel accepts it, and
// the table entry is the whole of what it means: a signal disposition belongs
// to the *process*, and a Runner is a library living inside somebody else's.
// `trap ” KILL` reaching signal.Ignore would be this package asking the
// kernel for something it refuses without saying so, and a borrow recorded
// for a disposition never taken is one restoreDispositions would later
// "put back" on the way out.
//
// Nothing the script can observe comes of either, which is exactly why the
// ledger is what this reads: it is the one place the difference between
// doing nothing and doing something that did not work is written down.
//
// In-package for the same reason, and it imports no dialect, so it adds no
// cycle — see the note in AGENTS.md about why interp's tests are otherwise
// external.
func TestAnUncatchableSignalBorrowsNoDisposition(t *testing.T) {
	r := newTestRunner(t, &Runner{})
	t.Cleanup(r.stopSignalsAndRestore)
	action := "x"
	ignore := ""
	r.trapSignal("KILL", syscall.SIGKILL, &action)
	r.trapSignal("STOP", syscall.SIGSTOP, &ignore)

	s := r.sigs()
	s.mu.Lock()
	borrowed := len(s.borrowed)
	kill, hasKill := s.traps["KILL"]
	stop, hasStop := s.traps["STOP"]
	s.mu.Unlock()

	if borrowed != 0 {
		t.Errorf("borrowed %d process disposition(s) for signals nobody can catch, want 0", borrowed)
	}
	// The entry is still real. A shell that answered by dropping the request
	// on the floor would pass the line above and fail these: the listing has
	// to show what was asked for, and `trap -` has to have something to take
	// away.
	if !hasKill || kill != "x" {
		t.Errorf(`KILL: got %q/%v, want "x"`, kill, hasKill)
	}
	if !hasStop || stop != "" {
		t.Errorf(`STOP: got %q/%v, want ""`, stop, hasStop)
	}

	// And the reset, which is the line the old refusal made loudest: it
	// installs nothing, so there is nothing in it that could fail.
	r.trapSignal("KILL", syscall.SIGKILL, nil)
	s.mu.Lock()
	_, stillThere := s.traps["KILL"]
	borrowed = len(s.borrowed)
	s.mu.Unlock()
	if stillThere {
		t.Error("KILL survived its own reset")
	}
	if borrowed != 0 {
		t.Errorf("a reset borrowed %d disposition(s), want 0", borrowed)
	}

	// The control, without which an empty ledger proves only that the ledger
	// is never written. A signal that really can be caught does reach the
	// process, and is recorded as borrowed there.
	r.trapSignal("USR2", syscall.SIGUSR2, &action)
	s.mu.Lock()
	_, tookUSR2 := s.borrowed[syscall.SIGUSR2]
	s.mu.Unlock()
	if !tookUSR2 {
		t.Error("control: a catchable signal left nothing in the borrow ledger")
	}
}
