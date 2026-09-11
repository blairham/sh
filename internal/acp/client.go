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
	"strings"
	"sync"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/internal/jsonrpc"
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

	// Elicit puts a question from the agent to a person, as a form the agent
	// described. It is the protocol's own answer to "this needs a human", and
	// the only one that is not about permission.
	//
	// Nil is a client that cannot reach a person, and it is nil-ness that
	// decides what is advertised: the capability and the ability to honor it
	// are one field, so an agent is never told to ask a question that would go
	// nowhere.
	Elicit func(ctx context.Context, req CreateElicitationRequest) (CreateElicitationResponse, error)

	// Interpret runs a command *line* the way this shell runs one, writing
	// everything it produces to out, and returning its exit status.
	//
	// It is what `terminal/create` uses when the agent sent no `args`, which
	// is what every agent measured does when it means a shell command. Nil
	// exec's the command as a filename instead, which is what this client did
	// before #1782 and which no measured agent could get a command out of.
	//
	// A func rather than something this package does, for the reason Relaunch
	// is: interpreting shell is the whole of the rest of this program, and an
	// agent-protocol package that reached into it would be the dependency
	// pointing the wrong way. The caller wires it from the same driver.Shell
	// this client's boundary came from, so the policy and the audit trail on
	// this route are the session's own.
	Interpret func(ctx context.Context, cmd TerminalCommand, out io.Writer) int

	// Terminals says whether to advertise terminal/*, and it is the sharper
	// half of Files. An agent that cannot ask us to run something runs it
	// itself, and no gate anywhere sees the argv; off is for a caller that
	// accepts that.
	Terminals bool

	conn  *jsonrpc.Conn
	agent InitializeResponse

	// mu guards the terminals this client is running for the agent. They are
	// reached from more than one goroutine by construction: every inbound
	// request is handled on its own, so an agent may be creating one while it
	// waits on another.
	mu           sync.Mutex
	running      map[string]*terminal
	nextTerminal int

	// What this client knows about the commands the agent ran. Two counts and
	// no attempt to join them: see Commands.
	asked     int
	announced int
}

// Commands reports how many commands the agent asked this shell to run, and
// how many it announced having run.
//
// Two counts rather than one answer, and the gap between them is the whole of
// #786. `terminal/*` is the route by which a command an agent runs becomes a
// command *we* start, through the gate, into the audit trail. An agent that
// does not take it forks and execs in its own process, and no gate anywhere
// sees the argv — which nothing in this repository can force, exactly as any
// allowed exec is outside the boundary once it has started.
//
// Measured against the published adapters, that is not a corner case: asked in
// as many words to run a shell command, with `terminal: true` advertised, two
// of the three ran it themselves and called no client method at all.
//
// So both facts are counted and **neither is inferred from the other**. Asked
// is `terminal/create` requests served, allowed or refused. Announced is tool
// calls of kind `execute` the agent reported. There is no id joining an
// agent's tool call to a terminal it asked us for, and inventing one is what
// #719 already declined; a caller that wants to say something about the
// difference says it about the two numbers.
func (c *Client) Commands() (asked, announced int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.asked, c.announced
}

// Connect wires this client to an agent's streams.
//
// Separate from Serve, and not for tidiness: Serve blocks, so a caller runs it
// on a goroutine, and a client that built its connection *inside* Serve would
// hand every caller a race between starting that goroutine and making the
// first request. Connect happens on the caller's own goroutine, before either.
func (c *Client) Connect(r io.Reader, w io.Writer) { c.conn = jsonrpc.NewConn(r, w, c) }

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
	case MethodCreateElicitation:
		return c.elicit(ctx, params)
	case MethodCreateTerminal:
		return c.createTerminal(ctx, params)
	}
	// The four that name a terminal this client already made. Behind one
	// check, because an id can only exist if create was served, so serving
	// them while create is withheld would be answering about a thing that
	// cannot be.
	if c.Terminals {
		switch method {
		case MethodTerminalOutput:
			return c.terminalOutput(params)
		case MethodWaitForExit:
			return c.waitForExit(ctx, params)
		case MethodKillTerminal:
			return c.killTerminal(ctx, params)
		case MethodReleaseTerminal:
			return c.releaseTerminal(ctx, params)
		}
	}
	// Everything else is a capability we did not advertise. A client that
	// answered a method it never claimed would be telling the agent
	// something untrue about what it can rely on.
	return nil, jsonrpc.Errorf(jsonrpc.CodeMethodNotFound, "this client does not serve %s", method)
}

