// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp_test

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acp"
	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/interp"
)

// loopback runs this shell's client against this shell's agent, which is worth
// more than a mock in both directions: the client is graded against a real
// agent and the agent against a real client, and the two were written to the
// same schema rather than to each other.
func loopback(t *testing.T, c *acp.Client) (*acp.Client, string) {
	t.Helper()
	dir := t.TempDir()
	a, b := net.Pipe()
	agent := acp.NewAgent(driver.Shell{Name: "sh"}, acp.Implementation{Name: "sh", Version: "test"})
	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = agent.Serve(ctx, a, a) }()
	c.Connect(b, b)
	go func() { defer wg.Done(); _ = c.Serve(ctx) }()
	t.Cleanup(func() {
		agent.Close(context.Background())
		cancel()
		_ = a.Close()
		_ = b.Close()
		wg.Wait()
	})
	if _, err := c.Initialize(t.Context()); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	id, err := c.NewSession(t.Context(), dir)
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	return c, id
}

// collector gathers the text of the updates an agent sent, in order.
type collector struct {
	mu sync.Mutex
	b  strings.Builder
}

func (c *collector) update(n acp.SessionNotification) {
	raw, ok := n.Update.(json.RawMessage)
	if !ok {
		return
	}
	var u struct {
		SessionUpdate string           `json:"sessionUpdate"`
		Content       acp.ContentBlock `json:"content"`
	}
	if err := json.Unmarshal(raw, &u); err != nil {
		return
	}
	if u.SessionUpdate != acp.UpdateAgentMessageChunk {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.b.WriteString(u.Content.Text)
}

func (c *collector) text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.b.String()
}

// The whole loop: this shell drives this shell.
func TestAClientDrivesAnAgentEndToEnd(t *testing.T) {
	t.Parallel()
	var got collector
	c, id := loopback(t, &acp.Client{
		Info:   acp.Implementation{Name: "test-client", Version: "1"},
		Update: got.update,
		Answer: allowOnce,
	})

	stop, err := c.Prompt(t.Context(), id, "echo one; echo two")
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if stop != acp.StopEndTurn {
		t.Errorf("stopReason = %q", stop)
	}
	if want := "one\ntwo\n"; got.text() != want {
		t.Errorf("output = %q, want %q", got.text(), want)
	}
}

// A permission request the agent makes reaches whoever answers for the person,
// and their choice is what the agent acts on.
func TestAPermissionRequestReachesTheAnswerer(t *testing.T) {
	t.Parallel()
	var got collector
	var asked []acp.RequestPermissionRequest
	var mu sync.Mutex
	c, id := loopback(t, &acp.Client{
		Info:   acp.Implementation{Name: "test-client", Version: "1"},
		Update: got.update,
		Answer: func(_ context.Context, req acp.RequestPermissionRequest) (acp.PermissionOutcome, error) {
			mu.Lock()
			asked = append(asked, req)
			mu.Unlock()
			return acp.PermissionOutcome{
				Outcome: acp.OutcomeSelected, OptionID: acp.OptionRejectOnce,
			}, nil
		},
	})

	if _, err := c.Prompt(t.Context(), id, "/bin/echo ran-anyway"); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(asked) != 1 {
		t.Fatalf("%d permission requests reached the answerer, want 1", len(asked))
	}
	if len(asked[0].Options) != 4 {
		t.Errorf("%d options offered, want all four kinds", len(asked[0].Options))
	}
	// The word is not "refused": the shell's own diagnostic for a denied
	// exec contains that, and an assertion that matched it would pass
	// whether or not the command ran.
	if strings.Contains(got.text(), "ran-anyway") {
		t.Errorf("output = %q — the refusal did not stop the command", got.text())
	}
}

// Nobody to ask is a refusal, and it is a refusal the agent is told about
// rather than an error it has to interpret.
func TestAClientWithNobodyToAskRefuses(t *testing.T) {
	t.Parallel()
	var got collector
	c, id := loopback(t, &acp.Client{
		Info:   acp.Implementation{Name: "test-client", Version: "1"},
		Update: got.update,
	})

	if _, err := c.Prompt(t.Context(), id, "/bin/echo ran-anyway"); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if strings.Contains(got.text(), "ran-anyway") {
		t.Errorf("output = %q — a client with no answerer allowed the command", got.text())
	}
}

func allowOnce(context.Context, acp.RequestPermissionRequest) (acp.PermissionOutcome, error) {
	return acp.PermissionOutcome{Outcome: acp.OutcomeSelected, OptionID: acp.OptionAllowOnce}, nil
}

// agentSide is a stand-in agent, for the half of the protocol our own agent
// does not exercise: the requests an agent makes *of* a client.
type agentSide struct {
	conn *acp.Conn

	mu  sync.Mutex
	saw []json.RawMessage
	// answer, when set, is what the fake answers to every request rather
	// than the usual handshake.
	answer func(method string) (any, error)
}

