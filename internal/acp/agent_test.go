// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp_test

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

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acp"

	"github.com/blairham/sh/internal/jsonrpc"
)

// A client, for testing an agent against. It records what it was told and
// answers what it was asked, which is the whole of a client's job.
type client struct {
	conn *jsonrpc.Conn

	mu      sync.Mutex
	updates []acp.SessionNotification
	raw     []map[string]any
	asked   []map[string]any

	// answer is the option id to choose. Empty means allow once.
	answer string
	// hold, when set, blocks a permission request until it is closed, so a
	// test can do something while a turn is stopped mid-action.
	hold chan struct{}
	// asking is closed the first time a permission request arrives.
	asking chan struct{}
	once   sync.Once
}

func (c *client) Handle(ctx context.Context, method string, params json.RawMessage) (any, error) {
	if method != acp.MethodRequestPermission {
		return nil, jsonrpc.Errorf(jsonrpc.CodeMethodNotFound, "client has no %s", method)
	}
	var raw map[string]any
	_ = json.Unmarshal(params, &raw)
	c.mu.Lock()
	c.asked = append(c.asked, raw)
	answer, hold := c.answer, c.hold
	c.mu.Unlock()
	c.once.Do(func() {
		if c.asking != nil {
			close(c.asking)
		}
	})
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if answer == "" {
		answer = acp.OptionAllowOnce
	}
	return acp.RequestPermissionResponse{Outcome: acp.PermissionOutcome{
		Outcome: acp.OutcomeSelected, OptionID: answer,
	}}, nil
}

func (c *client) Notify(_ context.Context, method string, params json.RawMessage) {
	if method != acp.MethodSessionUpdate {
		return
	}
	var n acp.SessionNotification
	if err := json.Unmarshal(params, &n); err != nil {
		return
	}
	var raw struct {
		Update map[string]any `json:"update"`
	}
	_ = json.Unmarshal(params, &raw)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updates = append(c.updates, n)
	c.raw = append(c.raw, raw.Update)
}

// text is everything the shell wrote on one stream, in order.
func (c *client) text(stream string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var b strings.Builder
	for _, u := range c.raw {
		if u["sessionUpdate"] != acp.UpdateAgentMessageChunk {
			continue
		}
		meta, _ := u["_meta"].(map[string]any)
		if meta[acp.MetaStream] != stream {
			continue
		}
		content, _ := u["content"].(map[string]any)
		s, _ := content["text"].(string)
		b.WriteString(s)
	}
	return b.String()
}

// calls is every tool call update, in order.
func (c *client) calls() []map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []map[string]any
	for _, u := range c.raw {
		if u["sessionUpdate"] == acp.UpdateToolCall || u["sessionUpdate"] == acp.UpdateToolCallUpdate {
			out = append(out, u)
		}
	}
	return out
}

func (c *client) questions() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.asked)
}

// connect starts an agent on one end of a pipe and a client on the other,
// initializes, and opens a session in a directory of its own.
func connect(t *testing.T, c *client) (*client, string, string) {
	t.Helper()
	if c == nil {
		c = &client{}
	}
	dir := t.TempDir()
	a, b := net.Pipe()
	agent := acp.NewAgent(
		driver.Shell{Name: "sh"},
		acp.Implementation{Name: "sh", Version: "test"},
	)
	c.conn = jsonrpc.NewConn(b, b, c)
	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = agent.Serve(ctx, a, a) }()
	go func() { defer wg.Done(); _ = c.conn.Serve(ctx) }()
	t.Cleanup(func() {
		agent.Close(context.Background())
		cancel()
		_ = a.Close()
		_ = b.Close()
		wg.Wait()
	})

	var init acp.InitializeResponse
	if err := c.conn.Call(t.Context(), acp.MethodInitialize, acp.InitializeRequest{
		ProtocolVersion: acp.Version,
		ClientInfo:      &acp.Implementation{Name: "test-client", Version: "1"},
	}, &init); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var made acp.NewSessionResponse
	if err := c.conn.Call(t.Context(), acp.MethodNewSession, acp.NewSessionRequest{Cwd: dir}, &made); err != nil {
		t.Fatalf("session/new: %v", err)
	}
	return c, made.SessionID, dir
}

