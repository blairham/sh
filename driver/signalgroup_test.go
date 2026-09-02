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

// A signal aimed at a job reaches the whole group, not only its leader.
//
// Two processes in the group is the only arrangement that can tell those
// apart: signaling the leader alone leaves the other running, so a test with
// one process would pass on the bug.
func TestSignalGroupReachesTheWholeGroup(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "sleep 30 & sleep 30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Skipf("could not start a process group here: %v", err)
	}
	pgid := cmd.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) })

	// Give the inner shell a moment to start both.
	deadline := time.Now().Add(3 * time.Second)
	for syscall.Kill(-pgid, 0) != nil {
		if time.Now().After(deadline) {
			t.Skip("the group never came up")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := signalGroup(pgid, syscall.SIGTERM); err != nil {
		t.Fatalf("signalGroup: %v", err)
	}
	// Reaped, and this is load-bearing rather than tidiness. A zombie is
	// still a member of its process group, and the two platforms disagree
	// about what that means for a group holding nothing else: a BSD answers
	// EPERM and Linux answers success. Without this the test asks "are there
	// zombies" on one and "is anything left" on the other — it passed on a
	// deliberately broken build here and failed on a correct one in CI, which
	// is how the difference turned up.
	_ = cmd.Wait()

	// Nothing is left alive in it. The other member is not this process's
	// child — it belongs to the shell that started it — so once the leader is
	// reaped, anything still answering is something still running.
	deadline = time.Now().Add(5 * time.Second)
	for {
		if err := syscall.Kill(-pgid, 0); err != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("something is still alive in the group — the signal " +
				"reached its leader rather than the group")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
