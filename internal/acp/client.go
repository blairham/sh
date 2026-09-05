// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/interp"
)

// The shell as an ACP Client: it launches a coding agent and *is* the
// environment that agent runs inside.
//
// This is the direction where the seam does something no editor can. An agent
// running under an editor reads files with its own open(2) and nobody sees it.
// An agent running under this shell can be told to ask instead — fs/read_text_file
// and fs/write_text_file are advertised precisely so that it does — and every
// one of those is an interp.Action through the same gate, into the same audit
// trail, as a redirection a script writes. What the agent does with its own
// descriptors is still its own; what it asks us for is ours.
//
// Nothing below the dispatch is client-specific. The framing is a peer, the
// message shapes are one party writing what the other reads, and turning a
// permission option into a decision is the same function it is on the agent
// side — read the other way round, because here somebody else is asking.

// Client drives an agent.
type Client struct {
	// Boundary is the gate and the sink the agent's requests pass through.
	// The zero value allows everything and records nothing, which is what a
	// shell without a policy is.
	Boundary boundary.Boundary

	// Answer is who decides a permission request the agent makes. It is
	// about something the agent will do in its own process, which our
	// boundary does not cover, so this is a person rather than a policy.
	//
	// Nil refuses every request, for the reason the gate refuses when there
	// is nobody to ask: a question that cannot be put has not been answered.
	Answer func(ctx context.Context, req RequestPermissionRequest) (PermissionOutcome, error)

	// Update receives the agent's session updates, in the order they were
	// sent — which is their meaning, since the chunks of what it wrote are
	// only what it wrote in sequence. It must not block; it holds the
	// connection while it runs.
	Update func(SessionNotification)

	// Info is what this client calls itself to the agent.
	Info Implementation

	// Files says whether to advertise the file system capability. On is the
	// point of the exercise; off is for a caller that wants the agent to do
	// its own reading and accepts that nothing will see it.
	Files bool

	conn  *Conn
	agent InitializeResponse
}

// Connect wires this client to an agent's streams.
//
// Separate from Serve, and not for tidiness: Serve blocks, so a caller runs it
// on a goroutine, and a client that built its connection *inside* Serve would
// hand every caller a race between starting that goroutine and making the
// first request. Connect happens on the caller's own goroutine, before either.
func (c *Client) Connect(r io.Reader, w io.Writer) { c.conn = NewConn(r, w, c) }

// Serve reads the agent's output until it ends.
//
// It must be running before any request below is made: each of them waits for
// an answer, and nothing answers until something is reading.
func (c *Client) Serve(ctx context.Context) error {
	if c.conn == nil {
		return errors.New("acp: the client is not connected")
	}
	return c.conn.Serve(ctx)
}

// Handle answers what the agent asks of us.
func (c *Client) Handle(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case MethodRequestPermission:
		return c.permission(ctx, params)
	case MethodReadTextFile:
		return c.readFile(ctx, params)
	case MethodWriteTextFile:
		return c.writeFile(ctx, params)
	}
	// Everything else is a capability we did not advertise. A client that
	// answered a method it never claimed would be telling the agent
	// something untrue about what it can rely on.
	return nil, Errorf(CodeMethodNotFound, "this client does not serve %s", method)
}

// Notify takes the agent's session updates.
func (c *Client) Notify(_ context.Context, method string, params json.RawMessage) {
	if method != MethodSessionUpdate || c.Update == nil {
		return
	}
	var n SessionNotification
	if err := json.Unmarshal(params, &n); err != nil {
		return
	}
	// Decoded loosely, because the update union is wider than the part of it
	// this shell knows and an unknown kind must not be dropped: rule four of
	// the event contract, read across a protocol instead of a log.
	var raw struct {
		Update json.RawMessage `json:"update"`
	}
	if err := json.Unmarshal(params, &raw); err == nil {
		n.Update = raw.Update
	}
	c.Update(n)
}

// Initialize performs the handshake and returns what the agent claims.
func (c *Client) Initialize(ctx context.Context) (InitializeResponse, error) {
	info := c.Info
	req := InitializeRequest{ProtocolVersion: Version, ClientInfo: &info}
	req.Capabilities.FS = FileSystemCapabilities{ReadTextFile: c.Files, WriteTextFile: c.Files}
	var resp InitializeResponse
	if err := c.conn.Call(ctx, MethodInitialize, req, &resp); err != nil {
		return resp, err
	}
	if resp.ProtocolVersion != Version {
		// The agent answered with a version it will speak and we do not.
		// Disconnecting is what the protocol says to do, and pretending
		// otherwise would put messages of an unknown shape on the wire.
		return resp, Errorf(CodeInvalidParams,
			"the agent speaks protocol version %d and this client speaks %d",
			resp.ProtocolVersion, Version)
	}
	c.agent = resp
	return resp, nil
}

// Agent is what the agent claimed at initialize.
//
// Worth reading and worth distrusting: measured against the published agents,
// one of them advertises a capability and then answers an internal error to
// the method behind it. Degrade on the answer as well as on the claim.
func (c *Client) Agent() InitializeResponse { return c.agent }

// NewSession opens a session in a directory.
func (c *Client) NewSession(ctx context.Context, cwd string) (string, error) {
	var resp NewSessionResponse
	err := c.conn.Call(ctx, MethodNewSession, NewSessionRequest{
		Cwd: cwd,
		// Required by the schema and empty rather than absent: an agent that
		// reads null for a list is an agent we broke for no reason. This
		// shell is not an MCP host.
		MCPServers: []json.RawMessage{},
	}, &resp)
	return resp.SessionID, err
}