// prompt runs one turn and returns its stop reason.
func prompt(t *testing.T, c *client, id, src string) string {
	t.Helper()
	var resp acp.PromptResponse
	if err := c.conn.Call(t.Context(), acp.MethodPrompt, acp.PromptRequest{
		SessionID: id, Prompt: []acp.ContentBlock{acp.TextBlock(src)},
	}, &resp); err != nil {
		t.Fatalf("session/prompt: %v", err)
	}
	return resp.StopReason
}

func TestInitializeAnswersTheVersionAndWhatIsClaimed(t *testing.T) {
	t.Parallel()
	a, b := net.Pipe()
	agent := acp.NewAgent(driver.Shell{Name: "sh"}, acp.Implementation{Name: "sh", Version: "test"})
	conn := jsonrpc.NewConn(b, b, nil)
	go func() { _ = agent.Serve(t.Context(), a, a) }()
	go func() { _ = conn.Serve(t.Context()) }()
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })

	// Decoded loosely as well as into the type, because what matters here is
	// the JSON a client actually reads: authMethods must be an empty array
	// and not null.
	var raw map[string]any
	if err := conn.Call(t.Context(), acp.MethodInitialize,
		acp.InitializeRequest{ProtocolVersion: acp.Version}, &raw); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if raw["protocolVersion"] != float64(acp.Version) {
		t.Errorf("protocolVersion = %v, want %d", raw["protocolVersion"], acp.Version)
	}
	methods, ok := raw["authMethods"].([]any)
	if !ok || len(methods) != 0 {
		t.Errorf("authMethods = %v, want an empty array", raw["authMethods"])
	}
	caps, _ := raw["agentCapabilities"].(map[string]any)
	if caps["loadSession"] != false {
		t.Errorf("loadSession = %v, want false", caps["loadSession"])
	}
}

// The handshake is the first thing that happens. Nothing else can be answered
// before a version has been agreed.
func TestAMethodBeforeInitializeIsRefused(t *testing.T) {
	t.Parallel()
	a, b := net.Pipe()
	agent := acp.NewAgent(driver.Shell{Name: "sh"}, acp.Implementation{Name: "sh", Version: "test"})
	conn := jsonrpc.NewConn(b, b, nil)
	go func() { _ = agent.Serve(t.Context(), a, a) }()
	go func() { _ = conn.Serve(t.Context()) }()
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })

	err := conn.Call(t.Context(), acp.MethodNewSession, acp.NewSessionRequest{Cwd: t.TempDir()}, nil)
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidRequest {
		t.Errorf("err = %v, want an invalid-request error", err)
	}
}

// The whole thing, end to end: a client sends shell, and what the shell wrote
// comes back as content.
func TestAPromptRunsShellAndItsOutputComesBack(t *testing.T) {
	t.Parallel()
	c, id, _ := connect(t, nil)

	if got := prompt(t, c, id, "echo hello"); got != acp.StopEndTurn {
		t.Errorf("stopReason = %q, want %q", got, acp.StopEndTurn)
	}
	if got := c.text(acp.StreamStdout); got != "hello\n" {
		t.Errorf("stdout = %q, want %q", got, "hello\n")
	}
}

// The two streams are told apart, which v1 has no update kind for and _meta is
// the reserved place for.
func TestTheTwoStreamsAreToldApart(t *testing.T) {
	t.Parallel()
	c, id, _ := connect(t, nil)

	prompt(t, c, id, "echo out; echo err >&2")
	if got := c.text(acp.StreamStdout); got != "out\n" {
		t.Errorf("stdout = %q", got)
	}
	if got := c.text(acp.StreamStderr); got != "err\n" {
		t.Errorf("stderr = %q", got)
	}
}

