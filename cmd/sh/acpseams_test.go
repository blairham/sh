// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/internal/acp"
	"github.com/blairham/sh/internal/jsonrpc"
)

// `sh -acp` beside the flags that reach the gate and the event sink, through
// the invocation a person types.
//
// The bug this file exists for (#1335) was not in the permission model — the
// ordering rule has been unit-tested in internal/acp since it was written —
// but in the wiring: the session assigned its own gate *over* the one the
// invocation had installed, so `-policy` was accepted and meant nothing. A
// unit test of a gate cannot see that, and neither can a test of the flags: it
// takes the whole invocation, which is what sandbox_test.go says about the
// other routes and what this says about this one. `-audit` was assigned over
// in the same two lines and is here for the same reason.
//
// The assertion that matters is not that the refused command failed. It is
// that **nobody was asked**. A client answering reject would make a broken
// shell look correct, and the client here answers allow-once to everything —
// deliberately the most permissive answer there is, because a policy that only
// holds while the person on the other end is careful is not a policy. On this
// route the other end may not be a person at all: an agent harness driving
// `sh -acp` answers its own permission requests, and one configured to
// auto-approve is exactly this client.

// acpClient is a client on the other end of `-acp`: it answers every
// permission request with allow-once and records what it was asked and what
// the session sent back.
type acpClient struct {
	conn *jsonrpc.Conn

	mu     sync.Mutex
	asked  []acp.RequestPermissionRequest
	stdout strings.Builder
	stderr strings.Builder
}

func (c *acpClient) Handle(_ context.Context, method string, params json.RawMessage) (any, error) {
	if method != acp.MethodRequestPermission {
		return nil, jsonrpc.Errorf(jsonrpc.CodeMethodNotFound, "client has no %s", method)
	}
	var req acp.RequestPermissionRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "%v", err)
	}
	c.mu.Lock()
	c.asked = append(c.asked, req)
	c.mu.Unlock()
	return acp.RequestPermissionResponse{Outcome: acp.PermissionOutcome{
		Outcome: acp.OutcomeSelected, OptionID: acp.OptionAllowOnce,
	}}, nil
}

func (c *acpClient) Notify(_ context.Context, method string, params json.RawMessage) {
	if method != acp.MethodSessionUpdate {
		return
	}
	var raw struct {
		Update map[string]any `json:"update"`
	}
	if err := json.Unmarshal(params, &raw); err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if raw.Update["sessionUpdate"] != acp.UpdateAgentMessageChunk {
		return
	}
	content, _ := raw.Update["content"].(map[string]any)
	text, _ := content["text"].(string)
	meta, _ := raw.Update["_meta"].(map[string]any)
	if meta[acp.MetaStream] == acp.StreamStderr {
		c.stderr.WriteString(text)
		return
	}
	c.stdout.WriteString(text)
}

// questions is every permission request the shell put to this client.
func (c *acpClient) questions() []acp.RequestPermissionRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]acp.RequestPermissionRequest(nil), c.asked...)
}

func (c *acpClient) out() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stdout.String()
}

func (c *acpClient) errs() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stderr.String()
}

// served runs `sh -acp …` through run(), with this test on the other end of
// its standard input and output, and returns the client after one prompt.
//
// Through run() rather than through a shell assembled here, for the reason
// sandbox_test.go gives: the mistake being guarded against is wiring that
// exists and is not reached, and a helper that reassembled the invocation
// would grade the helper.
//
// One prompt and then the input closes, which is how a client says it is
// finished and how the server's Serve returns.
func served(t *testing.T, dir, src string, args ...string) *acpClient {
	t.Helper()
	toShell, fromTest := io.Pipe()
	fromShell, toTest := io.Pipe()
	c := &acpClient{}
	c.conn = jsonrpc.NewConn(fromShell, fromTest, c)

	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	code := -1
	wg.Add(2)
	go func() {
		defer wg.Done()
		// The client's own reader ends when ours does, which is how its
		// Serve returns and how the wait below finishes.
		defer func() { _ = toTest.Close() }()
		argv := append([]string{"sh", "-dialect", "bash", "-acp"}, args...)
		// The shell's own standard error is the process's on this route and
		// nothing is asserted about it; what a session writes comes back as
		// updates.
		code = run(argv, toShell, toTest, io.Discard)
	}()
	go func() { defer wg.Done(); _ = c.conn.Serve(ctx) }()
	t.Cleanup(func() {
		// Closing the *writing* end is the client saying it is finished, and
		// it is what a client that exits does. Closing the shell's reader
		// instead would end the run with a read error on a torn-down pipe,
		// which is a different shutdown and would hide the status below.
		_ = fromTest.Close()
		wg.Wait()
		cancel()
		_ = toShell.Close()
		_ = fromShell.Close()
		if code != 0 {
			t.Errorf("sh -acp exited %d, want a clean end when the client closes its input", code)
		}
	})

	// One call at a time, each waiting for its answer: the server reads the
	// handshake and the session in order, and a burst would have session/new
	// answered "initialize first" by a server that had read both before
	// finishing the first.
	var init acp.InitializeResponse
	if err := c.conn.Call(ctx, acp.MethodInitialize, acp.InitializeRequest{
		ProtocolVersion: acp.Version,
		ClientInfo:      &acp.Implementation{Name: "test-client", Version: "1"},
	}, &init); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var made acp.NewSessionResponse
	if err := c.conn.Call(ctx, acp.MethodNewSession, acp.NewSessionRequest{Cwd: dir}, &made); err != nil {
		t.Fatalf("session/new: %v", err)
	}
	var resp acp.PromptResponse
	if err := c.conn.Call(ctx, acp.MethodPrompt, acp.PromptRequest{
		SessionID: made.SessionID, Prompt: []acp.ContentBlock{acp.TextBlock(src)},
	}, &resp); err != nil {
		t.Fatalf("session/prompt: %v", err)
	}
	return c
}

