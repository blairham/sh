// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver

import (
	"io"
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
	// The inner shell says when the members exist, and that handshake is the
	// whole of #2382's second flake.
	//
	// What was here waited for `kill(-pgid, 0)` to succeed, which is true the
	// moment the **leader** exists and says nothing at all about the two
	// `sleep`s the assertion is about. Measured on this machine, 2026-09-14,
	// by counting the group's real membership at the instant that loop
	// returned: **166 of 200 rounds had one process in the group**, 19 had two
	// and 15 had three. So four times in five the signal went to a group whose
	// members had not been forked yet.
	//
	// A member forked *after* the group is signaled escapes it outright: the
	// shell is inside fork(2) when SIGTERM lands, the child is created with no
	// pending signal of its own, it execs `sleep 30`, and it is still there
	// thirty seconds later. The window is one fork wide, which is why this
	// only ever failed on a loaded runner — `FAIL (5.02s)` is the five-second
	// deadline below expiring on a `sleep` that was never signaled, and it
	// read as "the signal reached the leader rather than the group" when the
	// signal had in fact reached every member there was.
	//
	// So both are backgrounded and the shell prints after forking them.
	// Nothing about what is being measured changes: with three processes in
	// the group, a signalGroup that reached only the leader still leaves two
	// `sleep`s answering, which is the failure this test exists to catch.
	cmd := exec.Command("/bin/sh", "-c", "sleep 30 & sleep 30 & echo ready; wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	ready, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Skipf("could not start a process group here: %v", err)
	}
	pgid := cmd.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) })

	// Bounded, because a shell that never says it is ready must fail this
	// test rather than wedge the package until `go test` gives up on it.
	said := make(chan error, 1)
	go func() {
		var buf [len("ready\n")]byte
		_, err := io.ReadFull(ready, buf[:])
		said <- err
	}()
	select {
	case err := <-said:
		if err != nil {
			t.Skipf("the group never came up: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the inner shell never reported its jobs started")
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
	deadline := time.Now().Add(5 * time.Second)
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