func (a *agentSide) Handle(_ context.Context, method string, params json.RawMessage) (any, error) {
	a.mu.Lock()
	a.saw = append(a.saw, params)
	a.mu.Unlock()
	if a.answer != nil {
		return a.answer(method)
	}
	if method == acp.MethodInitialize {
		return acp.InitializeResponse{ProtocolVersion: acp.Version, AuthMethods: []acp.AuthMethod{}}, nil
	}
	return nil, acp.Errorf(acp.CodeMethodNotFound, "no %s", method)
}

func (a *agentSide) Notify(context.Context, string, json.RawMessage) {}

// against wires a client to a stand-in agent and returns both, without a
// handshake: the tests that use it are about what happens when the handshake
// or the first call does not go the usual way.
func against(t *testing.T, c *acp.Client, fake *agentSide) *acp.Conn {
	t.Helper()
	a, b := net.Pipe()
	fake.conn = acp.NewConn(a, a, fake)
	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = fake.conn.Serve(ctx) }()
	c.Connect(b, b)
	go func() { defer wg.Done(); _ = c.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = a.Close()
		_ = b.Close()
		wg.Wait()
	})
	return fake.conn
}

// A file the agent asks us to read is a file *we* open, through the gate.
func TestAFileTheAgentAsksForPassesTheGate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "read-me")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	seen := &recorder{}
	c := &acp.Client{
		Info:     acp.Implementation{Name: "test-client", Version: "1"},
		Files:    true,
		Boundary: boundary.Boundary{Gate: seen, Events: seen},
	}
	fake := &agentSide{}
	against(t, c, fake)

	var resp acp.ReadTextFileResponse
	if err := fake.conn.Call(t.Context(), acp.MethodReadTextFile,
		acp.ReadTextFileRequest{SessionID: "s1", Path: path}, &resp); err != nil {
		t.Fatalf("fs/read_text_file: %v", err)
	}
	if resp.Content != "one\ntwo\nthree\n" {
		t.Errorf("content = %q", resp.Content)
	}
	if !seen.asked1(interp.ActionOpen, path, false) {
		t.Errorf("the gate was not asked about the read: %v", seen.actions())
	}
}

// And a file the gate refuses is not read, and is refused the way a denied
// open is refused everywhere else — as a path that is not there.
func TestAFileTheGateRefusesIsNotRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("classified\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	seen := &recorder{deny: func(interp.Action) bool { return true }}
	c := &acp.Client{
		Info:     acp.Implementation{Name: "test-client", Version: "1"},
		Files:    true,
		Boundary: boundary.Boundary{Gate: seen, Events: seen},
	}
	fake := &agentSide{}
	against(t, c, fake)

	var resp acp.ReadTextFileResponse
	err := fake.conn.Call(t.Context(), acp.MethodReadTextFile,
		acp.ReadTextFileRequest{SessionID: "s1", Path: path}, &resp)
	if err == nil {
		t.Fatalf("the read succeeded and returned %q", resp.Content)
	}
	if strings.Contains(err.Error(), "classified") {
		t.Errorf("the refusal leaked the content: %v", err)
	}
}

// A write the gate refuses does not reach the file system.
func TestAWriteTheGateRefusesDoesNotHappen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "keep-out")
	seen := &recorder{deny: func(a interp.Action) bool { return a.Write }}
	c := &acp.Client{
		Info:     acp.Implementation{Name: "test-client", Version: "1"},
		Files:    true,
		Boundary: boundary.Boundary{Gate: seen, Events: seen},
	}
	fake := &agentSide{}
	against(t, c, fake)

	err := fake.conn.Call(t.Context(), acp.MethodWriteTextFile,
		acp.WriteTextFileRequest{SessionID: "s1", Path: path, Content: "x"}, nil)
	if err == nil {
		t.Fatal("the write was allowed")
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("the file was written despite the refusal")
	}
}

// A capability we did not advertise is a method we do not serve. Answering one
// we never claimed would tell the agent something untrue about what it can
// rely on.
func TestAnUnadvertisedCapabilityIsNotServed(t *testing.T) {
	t.Parallel()
	c := &acp.Client{Info: acp.Implementation{Name: "test-client", Version: "1"}}
	fake := &agentSide{}
	against(t, c, fake)

	err := fake.conn.Call(t.Context(), acp.MethodReadTextFile,
		acp.ReadTextFileRequest{SessionID: "s1", Path: "/etc/hosts"}, nil)
	if !acp.Unsupported(err) {
		t.Errorf("err = %v, want method not found", err)
	}
}

