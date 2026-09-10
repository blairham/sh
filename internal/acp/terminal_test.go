// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/acp"
	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/internal/jsonrpc"
)

// withTerminals wires a client that serves terminal/* to a stand-in agent, and
// returns the agent's end so a test can make the calls an agent would.
func withTerminals(t *testing.T, seen *recorder) (*jsonrpc.Conn, *acp.Client) {
	t.Helper()
	c := &acp.Client{
		Info:      acp.Implementation{Name: "test-client", Version: "1"},
		Terminals: true,
	}
	if seen != nil {
		c.Boundary = boundary.Boundary{Gate: seen, Events: seen, Session: "run-1"}
	}
	fake := &agentSide{}
	return against(t, c, fake), c
}

// create runs a command through the client and returns its terminal id.
func create(t *testing.T, conn *jsonrpc.Conn, req acp.CreateTerminalRequest) string {
	t.Helper()
	var resp acp.CreateTerminalResponse
	if err := conn.Call(t.Context(), acp.MethodCreateTerminal, req, &resp); err != nil {
		t.Fatalf("terminal/create: %v", err)
	}
	return resp.TerminalID
}

// A command the agent asks us to run is a command *we* start, so the gate sees
// the argv. This is the whole reason the capability is served: an agent that
// runs it for itself is an agent no policy here can see at all.
func TestACommandTheAgentAsksForPassesTheGate(t *testing.T) {
	t.Parallel()
	seen := &recorder{}
	conn, _ := withTerminals(t, seen)

	id := create(t, conn, acp.CreateTerminalRequest{
		SessionID: "s1", Command: "/bin/echo", Args: []string{"from-the-agent"},
	})
	var exit acp.WaitForTerminalExitResponse
	if err := conn.Call(t.Context(), acp.MethodWaitForExit,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, &exit); err != nil {
		t.Fatalf("terminal/wait_for_exit: %v", err)
	}
	if exit.ExitCode == nil || *exit.ExitCode != 0 {
		t.Errorf("exit = %+v, want code 0", exit)
	}
	var out acp.TerminalOutputResponse
	if err := conn.Call(t.Context(), acp.MethodTerminalOutput,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, &out); err != nil {
		t.Fatalf("terminal/output: %v", err)
	}
	if !strings.Contains(out.Output, "from-the-agent") {
		t.Errorf("output = %q, want what the command wrote", out.Output)
	}

	var ran bool
	for _, a := range seen.actions() {
		if a.Kind == interp.ActionExec && a.Path == "/bin/echo" {
			ran = true
			if len(a.Args) != 2 || a.Args[0] != "/bin/echo" || a.Args[1] != "from-the-agent" {
				t.Errorf("argv = %v, want the whole vector, argv[0] included", a.Args)
			}
			if a.ID == "" {
				t.Error("the gate was consulted about an action with no identity")
			}
		}
	}
	if !ran {
		t.Errorf("the gate was never asked about the command: %v", seen.actions())
	}
	// And the record of it names the same action and the run it belongs to,
	// which is what makes a permission decision and its consequences provably
	// one thing rather than two entries that look alike.
	var joined bool
	for _, e := range seen.events() {
		if e.Action.Kind == interp.ActionExec && e.Action.ID != "" && e.Session == "run-1" {
			joined = true
		}
	}
	if !joined {
		t.Errorf("no record of the command names both the action and the run: %v", seen.events())
	}
}

// A refused create is a command that never started, and the agent is told so
// rather than handed a terminal id naming nothing.
func TestACommandTheGateRefusesNeverRuns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	seen := &recorder{deny: func(a interp.Action) bool { return a.Kind == interp.ActionExec }}
	conn, _ := withTerminals(t, seen)

	var resp acp.CreateTerminalResponse
	err := conn.Call(t.Context(), acp.MethodCreateTerminal, acp.CreateTerminalRequest{
		SessionID: "s1", Command: "/usr/bin/touch", Args: []string{marker},
	}, &resp)
	if err == nil {
		t.Fatalf("the command was allowed, as terminal %q", resp.TerminalID)
	}
	if resp.TerminalID != "" {
		t.Errorf("a terminal id was handed out for a refused command: %q", resp.TerminalID)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Error("the command ran despite the refusal")
	}
}