// A session is a shell rather than a series of unrelated commands.
func TestASessionKeepsItsShellBetweenTurns(t *testing.T) {
	t.Parallel()
	c, id, _ := connect(t, nil)

	prompt(t, c, id, "x=kept")
	prompt(t, c, id, "echo $x")
	if got := c.text(acp.StreamStdout); got != "kept\n" {
		t.Errorf("stdout = %q, want the variable to have survived the turn", got)
	}
}

// The session runs where the client said, and two sessions on one connection
// do not share a directory — which is what a process-wide chdir would have
// made impossible.
func TestASessionRunsInTheDirectoryTheClientNamed(t *testing.T) {
	t.Parallel()
	c, id, dir := connect(t, nil)

	prompt(t, c, id, "echo written > made-here")
	if _, err := os.ReadFile(filepath.Join(dir, "made-here")); err != nil {
		t.Fatalf("the redirect did not land in the session's directory: %v", err)
	}
	prompt(t, c, id, "pwd")
	if got := strings.TrimSpace(c.text(acp.StreamStdout)); got != dir {
		t.Errorf("pwd = %q, want %q", got, dir)
	}
}

// Standard input is empty and, crucially, is not the protocol stream. A `read`
// that consumed the connection would eat the client's next message.
func TestAReadGetsEndOfFileRatherThanTheConnection(t *testing.T) {
	t.Parallel()
	c, id, _ := connect(t, nil)

	prompt(t, c, id, "read x; echo \"[$x]\"")
	if got := c.text(acp.StreamStdout); got != "[]\n" {
		t.Errorf("stdout = %q, want an empty read", got)
	}
	// And the connection is still usable, which is the half that would fail
	// if the read had taken a message off it.
	if got := prompt(t, c, id, "echo after"); got != acp.StopEndTurn {
		t.Errorf("the second turn answered %q", got)
	}
}

// A command is put to the client before it runs, as a tool call and a
// permission request naming it.
func TestAnExecIsAskedAboutAndReported(t *testing.T) {
	t.Parallel()
	c, id, _ := connect(t, nil)

	prompt(t, c, id, "/bin/echo external")
	if c.questions() != 1 {
		t.Fatalf("%d permission requests, want 1", c.questions())
	}
	if got := c.text(acp.StreamStdout); got != "external\n" {
		t.Errorf("stdout = %q, want the command to have run", got)
	}
	calls := c.calls()
	if len(calls) == 0 {
		t.Fatal("no tool calls were reported")
	}
	first := calls[0]
	if first["kind"] != acp.KindExecute {
		t.Errorf("kind = %v, want %q", first["kind"], acp.KindExecute)
	}
	if title, _ := first["title"].(string); !strings.Contains(title, "/bin/echo") {
		t.Errorf("title = %q, want it to name the command", title)
	}
	// The permission request must name the tool call it is about, or a
	// client has nothing to show a person.
	asked, _ := c.asked[0]["toolCall"].(map[string]any)
	if asked["toolCallId"] != first["toolCallId"] {
		t.Errorf("asked about %v, announced %v", asked["toolCallId"], first["toolCallId"])
	}
	// And the same tool call is the one that completes, rather than a second
	// one appearing beside it.
	var completed bool
	for _, u := range calls {
		if u["toolCallId"] == first["toolCallId"] && u["status"] == acp.StatusCompleted {
			completed = true
		}
	}
	if !completed {
		t.Errorf("the announced tool call never completed: %v", calls)
	}
}

// A refusal reaches the client as a failed tool call, and the command does not
// run.
func TestARefusedCommandDoesNotRunAndIsReported(t *testing.T) {
	t.Parallel()
	c, id, _ := connect(t, &client{answer: acp.OptionRejectOnce})

	prompt(t, c, id, "/bin/echo external")
	if got := c.text(acp.StreamStdout); got != "" {
		t.Errorf("stdout = %q, want nothing — the command was refused", got)
	}
	var failed bool
	for _, u := range c.calls() {
		if u["status"] == acp.StatusFailed {
			failed = true
		}
	}
	if !failed {
		t.Errorf("no failed tool call reached the client: %v", c.calls())
	}
}

