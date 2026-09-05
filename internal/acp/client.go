// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	// Relaunch runs the agent's own command again — the same program with the
	// same arguments, plus these — with env set over the top, attached to a
	// terminal a person can type into. A zero exit status means they
	// authenticated; any other termination means they did not.
	//
	// It is a hook rather than something this package does, because this
	// package does not know how the agent was started: only the caller that
	// launched it can reproduce that invocation, which is precisely what
	// terminal authentication asks for.
	//
	// Nil is a client that cannot serve terminal authentication, and it is
	// nil-ness that decides what is *advertised*: the capability and the
	// ability to honor it are one field, so there is no arrangement in which
	// an agent is offered a login this client cannot run.
	Relaunch func(ctx context.Context, args []string, env map[string]string) error

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
	// Claimed from the hook rather than from a flag beside it. An agent only
	// offers a terminal method to a client that says it can run one, so this
	// is what decides whether such a method is ever on the list — and reading
	// it off the same field that performs one is how a capability that is
	// served but not advertised, or advertised but not served, is made
	// unrepresentable.
	req.Capabilities.Auth = AuthCapabilities{Terminal: c.Relaunch != nil}
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

// Authenticate settles one of the methods the agent advertised.
//
// Authentication is the *first* thing that happens with a real agent rather
// than the last: measured, two of the three published ones refuse session/new
// with -32000 until it has. So this belongs between Initialize and NewSession,
// and a client that treats a successful handshake as "ready to work" is a
// client that works with one agent in three.
//
// The method is looked up in what the agent actually advertised, and an id
// that is not on that list is refused here without a message being sent. That
// is the same rule as an option id we never offered, read the other way round:
// the agent named the choices, so a choice it did not name is not one.
//
// What happens next is the kind's, not ours to pick. An agent method is the
// `authenticate` call; a terminal method is deliberately *not* — the schema
// forbids passing one to `authenticate` — and is the relaunch instead.
func (c *Client) Authenticate(ctx context.Context, id string) error {
	m, ok := c.authMethod(id)
	if !ok {
		return fmt.Errorf("acp: %q is not one of the authentication methods this agent offers%s",
			id, offered(c.agent.AuthMethods))
	}
	if m.Kind() == AuthTerminal {
		if c.Relaunch == nil {
			// Only reachable from an agent that offered a terminal method
			// without being told it could, since the capability is the hook.
			// Worth saying rather than assuming, because the failure it
			// prevents is a person watching a login that never opens.
			return fmt.Errorf("acp: %q wants a terminal to log in on and this client has none", id)
		}
		if err := c.Relaunch(ctx, m.Args, m.Env); err != nil {
			return fmt.Errorf("acp: %q: %w", id, err)
		}
		return nil
	}
	return c.conn.Call(ctx, MethodAuthenticate, AuthenticateRequest{MethodID: id}, nil)
}

// authMethod finds an advertised method by id.
func (c *Client) authMethod(id string) (AuthMethod, bool) {
	for _, m := range c.agent.AuthMethods {
		if m.ID == id {
			return m, true
		}
	}
	return AuthMethod{}, false
}

// offered names what was on the list, for a message about something that was
// not. An agent that advertised nothing is a distinct answer from one that
// advertised something else, and saying "offers ()" would hide it.
func offered(methods []AuthMethod) string {
	if len(methods) == 0 {
		return "; it advertised none"
	}
	ids := make([]string, 0, len(methods))
	for _, m := range methods {
		ids = append(ids, m.ID)
	}
	return " (" + strings.Join(ids, ", ") + ")"
}

// NewSession opens a session in a directory.
func (c *Client) NewSession(ctx context.Context, cwd string) (string, error) {
	var resp NewSessionResponse
	// No MCP servers: this shell is not an MCP host. The list is still
	// written, as an empty array — see NewSessionRequest, where leaving it out
	// was a real refusal from a real agent rather than a hypothetical one.
	err := c.conn.Call(ctx, MethodNewSession, NewSessionRequest{Cwd: cwd}, &resp)
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