// Notify takes the agent's session updates.
func (c *Client) Notify(_ context.Context, method string, params json.RawMessage) {
	if method != MethodSessionUpdate {
		return
	}
	var n SessionNotification
	if err := json.Unmarshal(params, &n); err != nil {
		return
	}
	// Counted before anything is done with it, and whether or not anybody is
	// watching: what Commands reports must not depend on whether a caller
	// wired Update, or the answer would change with the front end.
	c.count(params)
	if c.Update == nil {
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

// count notes a tool call the agent announced for a command it ran.
//
// The announcement only — `tool_call` and not `tool_call_update` — so a
// command whose status changes three times is one command. Decoded on its own
// rather than out of the loose update above, because this reads two fields of
// one variant and that one must keep handing the caller the bytes it was sent.
func (c *Client) count(params json.RawMessage) {
	var n struct {
		Update struct {
			SessionUpdate string `json:"sessionUpdate"`
			Kind          string `json:"kind"`
		} `json:"update"`
	}
	if err := json.Unmarshal(params, &n); err != nil {
		return
	}
	if n.Update.SessionUpdate != UpdateToolCall || n.Update.Kind != KindExecute {
		return
	}
	c.mu.Lock()
	c.announced++
	c.mu.Unlock()
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
	// The sharper half of the file capability. An agent that was not told it
	// can ask us to run something runs it itself, and there is no argv for any
	// gate to see.
	req.Capabilities.Terminal = c.Terminals
	// Read off the hook, for the same reason and by the same rule: a mode this
	// client cannot serve is a mode it must not name. Only the form mode is
	// named, because a browser and a completion notification are a different
	// mechanism and not one a terminal answers.
	if c.Elicit != nil {
		req.Capabilities.Elicitation = &ElicitationCapabilities{Form: &ElicitationMode{}}
	}
	var resp InitializeResponse
	if err := c.conn.Call(ctx, MethodInitialize, req, &resp); err != nil {
		return resp, err
	}
	if resp.ProtocolVersion != Version {
		// The agent answered with a version it will speak and we do not.
		// Disconnecting is what the protocol says to do, and pretending
		// otherwise would put messages of an unknown shape on the wire.
		return resp, jsonrpc.Errorf(jsonrpc.CodeInvalidParams,
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
	var rpcErr *jsonrpc.Error
	return errors.As(err, &rpcErr) && rpcErr.Code == CodeAuthRequired
}

// Unsupported reports whether an agent answered "I do not do that".
//
// Also not an edge case: one of the three answers it for every optional
// method. A client must carry on rather than fail, which is the protocol's own
// rule about capabilities read at the level of a single call.
func Unsupported(err error) bool {
	var rpcErr *jsonrpc.Error
	return errors.As(err, &rpcErr) && rpcErr.Code == jsonrpc.CodeMethodNotFound
}

func (c *Client) permission(ctx context.Context, params json.RawMessage) (any, error) {
	var req RequestPermissionRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "session/request_permission: %v", err)
	}
	if c.Answer == nil {
		// Nobody to ask, so nothing was allowed. Reported as a chosen
		// refusal rather than as an error, because the agent asked a
		// question and is owed an answer to it.
		return RequestPermissionResponse{
			Outcome: Select(req.Options, KindRejectOnce),
		}, nil
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
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "fs/read_text_file: %v", err)
	}
	if !c.Files {
		return nil, jsonrpc.Errorf(jsonrpc.CodeMethodNotFound, "this client does not serve %s", MethodReadTextFile)
	}
	// The read is the boundary's rather than the os package's, so what is
	// checked is the object the open reached and not the name the agent sent.
	// This is the call site where that matters most: every other path this
	// package gates comes from the invocation or from a shell variable, and
	// this one is chosen, live, by a party outside the process.
	b, err := c.Boundary.ReadFile(ctx, req.Path)
	if errors.Is(err, boundary.ErrRefused) {
		// Answered as a path that is not there, so that a policy hiding a
		// file and a file that does not exist are indistinguishable to
		// whoever asked — and a name that *resolved* into a hidden place is
		// answered the same way, so an agent cannot map a hidden directory
		// one symlink at a time by asking to be refused.
		//
		// Two things this is not, both of which the comment here used to
		// claim or imply (#1798):
		//
		// It is **not** what the rest of the shell does. A denied open on
		// every other route says so — `open: refused: <path>`, and `.` says
		// `Refused`. This handler is the one place that answers absence, and
		// it is a deliberate difference rather than the house style.
		//
		// And it holds only while this client withholds `terminal/*`.
		// Measured: with terminals served and `-deny read:<path>` set, an
		// agent that is told "no such file" here asks us to run `cat <path>`
		// and is handed the contents — the child does its own opening, and
		// an allowed exec is outside the boundary once it has started. So
		// this protects a `Files: true, Terminals: false` client, which is a
		// real configuration, and protects nothing at all in the one
		// `-acp-connect` builds. Do not read it as a guarantee that a denied
		// file stays unread.
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "%s: no such file or directory", req.Path)
	}
	if err != nil {
		c.failed(ctx, interp.Action{Kind: interp.ActionOpen, Path: req.Path}, err)
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "%s: %v", req.Path, err)
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
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "fs/write_text_file: %v", err)
	}
	if !c.Files {
		return nil, jsonrpc.Errorf(jsonrpc.CodeMethodNotFound, "this client does not serve %s", MethodWriteTextFile)
	}
	err := c.Boundary.WriteFile(ctx,
		boundary.File{Path: req.Path, Perm: 0o600}, []byte(req.Content))
	if errors.Is(err, boundary.ErrRefused) {
		// Said plainly, where a refused *read* says the path is not there,
		// and the difference is deliberate rather than an oversight (#1798).
		//
		// What the read hides is whether a file **exists**, which is a fact
		// about the disk that the agent would not otherwise have. A refused
		// write reveals only that a rule stands here, which is a fact about
		// the policy — and one the agent is about to learn anyway, because a
		// write it is not allowed to do has to fail somehow. Answering "no
		// such file" to a write would also be a strange thing to say about a
		// path that is supposed not to exist yet: not existing is the normal
		// case for a file being created, so it would read as success.
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "%s: permission denied", req.Path)
	}
	if err != nil {
		c.failed(ctx, interp.Action{Kind: interp.ActionOpen, Path: req.Path, Write: true}, err)
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "%s: %v", req.Path, err)
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