// `allow always` settles the question for the session.
func TestAllowAlwaysIsAskedOnce(t *testing.T) {
	t.Parallel()
	c, id, _ := connect(t, &client{answer: acp.OptionAllowAlways})

	prompt(t, c, id, "/bin/echo one")
	prompt(t, c, id, "/bin/echo one")
	if c.questions() != 1 {
		t.Errorf("%d permission requests, want 1", c.questions())
	}
	if got := c.text(acp.StreamStdout); got != "one\none\n" {
		t.Errorf("stdout = %q, want both runs", got)
	}
}

// A stat is not a question. One PATH search makes many of them, and a client
// shown one prompt per candidate is a client showing nothing.
func TestAStatIsNotAskedAbout(t *testing.T) {
	t.Parallel()
	c, id, _ := connect(t, nil)

	prompt(t, c, id, "[ -f /etc/hosts ] && echo yes")
	if c.questions() != 0 {
		t.Errorf("%d permission requests for a file test, want none", c.questions())
	}
	if got := c.text(acp.StreamStdout); got != "yes\n" {
		t.Errorf("stdout = %q", got)
	}
}

// A method a shell does not serve is method-not-found rather than an error
// with a code a client cannot act on.
func TestAnUnservedMethodIsMethodNotFound(t *testing.T) {
	t.Parallel()
	c, _, _ := connect(t, nil)

	for _, m := range []string{"session/load", "authenticate", "session/set_mode"} {
		err := c.conn.Call(t.Context(), m, map[string]any{}, nil)
		var rpcErr *jsonrpc.Error
		if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeMethodNotFound {
			t.Errorf("%s: err = %v, want method not found", m, err)
		}
	}
}

// A second turn while one is running is refused rather than interleaved. One
// shell runs one program at a time, and a caller waiting for a turn that has
// not started is owed an answer rather than a delay.
func TestASecondTurnIsRefusedWhileOneIsRunning(t *testing.T) {
	t.Parallel()
	held := make(chan struct{})
	asking := make(chan struct{})
	c, id, _ := connect(t, &client{hold: held, asking: asking})

	done := make(chan string, 1)
	go func() { done <- prompt(t, c, id, "/bin/echo slow") }()
	<-asking

	err := c.conn.Call(t.Context(), acp.MethodPrompt, acp.PromptRequest{
		SessionID: id, Prompt: []acp.ContentBlock{acp.TextBlock("echo second")},
	}, nil)
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Errorf("err = %v, want the second turn refused", err)
	}
	close(held)
	if got := <-done; got != acp.StopEndTurn {
		t.Errorf("the first turn answered %q", got)
	}
}

// session/cancel stops the turn, and the turn says so.
func TestCancelEndsTheTurn(t *testing.T) {
	t.Parallel()
	held := make(chan struct{})
	asking := make(chan struct{})
	c, id, _ := connect(t, &client{hold: held, asking: asking})
	t.Cleanup(func() { close(held) })

	done := make(chan string, 1)
	go func() { done <- prompt(t, c, id, "/bin/echo slow") }()
	<-asking

	if err := c.conn.Notify(acp.MethodCancel, acp.CancelNotification{SessionID: id}); err != nil {
		t.Fatalf("session/cancel: %v", err)
	}
	if got := <-done; got != acp.StopCancelled {
		t.Errorf("stopReason = %q, want %q", got, acp.StopCancelled)
	}
}

// A prompt for a session that does not exist is a parameter error rather than
// a crash or a silent success.
func TestAPromptForAnUnknownSessionIsRefused(t *testing.T) {
	t.Parallel()
	c, _, _ := connect(t, nil)

	err := c.conn.Call(t.Context(), acp.MethodPrompt, acp.PromptRequest{
		SessionID: "sess-nope", Prompt: []acp.ContentBlock{acp.TextBlock("echo hi")},
	}, nil)
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Errorf("err = %v, want an invalid-params error", err)
	}
}

