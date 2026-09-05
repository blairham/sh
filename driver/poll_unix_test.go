// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver_test

import (
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
)

// The poll a shell asks a resumed job with: nothing while it runs, and the
// status once it has ended.
//
// The negative half is the half worth having. A poll that answered "changed"
// for a running child would finish the job the moment it was asked about, so
// `bg` would report a command as done while it was still running — and a test
// that only checked the positive half would pass for exactly that bug.
func TestPollingACommandAnswersOnlyWhenItHasChanged(t *testing.T) {
	cmd := exec.Command("/bin/sleep", "0.4")
	// A group of its own, which is what a shell puts a job in; the poll must
	// work the same either way, and this is the arrangement it is used in.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Skipf("could not start a child to poll: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })

	// Still running, repeatedly: one negative answer could be a poll that has
	// simply not been reached yet, and a hundred of them across the child's
	// whole life could not.
	for i := 0; i < 100; i++ {
		w, changed, err := driver.PollCommandForTest(pid)
		if err != nil {
			t.Fatalf("poll %d: %v", i, err)
		}
		if changed {
			t.Fatalf("poll %d said the child changed to %+v while it was still running", i, w)
		}
		time.Sleep(time.Millisecond)
	}

	// And the status, once there is one. Waited for by polling rather than by
	// sleeping a guessed amount: the deadline turns a hang into a failure and
	// the loop is the thing being tested.
	deadline := time.Now().Add(10 * time.Second)
	for {
		w, changed, err := driver.PollCommandForTest(pid)
		if err != nil {
			t.Fatalf("poll after the child ended: %v", err)
		}
		if changed {
			if w.Stopped || w.Killed || w.Status != 0 {
				t.Errorf("poll = %+v, want a plain exit at 0", w)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the child never showed as finished")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// A child that stopped is reported as stopped, and reported once: the second
// ask has nothing new to say, which is what keeps a stopped job from being
// re-announced at every prompt.
func TestPollingReportsAStopOnce(t *testing.T) {
	cmd := exec.Command("/bin/sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Skipf("could not start a child to poll: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })

	// SIGTSTP rather than SIGSTOP, and the difference is the platform's
	// rather than a preference. On a BSD a continued child is reported as
	// "stopped by SIGSTOP", so Go's WaitStatus.Stopped answers false for that
	// signal to keep the two apart — and a child really stopped with SIGSTOP
	// is invisible to it there. SIGTSTP is what ^Z sends and what job control
	// is about, and it reads the same on both platforms.
	if err := syscall.Kill(-pid, syscall.SIGTSTP); err != nil {
		t.Fatalf("stopping the child: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		w, changed, err := driver.PollCommandForTest(pid)
		if err != nil {
			t.Fatalf("poll: %v", err)
		}
		if changed {
			if !w.Stopped {
				t.Fatalf("poll = %+v, want it stopped", w)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the stop was never reported")
		}
		time.Sleep(2 * time.Millisecond)
	}
	// Told once. A kernel that re-reported it would have the shell setting a
	// job's state over and over, which is harmless — and a shell built on the
	// other reading, that a stop is news every time, would not be.
	for i := 0; i < 20; i++ {
		w, changed, err := driver.PollCommandForTest(pid)
		if err != nil {
			t.Fatalf("poll %d: %v", i, err)
		}
		if changed {
			t.Fatalf("poll %d said %+v, want the stop reported only once", i, w)
		}
		time.Sleep(time.Millisecond)
	}
}