// The window an agent asks for, counted in lines from one.
func TestAReadCanAskForAWindow(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "lines")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\nfour\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &acp.Client{Info: acp.Implementation{Name: "test-client", Version: "1"}, Files: true}
	fake := &agentSide{}
	against(t, c, fake)

	line, limit := 2, 2
	var resp acp.ReadTextFileResponse
	if err := fake.conn.Call(t.Context(), acp.MethodReadTextFile,
		acp.ReadTextFileRequest{SessionID: "s1", Path: path, Line: &line, Limit: &limit}, &resp); err != nil {
		t.Fatalf("fs/read_text_file: %v", err)
	}
	if want := "two\nthree\n"; resp.Content != want {
		t.Errorf("content = %q, want %q", resp.Content, want)
	}
}

// Two answers a client must expect from a real agent and act on rather than
// fail: authentication required, which two of the three published agents send
// to the *first* thing tried after initialize, and method not found, which the
// third sends for every optional method.
func TestAuthRequiredAndUnsupportedAreRecognized(t *testing.T) {
	t.Parallel()
	c := &acp.Client{Info: acp.Implementation{Name: "test-client", Version: "1"}}
	fake := &agentSide{answer: func(method string) (any, error) {
		switch method {
		case acp.MethodInitialize:
			return acp.InitializeResponse{ProtocolVersion: acp.Version}, nil
		case acp.MethodNewSession:
			return nil, acp.Errorf(acp.CodeAuthRequired, "Authentication required")
		}
		return nil, acp.Errorf(acp.CodeMethodNotFound, "no %s", method)
	}}
	against(t, c, fake)

	if _, err := c.Initialize(t.Context()); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	_, err := c.NewSession(t.Context(), t.TempDir())
	if !acp.AuthRequired(err) {
		t.Errorf("err = %v, want an authentication-required error", err)
	}
	if acp.Unsupported(err) {
		t.Error("an authentication error was read as an unsupported method")
	}
	if err := c.Cancel("s1"); err != nil {
		t.Fatalf("session/cancel: %v", err)
	}
}

// An agent that answers a version we do not speak is a disconnection rather
// than an attempt: messages of an unknown shape are not worth putting on a
// wire.
func TestAVersionWeDoNotSpeakIsRefused(t *testing.T) {
	t.Parallel()
	c := &acp.Client{Info: acp.Implementation{Name: "test-client", Version: "1"}}
	fake := &agentSide{answer: func(string) (any, error) {
		return acp.InitializeResponse{ProtocolVersion: acp.Version + 7}, nil
	}}
	against(t, c, fake)

	if _, err := c.Initialize(t.Context()); err == nil {
		t.Error("a version this client does not speak was accepted")
	}
}

// recorder is a Gate and a Sink in one, with a rule about what to refuse.
//
// Guarded, because both are called from more than one goroutine: an agent may
// have several requests in flight and each is handled on its own.
type recorder struct {
	mu   sync.Mutex
	deny func(interp.Action) bool
	seen []interp.Action
	got  []interp.Event
}

func (r *recorder) Allow(_ context.Context, a interp.Action) interp.Decision {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, a)
	if r.deny != nil && r.deny(a) {
		return interp.Deny
	}
	return interp.Allow
}

func (r *recorder) Emit(_ context.Context, e interp.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, e)
}

func (r *recorder) actions() []interp.Action {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]interp.Action(nil), r.seen...)
}

// asked1 reports whether the gate was asked about exactly this access.
func (r *recorder) asked1(kind interp.ActionKind, path string, write bool) bool {
	for _, a := range r.actions() {
		if a.Kind == kind && a.Path == path && a.Write == write {
			return true
		}
	}
	return false
}

// params is what the agent was sent for the nth request it answered.
func (a *agentSide) params(n int) json.RawMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	if n >= len(a.saw) {
		return nil
	}
	return a.saw[n]
}

// A capability is only usable if the agent is told about it. The client's
// whole reason for serving the file methods is that an agent asks rather than
// opening for itself, and an agent that was never told cannot ask.
func TestTheFileCapabilityIsAdvertised(t *testing.T) {
	t.Parallel()
	for _, files := range []bool{true, false} {
		t.Run(map[bool]string{true: "offered", false: "withheld"}[files], func(t *testing.T) {
			t.Parallel()
			c := &acp.Client{Info: acp.Implementation{Name: "test-client", Version: "1"}, Files: files}
			fake := &agentSide{}
			against(t, c, fake)
			if _, err := c.Initialize(t.Context()); err != nil {
				t.Fatalf("initialize: %v", err)
			}
			var req struct {
				Capabilities acp.ClientCapabilities `json:"clientCapabilities"`
			}
			if err := json.Unmarshal(fake.params(0), &req); err != nil {
				t.Fatalf("the agent could not read what it was sent: %v", err)
			}
			if req.Capabilities.FS.ReadTextFile != files || req.Capabilities.FS.WriteTextFile != files {
				t.Errorf("fs capabilities = %+v, want both %v", req.Capabilities.FS, files)
			}
		})
	}
}
