// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
)

// interpretLine is what makes a bare `terminal/create` command mean anything
// (#1782), so what it has to be is a *shell* and not a command runner. These
// ask for the things an agent's line will actually contain — more than one
// command, a pipeline, a status, a working directory and an environment —
// because "it ran something" was true of the old behavior for a single
// program name too.
func TestInterpretLineRunsAWholeShellLine(t *testing.T) {
	t.Parallel()
	run := interpretLine(driver.Shell{Name: "sh"})

	for _, c := range []struct {
		name, line, want string
		status           int
	}{
		{
			// The thing an argv cannot express, and the reason the agent's
			// own `bash -c '…'` workaround could not have worked either.
			name: "more than one command", line: "echo one; echo two", want: "one\ntwo\n",
		},
		{name: "a pipeline", line: "echo hi | tr a-z A-Z", want: "HI\n"},
		{name: "an expansion", line: `x=world; echo "hello $x"`, want: "hello world\n"},
		{
			// Both streams are one stream, because a terminal is: the agent
			// is asking what a person would have seen on a screen.
			name: "standard error too", line: "echo out; echo err >&2", want: "out\nerr\n",
		},
		{name: "a status", line: "exit 7", status: 7},
		{name: "a failing command's status", line: "false", status: 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			status := run(t.Context(), c.line, "", nil, &out)
			if got := out.String(); got != c.want {
				t.Errorf("%s\n  wrote %q\n  want  %q", c.line, got, c.want)
			}
			if status != c.status {
				t.Errorf("%s: status %d, want %d", c.line, status, c.status)
			}
		})
	}
}

// The agent says where its command runs, and a create that ignored it would
// run the right line in the wrong place.
func TestInterpretLineRunsWhereTheAgentAsked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var out bytes.Buffer
	if status := interpretLine(driver.Shell{Name: "sh"})(t.Context(), "pwd -P", dir, nil, &out); status != 0 {
		t.Fatalf("pwd -P: status %d, output %q", status, out.String())
	}
	// Resolved on both sides, because a temporary directory here is reached
	// through a symlink and `pwd -P` answers with what the kernel calls it.
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("resolving %s: %v", dir, err)
	}
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("the line ran in %q, want %q", got, want)
	}
}

// `terminal/create` carries `env` entries the agent set — `CLAUDECODE=1` is
// the one it actually sends — and running the command it asked for includes
// the environment it asked for.
func TestInterpretLineTakesTheAgentsEnvironment(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	env := append(os.Environ(), "FROM_THE_AGENT=yes")
	if status := interpretLine(driver.Shell{Name: "sh"})(
		t.Context(), `echo "[$FROM_THE_AGENT]"`, "", env, &out); status != 0 {
		t.Fatalf("status %d, output %q", status, out.String())
	}
	if got := strings.TrimSpace(out.String()); got != "[yes]" {
		t.Errorf("the agent's variable did not reach the line: %q", got)
	}
}

// An `exec` inside the agent's line must not replace *this* process, which is
// the one serving the connection the agent is talking on. A shell binary
// wants the opposite, which is why interpretLine has to say so.
//
// If this regresses, the test binary is replaced mid-run and the whole package
// fails in a way no assertion here reports — so the surviving line below is
// itself the assertion.
func TestAnExecInTheAgentsLineDoesNotReplaceTheShell(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	status := interpretLine(driver.Shell{Name: "sh"})(
		t.Context(), "exec echo replaced", "", nil, &out)
	if status != 0 {
		t.Fatalf("status %d, output %q", status, out.String())
	}
	if got := strings.TrimSpace(out.String()); got != "replaced" {
		t.Errorf("exec'd command wrote %q, want %q", got, "replaced")
	}
	// Reached only because the process is still here.
	t.Log("the shell survived an exec in the agent's line")
}

// Cancelling the run ends the line, which is what release and a dropped
// connection do. There is no process to signal, so the context is the only
// handle — this is the driver.Shell.Context field earning its place.
func TestCancellingTheRunEndsTheLine(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan int, 1)
	go func() {
		var out bytes.Buffer
		done <- interpretLine(driver.Shell{Name: "sh"})(ctx, "sleep 30", "", nil, &out)
	}()
	// Let it get as far as starting the sleep before taking it away.
	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the line outlived its context, so nothing a caller does can end one")
	}
}
