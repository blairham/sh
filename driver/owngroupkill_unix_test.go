// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/testenv"
)

// ownGroupScript carries a script into a re-executed test binary that leads a
// process group of its own.
const ownGroupScript = "SH_TEST_OWN_GROUP_KILL_SCRIPT"

// **A trapped signal a script sends its own process group runs the handler
// before the next command**, as one sent to `$$` does (#7043).
//
// The group is the shell's and its children's, so the send has to reach the
// kernel — but the shell's own copy then came back through os/signal on a
// goroutine, and the handler ran whenever that goroutine was scheduled: after
// `echo after`, or not at all once the script had ended. Measured 2026-10-31,
// each shell in a group of its own: bash 5.3.20, bash 3.2.57, ksh93u+, zsh
// 5.9.2 and dash all write `T` and then `after` for the first two rows, and
// this shell wrote `after` and then `T`, or `after` alone.
//
// The shell half refuses to run unless it leads its group. `kill 0` from a
// process in the test's own group would reach `go test` and whatever started
// it, and that is not a failure anybody should have to read about in a
// terminal that has just been signaled.
func TestATrappedSignalSentToTheShellsOwnGroupIsHandledBeforeTheNextCommand(t *testing.T) {
	if src := os.Getenv(ownGroupScript); src != "" {
		writeNoCore()
		if syscall.Getpgrp() != os.Getpid() {
			fmt.Fprintln(os.Stderr, "the shell half does not lead a process group of its own")
			os.Exit(125)
		}
		sh := shell()
		sh.Dialect, sh.Semantics, sh.Diagnostics = bash.Dialect(), bash.Semantics(), bash.Diagnostics()
		os.Exit(driver.MainArgs(sh, []string{"testsh", "-c", src}))
	}
	usr1 := strconv.Itoa(128 + int(syscall.SIGUSR1))
	for _, c := range []struct{ name, src, want string }{
		{"to group 0", `trap 'echo T' USR1; kill -USR1 0; echo after`, "T\nafter\n"},
		{"to the group by its number", `trap 'echo T' USR1; kill -USR1 -$$; echo after`, "T\nafter\n"},
		{"with -s and another signal", `trap 'echo H' HUP; kill -s HUP 0; echo after`, "H\nafter\n"},
		// The kernel's copy arrives during the `sleep`, and the send has
		// already answered it. Running it as well is the handler twice.
		{
			"and the kernel's copy is not run again",
			`trap 'echo T' USR1; kill -USR1 0; sleep 0.3; echo after`, "T\nafter\n",
		},
		// Two sends are two arrivals, as in every reference; with the copies
		// left to the runtime, it folded them into one and ran it late.
		{
			"twice, which runs it twice",
			`trap 'echo T' USR1; kill -USR1 0; kill -USR1 0; sleep 0.3; echo after`, "T\nT\nafter\n",
		},
		// And the rest of the group still receives it: the child dies of the
		// signal, which is what makes this a send and not only a recording.
		{
			"while a child in the group dies of it",
			`trap 'echo T' USR1; sleep 30 & sleep 0.3; kill -USR1 0; wait $!; echo st=$?`, "T\nst=" + usr1 + "\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs := runInAGroupOfItsOwn(t, c.src)
			if out != c.want {
				t.Errorf("%s\nwrote %q, want %q (stderr %q)", c.src, out, c.want, errs)
			}
		})
	}
}

// runInAGroupOfItsOwn re-executes this test binary as a shell running src, as
// the leader of a new process group, and returns what it wrote to each stream.
func runInAGroupOfItsOwn(t *testing.T, src string) (out, errs string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), deathDeadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0],
		"-test.run=^TestATrappedSignalSentToTheShellsOwnGroupIsHandledBeforeTheNextCommand$")
	cmd.Env = testenv.WithoutRaceExitPause(append(os.Environ(), ownGroupScript+"="+src))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = time.Second
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the shell half: %v", err)
	}
	// The whole group, so a `sleep` the shell left behind goes too.
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
	_ = cmd.Wait()
	if ctx.Err() != nil {
		t.Fatalf("%s: still running after %v", src, deathDeadline)
	}
	return o.String(), e.String()
}