// Killing is the agent reaching a running process it chose to reach, which is
// the signal case exactly, so the gate decides it — and a refusal leaves the
// command running.
func TestKillingATerminalPassesTheGate(t *testing.T) {
	t.Parallel()
	seen := &recorder{deny: func(a interp.Action) bool { return a.Kind == interp.ActionSignal }}
	conn, _ := withTerminals(t, seen)

	id := create(t, conn, acp.CreateTerminalRequest{
		SessionID: "s1", Command: "/bin/sleep", Args: []string{"30"},
	})
	err := conn.Call(t.Context(), acp.MethodKillTerminal,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, nil)
	if err == nil {
		t.Fatal("the kill was allowed by a gate that refuses signals")
	}
	var asked bool
	for _, a := range seen.actions() {
		if a.Kind == interp.ActionSignal {
			asked = true
		}
	}
	if !asked {
		t.Errorf("the gate was not asked about the signal: %v", seen.actions())
	}
	// Released rather than left running, so the test does not leave a sleep
	// behind — and release is the method that must work when kill was refused.
	if err := conn.Call(t.Context(), acp.MethodReleaseTerminal,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, nil); err != nil {
		t.Fatalf("terminal/release: %v", err)
	}
}

// Release is the protocol's only way to say "I am finished with this", so it
// works even where the gate refuses every signal — and it is still recorded,
// because a process was ended.
func TestReleaseIsRecordedRatherThanRefused(t *testing.T) {
	t.Parallel()
	seen := &recorder{deny: func(a interp.Action) bool { return a.Kind == interp.ActionSignal }}
	conn, _ := withTerminals(t, seen)

	id := create(t, conn, acp.CreateTerminalRequest{
		SessionID: "s1", Command: "/bin/sleep", Args: []string{"30"},
	})
	if err := conn.Call(t.Context(), acp.MethodReleaseTerminal,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, nil); err != nil {
		t.Fatalf("terminal/release: %v", err)
	}
	var recorded bool
	for _, e := range seen.events() {
		if e.Kind != interp.EventAccess || e.Action.Kind != interp.ActionSignal {
			continue
		}
		recorded = true
		// The record has to be joinable, which is the whole reason the two
		// fields exist. A release naming no run and no action would be a line
		// in the audit trail that nothing lines up with — and it is not a
		// hypothetical: emitting this beside the boundary rather than through
		// it produced exactly that.
		if e.Action.ID == "" {
			t.Error("the release's record names no action")
		}
		if e.Session != "run-1" {
			t.Errorf("the release's record names run %q, want the front end's", e.Session)
		}
	}
	if !recorded {
		t.Errorf("the release ended a process and left no record: %v", seen.events())
	}
	// And the terminal is gone: an id that has been released names nothing.
	if err := conn.Call(t.Context(), acp.MethodTerminalOutput,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, nil); err == nil {
		t.Error("a released terminal still answered for its output")
	}
}

// The output limit drops the oldest and says so, and the cut lands on a
// character boundary — the protocol requires it in so many words, and an agent
// handed half a rune has been handed something that is not text.
func TestOutputIsTruncatedAtACharacterBoundary(t *testing.T) {
	t.Parallel()
	conn, _ := withTerminals(t, nil)

	// Twenty three-byte runes, and a limit that falls inside one of them.
	limit := 20
	id := create(t, conn, acp.CreateTerminalRequest{
		SessionID: "s1", Command: "/bin/echo", Args: []string{strings.Repeat("é", 20)},
		OutputByteLimit: &limit,
	})
	if err := conn.Call(t.Context(), acp.MethodWaitForExit,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, nil); err != nil {
		t.Fatalf("terminal/wait_for_exit: %v", err)
	}
	var out acp.TerminalOutputResponse
	if err := conn.Call(t.Context(), acp.MethodTerminalOutput,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, &out); err != nil {
		t.Fatalf("terminal/output: %v", err)
	}
	if !out.Truncated {
		t.Errorf("output = %q, want it reported as truncated", out.Output)
	}
	if len(out.Output) > limit {
		t.Errorf("output is %d bytes, over the limit of %d", len(out.Output), limit)
	}
	// Slightly under the limit is the price of the boundary rule, and a string
	// that is not valid UTF-8 is the failure it prevents.
	if !utf8Valid(out.Output) {
		t.Errorf("output = %q, want a valid string", out.Output)
	}
}