// tripwire is a command whose only job is to have run: it is external, so it
// is an exec the gate sees, and it says so on standard output where a session
// update carries it back.
const tripwire = "/bin/echo tripwire"

// TestAPolicyDeniedExecIsNeverPutToTheClient is the whole of #1335.
//
// Three runs of one command, with the policy as the only thing that differs,
// because the tell in the report was not that curl was refused — it was that
// permission was *requested at all*, whether or not a policy was present,
// which is what "the policy is not shaping the question" means. Holding the
// command fixed and varying only the policy is the only arrangement that can
// tell those apart.
//
// The two allowing runs are the controls, and they must both ask: a fix that
// stopped escalating would pass a test that only checked the denied run, and
// would have quietly turned `-acp` into a shell that runs anything.
func TestAPolicyDeniedExecIsNeverPutToTheClient(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	denies := writePolicy(t, "default allow", "deny exec /bin/echo")
	allows := writePolicy(t, "default allow")

	t.Run("denied by the policy", func(t *testing.T) {
		t.Parallel()
		c := served(t, dir, tripwire, "-policy", denies)
		if q := c.questions(); len(q) != 0 {
			t.Errorf("the client was asked %d time(s) about an action the policy refuses: %v\n"+
				"\tA policy refusal is not negotiable. Asking makes it advisory, and on this\n"+
				"\troute the answer may come from a harness that approves everything.", len(q), q)
		}
		if strings.Contains(c.out(), "tripwire") {
			t.Errorf("stdout = %q, want the refused command not to have run", c.out())
		}
		if !strings.Contains(c.errs(), "refused") {
			t.Errorf("stderr = %q, want the refusal reported to the client", c.errs())
		}
	})

	t.Run("allowed by the policy", func(t *testing.T) {
		t.Parallel()
		c := served(t, dir, tripwire, "-policy", allows)
		if q := c.questions(); len(q) != 1 {
			t.Errorf("the client was asked %d time(s), want exactly once: a policy that\n"+
				"\tpermits an exec hands the question on, it does not answer it", len(q))
		}
		if !strings.Contains(c.out(), "tripwire") {
			t.Errorf("stdout = %q, want the allowed command to have run", c.out())
		}
	})

	t.Run("no policy at all", func(t *testing.T) {
		t.Parallel()
		c := served(t, dir, tripwire)
		if q := c.questions(); len(q) != 1 {
			t.Errorf("the client was asked %d time(s), want exactly once with no policy", len(q))
		}
		if !strings.Contains(c.out(), "tripwire") {
			t.Errorf("stdout = %q, want the command to have run", c.out())
		}
	})
}

// TestAPolicyDeniedWriteIsNeverPutToTheClient holds the other variable the
// exec case fixes: which kind of action it is.
//
// A write open is the second member of the escalation set, and it reaches the
// gate by a different route in the interpreter — a redirection rather than a
// command resolution. A composition that only covered exec would leave the
// file system negotiable while the process table was not, which is the same
// bug with a smaller blast radius.
func TestAPolicyDeniedWriteIsNeverPutToTheClient(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "f")
	p := writePolicy(t, "default allow", "deny write "+dir+"/**")

	c := served(t, dir, "echo tripwire > "+target, "-policy", p)
	if q := c.questions(); len(q) != 0 {
		t.Errorf("the client was asked %d time(s) about a write the policy refuses: %v", len(q), q)
	}
	if _, err := os.Stat(target); err == nil {
		t.Errorf("%s exists: the refused write happened", target)
	}

	// The control, in the same shape: with the rule gone the write is asked
	// about and then made. Without it this test would pass against a shell
	// that cannot redirect at all.
	allowed := served(t, dir, "echo tripwire > "+target, "-policy", writePolicy(t, "default allow"))
	if q := allowed.questions(); len(q) != 1 {
		t.Errorf("the client was asked %d time(s) about an allowed write, want once", len(q))
	}
	body, err := os.ReadFile(target)
	if err != nil || !strings.Contains(string(body), "tripwire") {
		t.Errorf("%s = %q, %v; want the allowed write to have happened", target, body, err)
	}
}

// TestAnAuditedACPSessionStillWritesTheRecord is the sibling half of #1335: the
// session assigned its sink over the invocation's exactly as it assigned its
// gate, so `sh -acp -audit f` created the file and wrote nothing to it.
//
// An audit trail a front end can silence is not an audit trail — the same
// sentence cmd/sh already writes about a plugin's observer in watched(), and
// the same mistake one level down.
func TestAnAuditedACPSessionStillWritesTheRecord(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	log := filepath.Join(dir, "audit.jsonl")

	c := served(t, dir, tripwire, "-audit", log)
	// The client is still told, which is the half that must not be traded for
	// the other: a record kept instead of the connection would be the same
	// bug pointed the other way.
	if q := c.questions(); len(q) != 1 {
		t.Errorf("the client was asked %d time(s), want once", len(q))
	}

	body, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	var execs []string
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		if line == "" {
			continue
		}
		var r struct {
			V      int    `json:"v"`
			Event  string `json:"event"`
			Action string `json:"action"`
			Path   string `json:"path"`
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("record %q did not decode: %v", line, err)
		}
		if r.V != 1 {
			t.Errorf("record %q: v = %d, want 1", line, r.V)
		}
		if r.Action == "exec" {
			execs = append(execs, r.Event+" "+r.Path)
		}
	}
	if !slices.Contains(execs, "command-start /bin/echo") {
		t.Errorf("the exec records are %q, want the command a session ran written down", execs)
	}
}