// Non-text blocks are tolerated rather than refused: the baseline requires an
// agent to accept a resource link and there is nothing a shell does with one.
func TestANonTextBlockIsSkippedRatherThanRefused(t *testing.T) {
	t.Parallel()
	c, id, _ := connect(t, nil)

	var resp acp.PromptResponse
	err := c.conn.Call(t.Context(), acp.MethodPrompt, acp.PromptRequest{
		SessionID: id,
		Prompt: []acp.ContentBlock{
			{Type: acp.ContentResourceLink, URI: "file:///tmp/x", Name: "x"},
			acp.TextBlock("echo kept"),
		},
	}, &resp)
	if err != nil {
		t.Fatalf("session/prompt: %v", err)
	}
	if resp.StopReason != acp.StopEndTurn {
		t.Errorf("stopReason = %q", resp.StopReason)
	}
	if got := c.text(acp.StreamStdout); got != "kept\n" {
		t.Errorf("stdout = %q, want the text block to have run", got)
	}
}

// Output arrives as characters rather than as bytes. A multi-byte character
// split across two writes would otherwise reach the client as mojibake, since
// the encoder replaces invalid UTF-8 rather than carrying it.
func TestMultiByteOutputSurvivesChunking(t *testing.T) {
	t.Parallel()
	c, id, _ := connect(t, nil)

	const word = "café ☕ 日本語"
	prompt(t, c, id, "printf '%s' '"+word+"'")
	if got := c.text(acp.StreamStdout); got != word {
		t.Errorf("stdout = %q, want %q", got, word)
	}
}

// A file the shell reads is reported and is *not* asked about. That pair is
// the whole of the escalation decision: an access a person cannot usefully be
// asked about at the rate it happens is still an access worth showing them.
func TestAReadOpenIsReportedWithoutBeingAskedAbout(t *testing.T) {
	t.Parallel()
	c, id, dir := connect(t, nil)
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("data\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	prompt(t, c, id, "read line < f; echo \"[$line]\"")
	if got := c.text(acp.StreamStdout); got != "[data]\n" {
		t.Errorf("stdout = %q, want the file to have been read", got)
	}
	if c.questions() != 0 {
		t.Errorf("%d permission requests for a read, want none", c.questions())
	}
	var found map[string]any
	for _, u := range c.calls() {
		if u["kind"] == acp.KindRead {
			found = u
		}
	}
	if found == nil {
		t.Fatalf("the read was not reported to the client: %v", c.calls())
	}
	if title, _ := found["title"].(string); !strings.Contains(title, "f") {
		t.Errorf("title = %q, want it to name the file", title)
	}
}

// A command that ran and failed is a failed tool call carrying its status, and
// the turn around it still ended normally: a failing command is a turn that
// completed, not a turn that stopped.
func TestAFailingCommandIsAFailedToolCallAndAnOrdinaryTurn(t *testing.T) {
	t.Parallel()
	c, id, _ := connect(t, nil)

	if got := prompt(t, c, id, "/usr/bin/false"); got != acp.StopEndTurn {
		t.Errorf("stopReason = %q, want %q", got, acp.StopEndTurn)
	}
	var end map[string]any
	for _, u := range c.calls() {
		if _, ok := u["rawOutput"]; ok {
			end = u
		}
	}
	if end == nil {
		t.Fatalf("no tool call reported an outcome: %v", c.calls())
	}
	if end["status"] != acp.StatusFailed {
		t.Errorf("status = %v, want %q", end["status"], acp.StatusFailed)
	}
	raw, _ := end["rawOutput"].(map[string]any)
	if raw["exitStatus"] != float64(1) {
		t.Errorf("exitStatus = %v, want 1", raw["exitStatus"])
	}
}