func utf8Valid(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

// A command that ends on a signal reports the signal and no exit code, which
// are not the same answer: an exit code of zero is not the absence of one.
func TestASignaledCommandReportsTheSignal(t *testing.T) {
	t.Parallel()
	conn, _ := withTerminals(t, nil)

	id := create(t, conn, acp.CreateTerminalRequest{
		SessionID: "s1", Command: "/bin/sleep", Args: []string{"30"},
	})
	if err := conn.Call(t.Context(), acp.MethodKillTerminal,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, nil); err != nil {
		t.Fatalf("terminal/kill: %v", err)
	}
	var exit acp.WaitForTerminalExitResponse
	if err := conn.Call(t.Context(), acp.MethodWaitForExit,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, &exit); err != nil {
		t.Fatalf("terminal/wait_for_exit: %v", err)
	}
	if exit.Signal == nil || *exit.Signal != "SIGKILL" {
		t.Errorf("signal = %v, want SIGKILL", exit.Signal)
	}
	if exit.ExitCode != nil {
		t.Errorf("exitCode = %v, want none beside a signal", *exit.ExitCode)
	}
}

// A terminal id this client never issued is invalid params rather than a
// terminal: the agent asked about something that does not exist.
func TestATerminalWeNeverMadeIsNotFound(t *testing.T) {
	t.Parallel()
	conn, _ := withTerminals(t, nil)
	for _, method := range []string{
		acp.MethodTerminalOutput, acp.MethodWaitForExit,
		acp.MethodKillTerminal, acp.MethodReleaseTerminal,
	} {
		if err := conn.Call(t.Context(), method,
			acp.TerminalRequest{SessionID: "s1", TerminalID: "term-99"}, nil); err == nil {
			t.Errorf("%s answered for a terminal that was never made", method)
		}
	}
}

// The capability has to be advertised or an agent cannot use it, and it must
// not be served when it was not: a client answering a method it never claimed
// tells the agent something untrue about what it can rely on.
func TestTheTerminalCapabilityIsAdvertisedAndGated(t *testing.T) {
	t.Parallel()
	for _, on := range []bool{true, false} {
		t.Run(map[bool]string{true: "offered", false: "withheld"}[on], func(t *testing.T) {
			t.Parallel()
			c := &acp.Client{Info: acp.Implementation{Name: "test-client", Version: "1"}, Terminals: on}
			fake := &agentSide{}
			conn := against(t, c, fake)
			if _, err := c.Initialize(t.Context()); err != nil {
				t.Fatalf("initialize: %v", err)
			}
			var req struct {
				Capabilities acp.ClientCapabilities `json:"clientCapabilities"`
			}
			if err := json.Unmarshal(fake.params(0), &req); err != nil {
				t.Fatalf("the agent could not read what it was sent: %v", err)
			}
			if req.Capabilities.Terminal != on {
				t.Errorf("terminal capability = %v, want %v", req.Capabilities.Terminal, on)
			}
			err := conn.Call(t.Context(), acp.MethodCreateTerminal,
				acp.CreateTerminalRequest{SessionID: "s1", Command: "/bin/echo"}, nil)
			if on {
				if err != nil {
					t.Errorf("terminal/create: %v", err)
				}
			} else if !acp.Unsupported(err) {
				t.Errorf("err = %v, want method not found for a capability never claimed", err)
			}
		})
	}
}

// The environment is inherited and then written over. A command started with
// only the agent's few variables has no PATH, so every create would fail for a
// reason nothing on the wire explains.
func TestATerminalInheritsTheEnvironmentAndTakesTheAgentsOverIt(t *testing.T) {
	// Not parallel: t.Setenv changes the process, which is the only way to ask
	// what a child inherits from it.
	t.Setenv("SH_ACP_INHERITED", "yes")
	conn, _ := withTerminals(t, nil)

	id := create(t, conn, acp.CreateTerminalRequest{
		SessionID: "s1", Command: "/bin/sh",
		Args: []string{"-c", `printf '%s %s' "$SH_ACP_INHERITED" "$SH_ACP_SET"`},
		Env:  []acp.EnvVariable{{Name: "SH_ACP_SET", Value: "also"}},
	})
	if err := conn.Call(t.Context(), acp.MethodWaitForExit,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, nil); err != nil {
		t.Fatalf("terminal/wait_for_exit: %v", err)
	}
	var out acp.TerminalOutputResponse
	if err := conn.Call(t.Context(), acp.MethodTerminalOutput,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, &out); err != nil {
		t.Fatalf("terminal/output: %v", err)
	}
	if out.Output != "yes also" {
		t.Errorf("output = %q, want both the inherited variable and the agent's", out.Output)
	}
}

// Both of the command's streams are one, because a terminal has one: an agent
// asking for output is asking what a person would have seen on a screen.
func TestATerminalCarriesBothStreams(t *testing.T) {
	t.Parallel()
	conn, _ := withTerminals(t, nil)

	id := create(t, conn, acp.CreateTerminalRequest{
		SessionID: "s1", Command: "/bin/sh",
		Args: []string{"-c", `printf out; printf err >&2`},
	})
	if err := conn.Call(t.Context(), acp.MethodWaitForExit,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, nil); err != nil {
		t.Fatalf("terminal/wait_for_exit: %v", err)
	}
	var out acp.TerminalOutputResponse
	if err := conn.Call(t.Context(), acp.MethodTerminalOutput,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, &out); err != nil {
		t.Fatalf("terminal/output: %v", err)
	}
	if !strings.Contains(out.Output, "out") || !strings.Contains(out.Output, "err") {
		t.Errorf("output = %q, want both streams", out.Output)
	}
}

// A working directory the agent named is where the command runs.
func TestATerminalRunsWhereTheAgentAsked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	conn, _ := withTerminals(t, nil)

	id := create(t, conn, acp.CreateTerminalRequest{
		SessionID: "s1", Command: "/bin/pwd", Cwd: dir,
	})
	if err := conn.Call(t.Context(), acp.MethodWaitForExit,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, nil); err != nil {
		t.Fatalf("terminal/wait_for_exit: %v", err)
	}
	var out acp.TerminalOutputResponse
	if err := conn.Call(t.Context(), acp.MethodTerminalOutput,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, &out); err != nil {
		t.Fatalf("terminal/output: %v", err)
	}
	// The temporary directory may be reached through a symbolic link, so the
	// resolved path is what pwd prints and what this compares.
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.Output) != want {
		t.Errorf("pwd = %q, want %q", strings.TrimSpace(out.Output), want)
	}
}

