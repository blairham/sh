// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/internal/jsonrpc"
	"github.com/blairham/sh/internal/mcp"
)

// The tests drive the server the way a client does — JSON-RPC over a real
// net.Pipe — rather than calling its handlers.
//
// That is the same rule internal/acp's tests follow and it is worth stating:
// the question about a wire format is what somebody else's program will
// understand, and a test that calls a handler directly is a test of a Go
// function that happens to be reachable over a wire.

// peer is a client on the other end. It records the progress notifications it
// was sent, which is the only thing this server ever says unprompted.
type peer struct {
	conn *jsonrpc.Conn

	mu       sync.Mutex
	progress []mcp.ProgressNotification
}

func (p *peer) Handle(_ context.Context, method string, _ json.RawMessage) (any, error) {
	// The revision says a server on stdio must not send requests at all. A
	// client that received one would be right to refuse it, and this is what
	// says so if one ever arrives.
	return nil, jsonrpc.Errorf(jsonrpc.CodeMethodNotFound, "a client serves no %s", method)
}

func (p *peer) Notify(_ context.Context, method string, params json.RawMessage) {
	if method != mcp.MethodProgress {
		return
	}
	var n mcp.ProgressNotification
	if err := json.Unmarshal(params, &n); err != nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.progress = append(p.progress, n)
}

func (p *peer) reports() []mcp.ProgressNotification {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]mcp.ProgressNotification(nil), p.progress...)
}

// connect stands a server up on one end of a pipe and a client on the other,
// in a directory of its own.
//
// inner is the shell's own gate — an embedder's sandbox, or `-policy` on the
// command line. One helper with the parameter rather than two, because a pair
// of them is how a fix goes into one route and not the other.
func connect(t *testing.T, inner interp.Gate) (*peer, string) {
	t.Helper()
	dir := t.TempDir()
	a, b := net.Pipe()
	server := mcp.NewServer(
		driver.Shell{Name: "sh", Dir: dir, Gate: inner},
		mcp.Implementation{Name: "sh", Version: "test"},
	)
	p := &peer{}
	p.conn = jsonrpc.NewConn(b, b, p)
	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = server.Serve(ctx, a, a) }()
	go func() { defer wg.Done(); _ = p.conn.Serve(ctx) }()
	t.Cleanup(func() {
		server.Close()
		cancel()
		_ = a.Close()
		_ = b.Close()
		wg.Wait()
	})
	return p, dir
}

// callTool is one tools/call, with no metadata on it. The result is decoded
// loosely so that a test can look at what a client would see rather than at
// what this package's own types say it should be.
func (p *peer) callTool(t *testing.T, name string, args any) mcp.CallToolResult {
	t.Helper()
	return p.callToolMeta(t, name, args, nil)
}

// callToolMeta is the same with request metadata, which is how a client asks
// for a protocol revision and how it asks to be sent progress.
func (p *peer) callToolMeta(t *testing.T, name string, args, m any) mcp.CallToolResult {
	t.Helper()
	params := map[string]any{"name": name}
	if args != nil {
		params["arguments"] = args
	}
	if m != nil {
		params["_meta"] = m
	}
	var out mcp.CallToolResult
	if err := p.conn.Call(t.Context(), mcp.MethodToolsCall, params, &out); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return out
}

