// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/internal/jsonrpc"
	"github.com/blairham/sh/internal/mcp"
)

// `sh -mcp` beside the flags that reach the gate, through run() rather than
// through a shell assembled here.
//
// The mistake being guarded against is wiring that exists and is not reached:
// `-acp -policy p` accepted the policy and ignored it for as long as the route
// opened its own streams and no test could be on the other end of them
// (#1335). This route is testable from the day it lands, and what it is tested
// for is the one claim #1338 rests on — a policy on the invocation reaches a
// command a coding agent asks for, where `$SHELL -c` has nowhere to put one.

// mcpPeer is a client on the other end of the shell's standard streams.
type mcpPeer struct{ conn *jsonrpc.Conn }

func (p *mcpPeer) Handle(_ context.Context, method string, _ json.RawMessage) (any, error) {
	return nil, jsonrpc.Errorf(jsonrpc.CodeMethodNotFound, "a client serves no %s", method)
}
func (p *mcpPeer) Notify(context.Context, string, json.RawMessage) {}

// speaking runs `sh -mcp …` through run(), with this test on the other end.
//
// One connection, closed by the client, which is how a client that exits ends
// a stdio server and how Serve returns.
func speaking(t *testing.T, args ...string) *mcpPeer {
	t.Helper()
	toShell, fromTest := io.Pipe()
	fromShell, toTest := io.Pipe()
	p := &mcpPeer{}
	p.conn = jsonrpc.NewConn(fromShell, fromTest, p)

	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	code := -1
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer func() { _ = toTest.Close() }()
		argv := append([]string{"sh", "-dialect", "bash", "-mcp"}, args...)
		code = run(argv, toShell, toTest, io.Discard)
	}()
	go func() { defer wg.Done(); _ = p.conn.Serve(ctx) }()
	t.Cleanup(func() {
		_ = fromTest.Close()
		wg.Wait()
		cancel()
		_ = toShell.Close()
		_ = fromShell.Close()
		if code != 0 {
			t.Errorf("sh -mcp exited %d, want a clean end when the client closes its input", code)
		}
	})
	return p
}

// call is one tools/call, answered as the client sees it.
func (p *mcpPeer) call(t *testing.T, name string, args map[string]any) mcp.CallToolResult {
	t.Helper()
	var out mcp.CallToolResult
	if err := p.conn.Call(t.Context(), mcp.MethodToolsCall, map[string]any{
		"name": name, "arguments": args,
	}, &out); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return out
}

// runs one command line to completion and answers what it wrote.
func (p *mcpPeer) runLine(t *testing.T, line string) string {
	t.Helper()
	var made struct {
		TerminalID string `json:"terminalId"`
	}
	res := p.call(t, mcp.ToolCreate, map[string]any{"command": line})
	if res.IsError {
		t.Fatalf("create: %v", res.Content)
	}
	b, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(b, &made); err != nil {
		t.Fatal(err)
	}
	p.call(t, mcp.ToolWait, map[string]any{"terminalId": made.TerminalID})
	var out struct {
		Output string `json:"output"`
	}
	got := p.call(t, mcp.ToolOutput, map[string]any{"terminalId": made.TerminalID})
	b, _ = json.Marshal(got.StructuredContent)
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out.Output
}

// The claim, in three runs.
//
// One run cannot decide it, and that is the discipline `make sandbox` is built
// on: a route reporting "refused" from a shell that never started looks
// exactly like a boundary that works. So the same write is attempted with no
// policy, with a `-deny` that forbids it, and with no policy again — and only
// all three agreeing says the flag reached the gate.
func TestAPolicyOnTheInvocationReachesACommandAClientAsksFor(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "written")

	ungated := speaking(t)
	ungated.runLine(t, "echo ok > "+target)
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("the ungated run wrote nothing, so the other two measure nothing: %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}

	denied := speaking(t, "-deny", dir)
	out := denied.runLine(t, "echo ok > "+target)
	if _, err := os.Stat(target); err == nil {
		t.Errorf("the refused write happened anyway; output %q", out)
	}
	if !strings.Contains(out, "refused") {
		t.Errorf("the shell said %q, want the refusal reported to the client", out)
	}

	// The refusal reached the *commands inside a line*, which is the stronger
	// half and the reason a line is interpreted rather than exec'd: an argv
	// level gate would have seen one program, and this saw the redirection.
	allowed := speaking(t)
	allowed.runLine(t, "echo ok > "+target)
	if _, err := os.Stat(target); err != nil {
		t.Errorf("with the policy gone the write was still refused: %v", err)
	}
}

// And the operand rule, because a word left on the line is somebody expecting
// something to run and there is nothing here to run it.
func TestMCPTakesNoOperands(t *testing.T) {
	var out strings.Builder
	if code := run([]string{"sh", "-mcp", "script.sh"}, strings.NewReader(""), io.Discard, &out); code == 0 {
		t.Error("sh -mcp with an operand exited 0")
	}
}