// Prompt sends one turn and returns its stop reason.
func (c *Client) Prompt(ctx context.Context, sessionID, text string) (string, error) {
	var resp PromptResponse
	err := c.conn.Call(ctx, MethodPrompt, PromptRequest{
		SessionID: sessionID,
		Prompt:    []ContentBlock{TextBlock(text)},
	}, &resp)
	return resp.StopReason, err
}

// Cancel asks for a session's running turn to stop. The turn's own answer is
// where the agent says that it did.
func (c *Client) Cancel(sessionID string) error {
	return c.conn.Notify(MethodCancel, CancelNotification{SessionID: sessionID})
}

// AuthRequired reports whether an agent refused because it has not been
// authenticated.
//
// It is worth a helper because it is not an edge case: two of the three
// published agents answer it to session/new, so a client that treats a
// successful initialize as "ready to work" fails on both.
func AuthRequired(err error) bool {
	var rpcErr *Error
	return errors.As(err, &rpcErr) && rpcErr.Code == CodeAuthRequired
}

// Unsupported reports whether an agent answered "I do not do that".
//
// Also not an edge case: one of the three answers it for every optional
// method. A client must carry on rather than fail, which is the protocol's own
// rule about capabilities read at the level of a single call.
func Unsupported(err error) bool {
	var rpcErr *Error
	return errors.As(err, &rpcErr) && rpcErr.Code == CodeMethodNotFound
}

func (c *Client) permission(ctx context.Context, params json.RawMessage) (any, error) {
	var req RequestPermissionRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, Errorf(CodeInvalidParams, "session/request_permission: %v", err)
	}
	if c.Answer == nil {
		// Nobody to ask, so nothing was allowed. Reported as a chosen
		// refusal rather than as an error, because the agent asked a
		// question and is owed an answer to it.
		return RequestPermissionResponse{Outcome: PermissionOutcome{
			Outcome: OutcomeSelected, OptionID: OptionRejectOnce,
		}}, nil
	}
	out, err := c.Answer(ctx, req)
	if err != nil {
		return nil, err
	}
	return RequestPermissionResponse{Outcome: out}, nil
}

func (c *Client) readFile(ctx context.Context, params json.RawMessage) (any, error) {
	var req ReadTextFileRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, Errorf(CodeInvalidParams, "fs/read_text_file: %v", err)
	}
	if !c.Files {
		return nil, Errorf(CodeMethodNotFound, "this client does not serve %s", MethodReadTextFile)
	}
	if !c.Boundary.Open(ctx, req.Path, false) {
		// Refused the way a denied open is refused everywhere else: as a
		// path that is not there. A policy hiding a file and a file that
		// does not exist are indistinguishable to whoever asked, which is
		// the rule the interpreter's own probes set.
		return nil, Errorf(CodeInvalidParams, "%s: no such file or directory", req.Path)
	}
	b, err := os.ReadFile(req.Path)
	if err != nil {
		c.failed(ctx, interp.Action{Kind: interp.ActionOpen, Path: req.Path}, err)
		return nil, Errorf(CodeInvalidParams, "%s: %v", req.Path, err)
	}
	return ReadTextFileResponse{Content: window(string(b), req.Line, req.Limit)}, nil
}

// failed records an access that was allowed and then did not work.
//
// The boundary records the *attempt*, because out here a failure is often not
// an error — a startup file that is not there is the normal case. An access
// the agent asked for and that then failed is worth the second record: the
// agent will be told, and an audit trail that saw only the attempt would show
// a read that never happened as one that did.
func (c *Client) failed(ctx context.Context, a interp.Action, err error) {
	if c.Boundary.Events == nil {
		return
	}
	c.Boundary.Events.Emit(ctx, interp.Event{Kind: interp.EventError, Action: a, Err: err})
}

// window is the slice of a file the agent asked for: from Line, counted in
// lines from one, for Limit lines. Absent means from the beginning, and for
// all of it.
func window(text string, line, limit *int) string {
	if line == nil && limit == nil {
		return text
	}
	lines := strings.SplitAfter(text, "\n")
	// SplitAfter leaves a trailing empty piece when the text ends in a
	// newline, which is not a line.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	start := 0
	if line != nil && *line > 1 {
		start = *line - 1
	}
	if start > len(lines) {
		return ""
	}
	rest := lines[start:]
	if limit != nil && *limit >= 0 && *limit < len(rest) {
		rest = rest[:*limit]
	}
	return strings.Join(rest, "")
}

func (c *Client) writeFile(ctx context.Context, params json.RawMessage) (any, error) {
	var req WriteTextFileRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, Errorf(CodeInvalidParams, "fs/write_text_file: %v", err)
	}
	if !c.Files {
		return nil, Errorf(CodeMethodNotFound, "this client does not serve %s", MethodWriteTextFile)
	}
	if !c.Boundary.Open(ctx, req.Path, true) {
		return nil, Errorf(CodeInvalidParams, "%s: permission denied", req.Path)
	}
	if err := os.WriteFile(req.Path, []byte(req.Content), 0o600); err != nil {
		c.failed(ctx, interp.Action{Kind: interp.ActionOpen, Path: req.Path, Write: true}, err)
		return nil, Errorf(CodeInvalidParams, "%s: %v", req.Path, err)
	}
	return WriteTextFileResponse{}, nil
}

// Stderr copies an agent's diagnostics somewhere a person can see them.
//
// The protocol says an agent may write UTF-8 to its standard error for logging
// and that a client may capture, forward or ignore it. Forwarding is the only
// one of those three that helps when an agent will not start, which — measured
// — is most of what goes wrong: an agent that cannot authenticate says so
// there and answers -32000 on the wire.
func Stderr(r io.Reader, w io.Writer, prefix string) {
	s := bufio.NewScanner(r)
	for s.Scan() {
		if _, err := io.WriteString(w, prefix+s.Text()+"\n"); err != nil {
			return
		}
	}
}