// structured decodes a tool's structured content into v.
func structured(t *testing.T, res mcp.CallToolResult, v any) {
	t.Helper()
	if res.IsError {
		t.Fatalf("the tool answered an error: %s", text(res))
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func text(res mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		b.WriteString(c.Text)
	}
	return b.String()
}

// run is the whole of a command: start it, wait for it, read what it wrote.
// Three calls, which is the shape #1338 decided on.
func (p *peer) run(t *testing.T, line string) (string, int) {
	t.Helper()
	var made mcp.CreateResult
	structured(t, p.callTool(t, mcp.ToolCreate, map[string]any{"command": line}), &made)
	var status struct {
		ExitCode *int `json:"exitCode"`
	}
	structured(t, p.callTool(t, mcp.ToolWait, map[string]any{"terminalId": made.TerminalID}), &status)
	var out struct {
		Output string `json:"output"`
	}
	structured(t, p.callTool(t, mcp.ToolOutput, map[string]any{"terminalId": made.TerminalID}), &out)
	if status.ExitCode == nil {
		t.Fatal("the command ended with no exit code and no signal")
	}
	return out.Output, *status.ExitCode
}

// until reads a terminal's output until it holds what the test is waiting for.
//
// A bounded poll rather than a sleep, because what is being waited for is a
// command having written something and not a duration: a fixed sleep either
// flakes on a loaded machine or is slower than it needs to be on every run.
// The deadline is the test's own context, so a wait that is never satisfied
// fails the test rather than hanging the package.
func (p *peer) until(t *testing.T, id, want string) {
	t.Helper()
	for {
		var out struct {
			Output string `json:"output"`
		}
		structured(t, p.callTool(t, mcp.ToolOutput, map[string]any{"terminalId": id}), &out)
		if strings.Contains(out.Output, want) {
			return
		}
		select {
		case <-t.Context().Done():
			t.Fatalf("waited for %q and the terminal holds %q", want, out.Output)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// A command is a handle, and the handle is what the whole mapping rests on:
// create answers an id rather than the output, and the output is a later call.
func TestACommandIsAHandleAndItsOutputIsReadBackSeparately(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, nil)
	var made mcp.CreateResult
	res := p.callTool(t, mcp.ToolCreate, map[string]any{"command": "echo handle-ok"})
	structured(t, res, &made)
	if made.TerminalID == "" {
		t.Fatal("create answered no terminal id")
	}
	if strings.Contains(text(res), "handle-ok") {
		t.Errorf("create answered the command's output: %s\n"+
			"\tA create must answer a handle. Returning the output is the mapping\n"+
			"\t#1338 rejected — it is the answer that looks fine until somebody\n"+
			"\truns tail -f.", text(res))
	}
	var out struct {
		Output string `json:"output"`
	}
	p.callTool(t, mcp.ToolWait, map[string]any{"terminalId": made.TerminalID})
	structured(t, p.callTool(t, mcp.ToolOutput, map[string]any{"terminalId": made.TerminalID}), &out)
	if !strings.Contains(out.Output, "handle-ok") {
		t.Errorf("the terminal's output is %q, want it to contain handle-ok", out.Output)
	}
}

// A command with no args is a command *line*, run by this shell — so
// redirection, pipelines and variables all work, and every exec and open
// inside the line crosses the boundary rather than the line crossing it once
// as a filename.
func TestACommandWithNoArgsIsInterpretedAsAShellLine(t *testing.T) {
	t.Parallel()
	p, dir := connect(t, nil)
	file := filepath.Join(dir, "written")
	out, status := p.run(t, "x=through-the-shell; printf '%s' \"$x\" > "+file+"; echo done")
	if status != 0 {
		t.Fatalf("the line exited %d, output %q", status, out)
	}
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("the redirection produced no file: %v", err)
	}
	if string(b) != "through-the-shell" {
		t.Errorf("the file holds %q, want through-the-shell: the line was not interpreted", b)
	}
}

// And a command *with* args is a program and its arguments, with no shell
// between: the two readings are told apart by whether any args were sent,
// which is the rule the shared machinery states.
func TestACommandWithArgsIsExecedRatherThanInterpreted(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, nil)
	var made mcp.CreateResult
	structured(t, p.callTool(t, mcp.ToolCreate, map[string]any{
		// A shell would expand the redirection; an exec hands it to echo as a
		// word. Written this way so the two readings answer differently.
		"command": "/bin/echo",
		"args":    []string{"a", ">", "b"},
	}), &made)
	p.callTool(t, mcp.ToolWait, map[string]any{"terminalId": made.TerminalID})
	var out struct {
		Output string `json:"output"`
	}
	structured(t, p.callTool(t, mcp.ToolOutput, map[string]any{"terminalId": made.TerminalID}), &out)
	if strings.TrimSpace(out.Output) != "a > b" {
		t.Errorf("the argv route wrote %q, want %q: it went through a shell", out.Output, "a > b")
	}
}

// denyExec refuses one program and allows everything else, which is what a
// `-policy` naming an exec amounts to from this side.
type denyExec struct{ name string }

func (d denyExec) Allow(_ context.Context, a interp.Action) interp.Decision {
	if a.Kind == interp.ActionExec && strings.Contains(a.Path, d.name) {
		return interp.Deny
	}
	return interp.Allow
}

// The whole reason the flag exists: a policy on the invocation reaches a
// command a client asks for. #1334 is the issue — an agent runs `$SHELL -c`
// and there is nowhere to put a policy — and this route has somewhere.
func TestAPolicyOnTheInvocationRefusesACommandTheClientAsksFor(t *testing.T) {
	t.Parallel()
	p, dir := connect(t, denyExec{name: "touch"})
	made := filepath.Join(dir, "made-anyway")
	out, _ := p.run(t, "touch "+made+"; echo after")
	if _, err := os.Stat(made); err == nil {
		t.Errorf("the refused command ran anyway and made %s; output %q", made, out)
	}
	// And the same command with the policy off does make it, because a check
	// that has only ever seen a refusal cannot tell a working gate from a
	// shell that fell over.
	allowed, dir2 := connect(t, nil)
	should := filepath.Join(dir2, "made")
	if _, status := allowed.run(t, "touch "+should); status != 0 {
		t.Fatalf("the allowed command exited %d", status)
	}
	if _, err := os.Stat(should); err != nil {
		t.Errorf("the allowed command made nothing: %v", err)
	}
}

// A refusal is reported *in the result*, which is the revision's own line:
// something a model can act on is a tool execution error, not a JSON-RPC one.
func TestARefusedProgramIsAToolErrorRatherThanAProtocolError(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, denyExec{name: "echo"})
	res := p.callTool(t, mcp.ToolCreate, map[string]any{
		"command": "/bin/echo",
		"args":    []string{"hi"},
	})
	if !res.IsError {
		t.Fatalf("a refused create answered %v with isError unset", text(res))
	}
	if !strings.Contains(text(res), "permission denied") {
		t.Errorf("the refusal reads %q, want it to say permission denied", text(res))
	}
}

