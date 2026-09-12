// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// TestAStoppedChildIsSeenWhateverStoppedIt is #2227's second stop, and the
// one the standard library will not answer: syscall.WaitStatus.Stopped is
//
//	w&mask == stopped && Signal(w>>shift) != SIGSTOP
//
// so the one signal a *script* sends to stop a job is the one it excludes.
// ^Z sends SIGTSTP, which is why this went unnoticed — and why the test sends
// both, since a decode that only handled the new case would still be half
// wrong and a test of SIGTSTP alone passed before the fix.
func TestAStoppedChildIsSeenWhateverStoppedIt(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGSTOP, syscall.SIGTSTP} {
		cmd := exec.Command("/bin/sleep", "30")
		if err := cmd.Start(); err != nil {
			t.Fatalf("%v: starting the child: %v", sig, err)
		}
		pid := cmd.Process.Pid
		if err := syscall.Kill(pid, sig); err != nil {
			t.Fatalf("%v: %v", sig, err)
		}
		w, err := waitForCommand(pid)
		if err != nil {
			t.Fatalf("%v: waiting: %v", sig, err)
		}
		if !w.Stopped {
			t.Errorf("%v: the wait says the child is not stopped, and it is", sig)
		}
		if w.Signal != sig {
			t.Errorf("%v: the wait says %v stopped it", sig, w.Signal)
		}
		// Still there, which is the whole difference from a child that
		// ended: nothing has been reaped, so this has to finish the job.
		_ = syscall.Kill(pid, syscall.SIGCONT)
		_ = syscall.Kill(pid, syscall.SIGKILL)
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Errorf("%v: the child was never collected", sig)
		}
	}
}

// TestAChildThatEndedIsNotStopped is the control in the other direction: the
// decode tests a bit pattern, so a status that is not a stop at all must not
// be read as one.
func TestAChildThatEndedIsNotStopped(t *testing.T) {
	cmd := exec.Command("/usr/bin/true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the child: %v", err)
	}
	w, err := waitForCommand(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("waiting: %v", err)
	}
	if w.Stopped || w.Killed || w.Status != 0 {
		t.Errorf("got %+v, want a plain exit at 0", w)
	}
}