// interpreting wires a client whose Interpret seam is the given func, which is
// what a caller holding a shell supplies. The client also has a gate, so a test
// can show which route an action took.
func interpreting(t *testing.T, seen *recorder,
	run func(context.Context, acp.TerminalCommand, io.Writer) int,
) *jsonrpc.Conn {
	t.Helper()
	c := &acp.Client{
		Info:      acp.Implementation{Name: "test-client", Version: "1"},
		Terminals: true,
		Interpret: run,
	}
	if seen != nil {
		c.Boundary = boundary.Boundary{Gate: seen, Events: seen, Session: "run-1"}
	}
	return against(t, c, &agentSide{})
}

// An agent that sends a command line and no args gets it interpreted.
//
// This is #1782. Every agent measured that means a shell command sends the
// whole line in `command` and no `args` at all — Claude Code 0.16.2 sends
// `printf "%s" "$0"; ps -o args= -p $$` that way — and exec'ing that as a
// filename failed every one of them with "executable file not found in $PATH",
// including a bare `echo hello`.
func TestACommandLineIsInterpretedRatherThanExeced(t *testing.T) {
	t.Parallel()
	var got acp.TerminalCommand
	conn := interpreting(t, nil, func(_ context.Context, cmd acp.TerminalCommand, out io.Writer) int {
		got = cmd
		_, _ = io.WriteString(out, "interpreted\n")
		return 3
	})

	id := create(t, conn, acp.CreateTerminalRequest{
		SessionID: "s1",
		Command:   `echo one; echo two`,
		Cwd:       "/tmp",
		Env:       []acp.EnvVariable{{Name: "FROM_AGENT", Value: "1"}},
	})
	var exit acp.WaitForTerminalExitResponse
	if err := conn.Call(t.Context(), acp.MethodWaitForExit,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, &exit); err != nil {
		t.Fatalf("terminal/wait_for_exit: %v", err)
	}
	if exit.ExitCode == nil || *exit.ExitCode != 3 {
		t.Errorf("exit = %+v, want the status the interpreter returned", exit)
	}
	var out acp.TerminalOutputResponse
	if err := conn.Call(t.Context(), acp.MethodTerminalOutput,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, &out); err != nil {
		t.Fatalf("terminal/output: %v", err)
	}
	if out.Output != "interpreted\n" {
		t.Errorf("output = %q, want what the interpreter wrote", out.Output)
	}
	if got.Line != `echo one; echo two` {
		t.Errorf("line = %q, want the command whole, syntax included", got.Line)
	}
	if got.Dir != "/tmp" {
		t.Errorf("dir = %q, want the directory the agent asked for", got.Dir)
	}
	// Inherited and then written over: a line started with only the agent's
	// few variables has no PATH.
	var carried, inherited bool
	for _, e := range got.Env {
		if e == "FROM_AGENT=1" {
			carried = true
		}
		if strings.HasPrefix(e, "PATH=") {
			inherited = true
		}
	}
	if !carried || !inherited {
		t.Errorf("env carried the agent's = %v, inherited a PATH = %v", carried, inherited)
	}
}

