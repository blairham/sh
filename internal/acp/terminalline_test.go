// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp_test

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blairham/sh/internal/acp"
	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/internal/jsonrpc"
)

// interpreter is a stand-in for the front end's shell: it records the line it
// was handed and answers with whatever the test wants.
type interpreter struct {
	mu     sync.Mutex
	lines  []string
	cwds   []string
	envs   [][]string
	write  string
	status int
	// block, when non-nil, holds the line until the context ends, which is
	// how a test asks whether cancelling reaches the run.
	block chan struct{}
	// returned, when non-nil, is closed once run has *returned*. A test that
	// waited for the line to be recorded instead would be waiting for the
	// moment before the one it cares about, and would pass whether or not
	// cancelling reached anything.
	returned chan struct{}
}

func (i *interpreter) run(ctx context.Context, line, cwd string, env []string, out io.Writer) int {
	if i.returned != nil {
		defer close(i.returned)
	}
	i.mu.Lock()
	i.lines = append(i.lines, line)
	i.cwds = append(i.cwds, cwd)
	i.envs = append(i.envs, env)
	write, status, block := i.write, i.status, i.block
	i.mu.Unlock()
	if write != "" {
		_, _ = io.WriteString(out, write)
	}
	if block != nil {
		select {
		case <-ctx.Done():
			// What a shell reports for a command a signal ended.
			return 130
		case <-block:
		}
	}
	return status
}

func (i *interpreter) seen() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]string(nil), i.lines...)
}

// withInterpreter is withTerminals plus a shell to interpret lines with.
func withInterpreter(t *testing.T, seen *recorder, sh *interpreter) (*jsonrpc.Conn, *acp.Client) {
	t.Helper()
	c := &acp.Client{
		Info:      acp.Implementation{Name: "test-client", Version: "1"},
		Terminals: true,
		Interpret: sh.run,
	}
	if seen != nil {
		c.Boundary = boundary.Boundary{Gate: seen, Events: seen, Session: "run-1"}
	}
	return against(t, c, &agentSide{}), c
}

func waitExit(t *testing.T, conn *jsonrpc.Conn, id string) acp.WaitForTerminalExitResponse {
	t.Helper()
	var exit acp.WaitForTerminalExitResponse
	if err := conn.Call(t.Context(), acp.MethodWaitForExit,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, &exit); err != nil {
		t.Fatalf("terminal/wait_for_exit: %v", err)
	}
	return exit
}

// The case from #1782, in the agent's own words.
//
// `claude-code-acp` 0.16.2 sends a shell line in `command` and no `args` at
// all, which says it expects the line interpreted rather than exec'd. It was
// exec'd as a filename, so nothing ran: four commands in one turn all failed
// the same way and the agent concluded the shell tool did not work.
func TestABareCommandIsAShellLineAndIsInterpreted(t *testing.T) {
	t.Parallel()
	sh := &interpreter{write: "argv0=sh\n"}
	conn, _ := withInterpreter(t, nil, sh)

	const asked = `printf "argv0=%s\n" "$0"; ps -o args= -p $$`
	id := create(t, conn, acp.CreateTerminalRequest{SessionID: "s1", Command: asked})
	waitExit(t, conn, id)

	got := sh.seen()
	if len(got) != 1 {
		t.Fatalf("the line reached the shell %d times, want once: %q", len(got), got)
	}
	if got[0] != asked {
		t.Errorf("the shell was handed %q, want the line as sent, %q.\n"+
			"A line taken apart on the way is a line the agent did not write.", got[0], asked)
	}

	var out acp.TerminalOutputResponse
	if err := conn.Call(t.Context(), acp.MethodTerminalOutput,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, &out); err != nil {
		t.Fatalf("terminal/output: %v", err)
	}
	if !strings.Contains(out.Output, "argv0=sh") {
		t.Errorf("the interpreted line's output did not come back: %q", out.Output)
	}
}

