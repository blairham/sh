// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/testenv"
)

// forwardedScript carries a script into a re-executed test binary.
const forwardedScript = "SH_TEST_FORWARDED_TRAP_SCRIPT"

// **A signal another process sends is handled between statements, while the
// shell is busy with nothing but its own statements** (#6496).
//
// The check at the top of every statement answers "nothing arrived" without
// taking the lock, which is where the time went in a counting loop. What it
// must still see is a signal the runtime forwarded and nobody has drained:
// the shell is not waiting on a child, so the only reader of that channel is
// the statement check itself. A check that looked only at what the shell
// queued for itself (`kill` aimed at `$$`) would leave this loop spinning to
// the deadline, and every in-process test of traps passes on that bug, since
// they all raise the signal through the builtin.
//
// The sender is a background job that sleeps first, so the arrival lands
// while the loop runs and not while the shell waits for the `sh` it started.
func TestASignalFromAnotherProcessIsHandledInABusyLoop(t *testing.T) {
	if src := os.Getenv(forwardedScript); src != "" {
		writeNoCore()
		sh := shell()
		sh.Dialect, sh.Semantics, sh.Diagnostics = bash.Dialect(), bash.Semantics(), bash.Diagnostics()
		os.Exit(driver.MainArgs(sh, []string{"testsh", "-c", src}))
	}
	const src = `trap 'got=1' USR1
/bin/sh -c 'sleep 0.2; kill -USR1 $PPID' &
got=
while [ -z "$got" ]; do :; done
echo caught`
	ctx, cancel := context.WithTimeout(context.Background(), deathDeadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestASignalFromAnotherProcessIsHandledInABusyLoop")
	cmd.Env = testenv.WithoutRaceExitPause(append(os.Environ(), forwardedScript+"="+src))
	cmd.WaitDelay = time.Second
	out, _ := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf(
			"the loop was still waiting for the trap after %v: the forwarded USR1 was never handled (wrote %q)",
			deathDeadline,
			out,
		)
	}
	if string(out) != "caught\n" {
		t.Fatalf("got %q, want %q", out, "caught\n")
	}
}