// The streaming decision, end to end: output reaches the client *while the
// command is still running*, as progress notifications on the wait that is in
// flight. Without it a client watching a long command sees nothing until it
// ends, which is the `tail -f` failure the mapping was chosen to avoid.
func TestALongCommandStreamsItsOutputAsProgressWhileItRuns(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, nil)
	var made mcp.CreateResult
	structured(t, p.callTool(t, mcp.ToolCreate, map[string]any{
		"command": "echo first; sleep 0.5; echo second",
	}), &made)
	p.callToolMeta(t,
		mcp.ToolWait,
		map[string]any{"terminalId": made.TerminalID},
		map[string]any{"progressToken": "tok-1"},
	)
	got := p.reports()
	if len(got) < 2 {
		t.Fatalf("the wait sent %d progress notifications, want at least two.\n"+
			"\tOne means everything arrived at the end, which is the string-at-the-end\n"+
			"\tmapping #1338 rejected wearing a notification. Sent: %v", len(got), got)
	}
	var all strings.Builder
	last := -1.0
	for _, n := range got {
		if string(n.ProgressToken) != `"tok-1"` {
			t.Errorf("a notification carried token %s, want the one the client sent", n.ProgressToken)
		}
		if n.Progress <= last {
			// The revision requires this in so many words, and a client that
			// tracks progress would read a repeat as a stall.
			t.Errorf("progress went %v then %v; it must increase with every notification", last, n.Progress)
		}
		last = n.Progress
		all.WriteString(n.Message)
	}
	if !strings.Contains(all.String(), "first") || !strings.Contains(all.String(), "second") {
		t.Errorf("the stream carried %q, want both lines", all.String())
	}
	// The stream is the whole of what was written and not a summary of it, so
	// a client that read only the notifications has the output.
	var out struct {
		Output string `json:"output"`
	}
	structured(t, p.callTool(t, mcp.ToolOutput, map[string]any{"terminalId": made.TerminalID}), &out)
	if all.String() != out.Output {
		t.Errorf("the stream said %q and the terminal holds %q; they must be the same bytes",
			all.String(), out.Output)
	}
}

// And nothing is sent to a client that did not ask, which is the other half:
// a progress notification may only reference a token the client supplied.
func TestAWaitWithNoProgressTokenSendsNothing(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, nil)
	var made mcp.CreateResult
	structured(t, p.callTool(t, mcp.ToolCreate, map[string]any{"command": "echo quiet"}), &made)
	p.callTool(t, mcp.ToolWait, map[string]any{"terminalId": made.TerminalID})
	if got := p.reports(); len(got) != 0 {
		t.Errorf("a wait with no token sent %d notifications: %v", len(got), got)
	}
}