// The gate is not asked about the line as a filename, and that is the point
// rather than a gap: there is no program at this moment to ask about. What
// replaces it is every exec, open and stat the line goes on to make, each
// named as itself — which is strictly more than one question about a name
// that never existed.
func TestTheLineItselfIsNotGatedAsAProgram(t *testing.T) {
	t.Parallel()
	seen := &recorder{}
	sh := &interpreter{}
	conn, _ := withInterpreter(t, seen, sh)

	const asked = `echo hello`
	id := create(t, conn, acp.CreateTerminalRequest{SessionID: "s1", Command: asked})
	waitExit(t, conn, id)

	for _, a := range seen.actions() {
		if a.Kind == interp.ActionExec && a.Path == asked {
			t.Errorf("the gate was asked to exec %q as a program.\n"+
				"That is the reading of #1782: a filename that never existed, and the one "+
				"question a policy cannot usefully answer. The line's own commands are what "+
				"reach the gate now.", asked)
		}
	}
}

// An agent that sends `args` has built an argv and gets the argv reading,
// unchanged. Both readings stay available because the schema leaves the choice
// to the client and only the agent knows which it meant.
func TestAnArgvIsStillExecd(t *testing.T) {
	t.Parallel()
	seen := &recorder{}
	sh := &interpreter{}
	conn, _ := withInterpreter(t, seen, sh)

	id := create(t, conn, acp.CreateTerminalRequest{
		SessionID: "s1", Command: "/bin/echo", Args: []string{"from-the-agent"},
	})
	waitExit(t, conn, id)

	if got := sh.seen(); len(got) != 0 {
		t.Errorf("an argv was handed to the interpreter: %q.\n"+
			"`args` is how an agent says it built an argv, and exec is what it asked for.", got)
	}
	if !seen.asked1(interp.ActionExec, "/bin/echo", false) {
		t.Errorf("the gate was not asked to exec /bin/echo.\n" +
			"The argv reading is gated exactly as before; only the no-args reading changed.")
	}
}

// A client with no interpreter is a client that can only exec an argv, and it
// behaves exactly as it did before this existed. An embedder that serves
// terminals without supplying a shell is still entitled to the old reading.
func TestWithoutAnInterpreterABareCommandIsStillExecd(t *testing.T) {
	t.Parallel()
	seen := &recorder{}
	conn, _ := withTerminals(t, seen)

	id := create(t, conn, acp.CreateTerminalRequest{SessionID: "s1", Command: "/bin/echo"})
	waitExit(t, conn, id)

	if !seen.asked1(interp.ActionExec, "/bin/echo", false) {
		t.Errorf("the gate was not asked to exec /bin/echo, so the fallback is not the " +
			"behavior it replaced.")
	}
}

// Releasing a terminal ends an interpreted line, which is what it does to a
// child process. There is no process to signal here, so cancelling the run's
// context is what has to reach it — and it is why driver.Shell grew a Context.
func TestReleasingATerminalEndsAnInterpretedLine(t *testing.T) {
	t.Parallel()
	sh := &interpreter{block: make(chan struct{}), returned: make(chan struct{})}
	conn, _ := withInterpreter(t, nil, sh)

	id := create(t, conn, acp.CreateTerminalRequest{SessionID: "s1", Command: "sleep forever"})

	var rel acp.ReleaseTerminalResponse
	if err := conn.Call(t.Context(), acp.MethodReleaseTerminal,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, &rel); err != nil {
		t.Fatalf("terminal/release: %v", err)
	}
	// The line has to actually end. Without the context reaching it, this
	// blocks until the test's deadline rather than failing quickly.
	select {
	case <-sh.returned:
	// Generous for something that should be immediate, and bounded so the
	// failure is this message rather than the whole package timing out.
	case <-time.After(5 * time.Second):
		t.Fatal("the interpreted line outlived the terminal that was released.\n" +
			"Releasing has to reach the run, and cancelling its context is the only way " +
			"in: there is no process here to signal.")
	}
}