// An argv is still an argv. An agent that says which word is the program is
// taken at its word, and the gate sees that vector exactly as before.
func TestAnArgvIsStillExecedWhenTheAgentSendsOne(t *testing.T) {
	t.Parallel()
	seen := &recorder{}
	var interpreted bool
	conn := interpreting(t, seen, func(context.Context, acp.TerminalCommand, io.Writer) int {
		interpreted = true
		return 0
	})

	id := create(t, conn, acp.CreateTerminalRequest{
		SessionID: "s1", Command: "/bin/echo", Args: []string{"argv-route"},
	})
	var exit acp.WaitForTerminalExitResponse
	if err := conn.Call(t.Context(), acp.MethodWaitForExit,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, &exit); err != nil {
		t.Fatalf("terminal/wait_for_exit: %v", err)
	}
	if interpreted {
		t.Error("a command sent with args was interpreted as a line")
	}
	var gated bool
	for _, a := range seen.actions() {
		if a.Kind == interp.ActionExec && a.Path == "/bin/echo" {
			gated = true
		}
	}
	if !gated {
		t.Errorf("the gate never saw the argv: %v", seen.actions())
	}
}

// terminal/kill on an interpreted line has no process to signal, so it cancels
// the context the interpreter is running under — and that has to actually
// reach it, or an agent's kill is a no-op that answers as though it worked.
func TestKillingAnInterpretedLineCancelsIt(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	conn := interpreting(t, &recorder{}, func(ctx context.Context, _ acp.TerminalCommand, _ io.Writer) int {
		close(started)
		<-ctx.Done()
		return 130
	})

	id := create(t, conn, acp.CreateTerminalRequest{SessionID: "s1", Command: "sleep forever"})
	<-started
	var killed acp.KillTerminalResponse
	if err := conn.Call(t.Context(), acp.MethodKillTerminal,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, &killed); err != nil {
		t.Fatalf("terminal/kill: %v", err)
	}
	var exit acp.WaitForTerminalExitResponse
	if err := conn.Call(t.Context(), acp.MethodWaitForExit,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, &exit); err != nil {
		t.Fatalf("terminal/wait_for_exit: %v", err)
	}
	if exit.ExitCode == nil || *exit.ExitCode != 130 {
		t.Errorf("exit = %+v, want the status the canceled interpreter returned", exit)
	}
}

// Releasing an interpreted line does not record a SIGKILL to pid 0. It ended a
// context, not a process, and the commands inside the line recorded themselves
// as they ran.
func TestReleasingAnInterpretedLineRecordsNoSignal(t *testing.T) {
	t.Parallel()
	seen := &recorder{}
	conn := interpreting(t, seen, func(context.Context, acp.TerminalCommand, io.Writer) int { return 0 })

	id := create(t, conn, acp.CreateTerminalRequest{SessionID: "s1", Command: "true"})
	var exit acp.WaitForTerminalExitResponse
	if err := conn.Call(t.Context(), acp.MethodWaitForExit,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, &exit); err != nil {
		t.Fatalf("terminal/wait_for_exit: %v", err)
	}
	var released acp.ReleaseTerminalResponse
	if err := conn.Call(t.Context(), acp.MethodReleaseTerminal,
		acp.TerminalRequest{SessionID: "s1", TerminalID: id}, &released); err != nil {
		t.Fatalf("terminal/release: %v", err)
	}
	for _, e := range seen.events() {
		if e.Action.Kind == interp.ActionSignal {
			t.Errorf("released an interpreted line and recorded a signal: %+v", e.Action)
		}
	}
}