// Killing ends the command and keeps the terminal, so what it wrote before it
// died is still there. Releasing forgets it, and the id stops meaning anything
// — which is the whole difference between the two verbs, and the reason only
// one of them is gated.
func TestKillKeepsTheOutputAndReleaseForgetsTheTerminal(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, nil)
	var made mcp.CreateResult
	structured(t, p.callTool(t, mcp.ToolCreate, map[string]any{
		// Long enough that the kill is what ends it. If this ever ends on its
		// own the assertion below about it being over would pass for the
		// wrong reason, so the marker is waited for rather than slept for.
		"command": "echo before-the-kill; sleep 30",
	}), &made)
	p.until(t, made.TerminalID, "before-the-kill")

	if res := p.callTool(t, mcp.ToolKill, map[string]any{"terminalId": made.TerminalID}); res.IsError {
		t.Fatalf("kill answered %v", text(res))
	}
	// A kill asks for the command to end; the wait is what says it has. The
	// two are separate verbs because they are separate facts, and a test that
	// read the output straight after the kill would be asserting on a race.
	p.callTool(t, mcp.ToolWait, map[string]any{"terminalId": made.TerminalID})
	var out struct {
		Output     string          `json:"output"`
		ExitStatus json.RawMessage `json:"exitStatus"`
	}
	structured(t, p.callTool(t, mcp.ToolOutput, map[string]any{"terminalId": made.TerminalID}), &out)
	if !strings.Contains(out.Output, "before-the-kill") {
		t.Errorf("a killed command's output reads %q; the terminal is meant to survive the kill", out.Output)
	}
	if len(out.ExitStatus) == 0 || string(out.ExitStatus) == "null" {
		t.Errorf("the killed command reports no exit status: %s", out.ExitStatus)
	}

	if res := p.callTool(t, mcp.ToolRelease, map[string]any{"terminalId": made.TerminalID}); res.IsError {
		t.Fatalf("release answered %v", text(res))
	}
	if res := p.callTool(t, mcp.ToolOutput, map[string]any{"terminalId": made.TerminalID}); !res.IsError {
		t.Errorf("the terminal still answers after release: %v", text(res))
	}
}

// An id this server never issued is a tool error rather than a protocol one,
// for the reason a refusal is: a model handed it can create a new terminal and
// carry on.
func TestAnUnknownTerminalIsAToolError(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, nil)
	for _, tool := range []string{mcp.ToolOutput, mcp.ToolWait, mcp.ToolKill, mcp.ToolRelease} {
		res := p.callTool(t, tool, map[string]any{"terminalId": "term-does-not-exist"})
		if !res.IsError {
			t.Errorf("%s answered %v for an id nobody issued, with isError unset", tool, text(res))
		}
	}
}

// A missing terminalId is the other kind of error: the call did not satisfy
// the schema the client was handed, which the revision keeps as a JSON-RPC
// error because a model is unlikely to fix it by retrying.
func TestACallThatDoesNotMatchTheSchemaIsAProtocolError(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, nil)
	var out mcp.CallToolResult
	err := p.conn.Call(t.Context(), mcp.MethodToolsCall, map[string]any{
		"name": mcp.ToolOutput, "arguments": map[string]any{},
	}, &out)
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) {
		t.Fatalf("a call with no terminalId answered %v, %v; want a JSON-RPC error", out, err)
	}
	if rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Errorf("the error code is %d, want %d", rpcErr.Code, jsonrpc.CodeInvalidParams)
	}
}

// An unknown tool is a protocol error too, and by name: the revision lists it
// as the worked example of one.
func TestAnUnknownToolIsAProtocolError(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, nil)
	err := p.conn.Call(t.Context(), mcp.MethodToolsCall, map[string]any{"name": "rm_rf"}, nil)
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("an unknown tool answered %v, want invalid params", err)
	}
}

// The output limit keeps the newest and says that it dropped something. A
// limit that silently truncated would be the silent-wrong-answer class: a
// model reading half a command's output with nothing saying so.
func TestAnOutputByteLimitKeepsTheNewestAndSaysItTruncated(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, nil)
	var made mcp.CreateResult
	structured(t, p.callTool(t, mcp.ToolCreate, map[string]any{
		"command":         "printf 'aaaaaaaaaabbbbbbbbbb'",
		"outputByteLimit": 10,
	}), &made)
	p.callTool(t, mcp.ToolWait, map[string]any{"terminalId": made.TerminalID})
	var out struct {
		Output    string `json:"output"`
		Truncated bool   `json:"truncated"`
	}
	structured(t, p.callTool(t, mcp.ToolOutput, map[string]any{"terminalId": made.TerminalID}), &out)
	if out.Output != "bbbbbbbbbb" {
		t.Errorf("the retained output is %q, want the newest ten bytes", out.Output)
	}
	if !out.Truncated {
		t.Error("output was dropped and truncated is false: a model would read half an answer as the whole of it")
	}
}
