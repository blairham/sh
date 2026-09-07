// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/internal/jsonrpc"
)

// The shell as an ACP Agent: an editor launches it, sends it shell to run,
// watches what it does, and answers when it asks whether it may.
//
// This is a fourth consumer of driver.Shell, beside `-c`, a script file and
// the prompt, and it adds no question to the interpreter — it answers the two
// that are already there. Gate consultations become session/request_permission
// and the event stream becomes session/update; execution goes through
// driver.Session, which is the `-c` route held open.
//
// The role-specific part is thin on purpose. Everything below the dispatch —
// the framing, the message shapes, the permission model — is shared with the
// client side and lives in this package without a side.

// Agent answers ACP on one connection.
//
// One agent serves many sessions, each with its own shell, its own working
// directory and its own memory of what a person allowed.
type Agent struct {
	// Shell is the template every session's shell is built from: the three
	// vectors, the prelude, the builtins. A session fills in what is its
	// own — the directory, the three streams, the gate and the sink — and
	// leaves the rest of it alone.
	Shell driver.Shell

	// Info is what this agent calls itself to the client.
	Info Implementation

	// Verbose reports every access as a tool call, including the stats and
	// directory reads a glob makes in bulk. Off by default: a client shown
	// one tool call per PATH candidate is a client showing nothing.
	Verbose bool

	conn *jsonrpc.Conn

	mu          sync.Mutex
	initialized bool
	sessions    map[string]*session
	nextSession int
}

// NewAgent builds one on a shell template.
func NewAgent(sh driver.Shell, info Implementation) *Agent {
	// A shell that is a protocol server must not stop being one. `exec cmd`
	// replacing this process would take the connection and every other
	// session on it, and a fatal signal aimed at the shell is the same
	// argument — so this is the one place a binary that *is* a shell says it
	// is not the whole process. See docs/design/acp.md.
	sh.KeepProcess = true
	return &Agent{Shell: sh, Info: info, sessions: map[string]*session{}}
}

// Serve reads ACP on r, writes it on w, and returns when the input ends.
//
// r and w are the process's standard input and output when this is a binary,
// and they are not the shell's: an agent must not write anything to standard
// output that is not a protocol message, and a shell's whole job is writing to
// standard output. Each session gives its runner writers of its own, and every
// write becomes a session update.
func (a *Agent) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	a.conn = jsonrpc.NewConn(r, w, a)
	return a.conn.Serve(ctx)
}

// Handle answers a request.
func (a *Agent) Handle(ctx context.Context, method string, params json.RawMessage) (any, error) {
	if method != MethodInitialize && !a.ready() {
		// The handshake is the first thing that happens. A version has not
		// been agreed yet, so nothing else can be answered in it.
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidRequest, "initialize first")
	}
	switch method {
	case MethodInitialize:
		return a.initialize(params)
	case MethodNewSession:
		return a.newSession(params)
	case MethodPrompt:
		return a.prompt(ctx, params)
	}
	// Everything else, deliberately. authenticate and logout because a local
	// shell authenticates by being a process the person already started;
	// session/load because a shell's session is its variables, functions,
	// directory and descriptors and none of that is in a transcript; modes
	// and config options because the obvious candidate is the dialect and it
	// is runtime state a script can change mid-turn. docs/design/acp.md says
	// so for each.
	return nil, jsonrpc.Errorf(jsonrpc.CodeMethodNotFound, "%s is not served by a shell", method)
}

// Notify takes a notification, which has no answer and cannot fail.
func (a *Agent) Notify(_ context.Context, method string, params json.RawMessage) {
	if method != MethodCancel {
		return
	}
	var n CancelNotification
	if err := json.Unmarshal(params, &n); err != nil {
		return
	}
	if s := a.session(n.SessionID); s != nil {
		s.cancel()
	}
}

func (a *Agent) ready() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.initialized
}

func (a *Agent) initialize(params json.RawMessage) (any, error) {
	var req InitializeRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "initialize: %v", err)
	}
	if req.ProtocolVersion < Version {
		// The client is older than we are and has told us the newest it
		// speaks. There is no negotiating downwards here — one version
		// exists — so this is a refusal rather than an agreement to
		// something neither side has implemented.
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams,
			"protocol version %d: this agent speaks %d", req.ProtocolVersion, Version)
	}
	a.mu.Lock()
	a.initialized = true
	a.mu.Unlock()
	info := a.Info
	return InitializeResponse{
		// Ours, which is the version this agent speaks rather than the
		// newest the client offered: the client disconnects if it cannot
		// speak it.
		ProtocolVersion: Version,
		AgentCapabilities: AgentCapabilities{
			// Every one of these is false and each is a decision rather
			// than an omission — docs/design/acp.md has the list.
			LoadSession:        false,
			PromptCapabilities: PromptCapabilities{},
			MCPCapabilities:    MCPCapabilities{},
		},
		// Empty rather than absent: the schema has an array here, and a
		// client that reads null for a list is a client we broke for no
		// reason. A local shell authenticates by being a process the person
		// already started.
		AuthMethods: []AuthMethod{},
		AgentInfo:   &info,
	}, nil
}

func (a *Agent) newSession(params json.RawMessage) (any, error) {
	var req NewSessionRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "session/new: %v", err)
	}
	a.mu.Lock()
	a.nextSession++
	id := "sess-" + strconv.Itoa(a.nextSession)
	a.mu.Unlock()

	s := &session{id: id, agent: a, mem: &Memory{}}
	sh := a.Shell
	// Where this session is. A process has one working directory and a
	// connection has many sessions, which is the whole reason driver.Shell
	// grew a Dir: chdir-ing between them is what the library rule forbids,
	// one level up.
	sh.Dir = req.Cwd
	// What the commands write, as they write it. Both streams go out as
	// message chunks and are told apart in _meta, because v1 has no update
	// kind that means "diagnostics".
	s.out = &chunker{emit: func(text string) { s.chunk(text, StreamStdout) }}
	s.errs = &chunker{emit: func(text string) { s.chunk(text, StreamStderr) }}
	sh.Stdout, sh.Stderr = s.out, s.errs
	// A read gets end of file, and it must get it from something of our
	// choosing rather than by being left nil: the front end fills a nil
	// standard input in with the *process's*, which here is the protocol
	// stream, and a script's `read` would then eat the client's next message.
	//
	// The null device, on the same footing as the other fixed
	// interpreter-chosen paths docs/design.md exempts: the script never named
	// it, and gating it would let a policy turn a shell's empty input into
	// the connection.
	//
	// A *file* and not an in-process empty reader, which the field would take
	// now that it is an io.Reader (#787). The session's input is inherited by
	// every external command it runs, and os/exec connects a child straight to
	// an *os.File and builds a pipe with a copying goroutine for anything
	// else. One descriptor for the session beats a pipe for every command in
	// it.
	//
	// ACP is non-interactive on this side. The protocol is on the descriptors
	// a prompt would need, there is no terminal, and where a script genuinely
	// needs a person the protocol has elicitation — which this side still does
	// not reach, for a reason that is now one level below this field and is
	// written down in docs/design/acp.md.
	empty, err := os.Open(os.DevNull)
	if err != nil {
		return nil, jsonrpc.Errorf(jsonrpc.CodeInternalError, "no empty input for the session: %v", err)
	}
	sh.Stdin = empty
	// The template's gate goes *inside* this one rather than being replaced by
	// it. A policy handed to `-acp` is what the invocation asked for, and a
	// session that assigned over it would take the flag and mean nothing by
	// it — which is what this line did until #1335: `-acp -policy p` requested
	// permission for every exec, the client answered allow-once, and a program
	// the policy refused ran anyway.
	//
	// Order is the whole of the rule, and it is the one docs/design/acp.md
	// states: the inner gate first, and its refusal is final and unaskable. A
	// person offered a button that overrides the sandbox is a sandbox that is
	// advisory — and on this route "a person" may be an agent harness that
	// answers by itself, which would make it advisory with nobody watching.
	sh.Gate = &Gate{Inner: sh.Gate, Ask: s.ask, Remembered: s.mem}
	sh.Events = watched(sh.Events, s)
	shell, code := driver.NewSession(sh)
	if shell == nil {
		return nil, jsonrpc.Errorf(jsonrpc.CodeInternalError, "the shell would not start: status %d", code)
	}
	s.shell = shell

	a.mu.Lock()
	a.sessions[id] = s
	a.mu.Unlock()
	return NewSessionResponse{SessionID: id}, nil
}

// watched is where a session's events go: to the client, and to whatever the
// invocation was already writing them to.
//
// The same argument cmd/sh makes about a plugin's observer, in the place the
// same mistake was made: `-audit f` writes a file and `-trace-events` writes
// to the terminal, and somebody who served ACP from a shell that was already
// keeping a record did not ask for the record to stop. An audit trail a
// protocol front end can silence is not an audit trail — and until #1335 this
// was an assignment, so `sh -acp -audit f` created the file, wrote nothing to
// it, and said nothing about either.
//
// Nil stays nil rather than becoming a discard, so a shell nobody pointed a
// sink at still pays one nil check per event.
func watched(already interp.Sink, s *session) interp.Sink {
	if already == nil {
		return s
	}
	return bothSinks{outer: already, session: s}
}

// bothSinks feeds two sinks as one, the record before the client: what a shell
// did is written down whether or not the connection is still there to hear
// about it.
type bothSinks struct{ outer, session interp.Sink }

func (b bothSinks) Emit(ctx context.Context, e interp.Event) {
	b.outer.Emit(ctx, e)
	b.session.Emit(ctx, e)
}

func (a *Agent) session(id string) *session {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sessions[id]
}

func (a *Agent) prompt(ctx context.Context, params json.RawMessage) (any, error) {
	var req PromptRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "session/prompt: %v", err)
	}
	s := a.session(req.SessionID)
	if s == nil {
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "no session %q", req.SessionID)
	}
	return s.run(ctx, program(req.Prompt))
}

// program is the shell a prompt asks for: the text blocks, joined by newlines.
//
// Non-text blocks are skipped rather than refused. The baseline requires an
// agent to tolerate a resource link in a prompt and there is nothing useful a
// shell does with one, so tolerating it is the whole of the obligation.
func program(blocks []ContentBlock) string {
	var text []string
	for _, b := range blocks {
		if b.Type == ContentText {
			text = append(text, b.Text)
		}
	}
	return strings.Join(text, "\n")
}

// session is one shell, and one client's view of it.
type session struct {
	id    string
	agent *Agent
	shell *driver.Session
	mem   *Memory
	calls tracker
	out   *chunker
	errs  *chunker

	mu     sync.Mutex
	stop   context.CancelFunc
	inTurn bool

	// report is where an update goes. Nil is the connection, which is what a
	// session on a real one has; a test that is about the mapping rather than
	// about the wire fills it in and reads the values.
	report func(any)
}

// run is one prompt turn.
func (s *session) run(ctx context.Context, src string) (any, error) {
	ctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	if s.inTurn {
		s.mu.Unlock()
		cancel()
		// One shell runs one program at a time, and interleaving two in one
		// runner would give neither the variables it wrote. A caller waiting
		// for a turn that has not started is owed an answer rather than a
		// delay.
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "session %s is already running a turn", s.id)
	}
	s.inTurn, s.stop = true, cancel
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.inTurn, s.stop = false, nil
		s.mu.Unlock()
		cancel()
		// Whatever was held back waiting for the rest of a character is
		// better shown than dropped, now that no more of it is coming.
		s.out.Flush()
		s.errs.Flush()
	}()

	s.shell.Run(ctx, src)
	if ctx.Err() != nil {
		return PromptResponse{StopReason: StopCancelled}, nil
	}
	// A failing command is a turn that completed, not a turn that stopped:
	// the status is the script's business and the turn's is whether it ran.
	return PromptResponse{StopReason: StopEndTurn}, nil
}

func (s *session) cancel() {
	s.mu.Lock()
	stop := s.stop
	s.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// notify sends one session update, dropping it if the connection has gone.
// There is nowhere to report a failed notification to except the connection
// that just failed.
func (s *session) notify(u any) {
	if s.report != nil {
		s.report(u)
		return
	}
	_ = s.agent.conn.Notify(MethodSessionUpdate, SessionNotification{SessionID: s.id, Update: u})
}

func (s *session) chunk(text, stream string) { s.notify(Chunk(text, stream)) }

// ask puts an action to the client and reads the answer.
//
// The tool call is opened *before* the request, because
// session/request_permission carries one and a client has to be able to show a
// person what it is asking about. The event that follows updates that tool
// call rather than announcing a second, which is what the tracker is for.
func (s *session) ask(ctx context.Context, a interp.Action) (Decision, error) {
	e := interp.Event{Action: a}
	id := s.calls.begin(e)
	s.notify(Notice{SessionUpdate: UpdateToolCall, ToolCall: ToolCall{
		ToolCallID: id,
		Title:      title(a),
		Kind:       toolKind(a),
		Status:     StatusPending,
		Locations:  locations(e),
		RawInput:   describe(a),
	}})
	var resp RequestPermissionResponse
	if err := s.agent.conn.Call(ctx, MethodRequestPermission, RequestPermissionRequest{
		SessionID: s.id,
		ToolCall:  ToolCall{ToolCallID: id},
		Options:   PermissionOptions(),
	}, &resp); err != nil {
		return Decision{}, err
	}
	return Decide(resp.Outcome), nil
}

// Emit turns one interpreter event into a session update.
//
// Called from every goroutine the shell has — a background job reports from
// the goroutine running it, and so does each half of a pipeline — and holds no
// lock of its own beyond the tracker's.
func (s *session) Emit(_ context.Context, e interp.Event) {
	switch e.Kind {
	case interp.EventCommandStart:
		id, ok := s.calls.head(e)
		if ok {
			// Announced already, when the gate asked about it.
			s.notify(Notice{
				SessionUpdate: UpdateToolCallUpdate,
				ToolCall:      ToolCall{ToolCallID: id, Status: StatusInProgress},
			})
			return
		}
		s.notify(Notice{SessionUpdate: UpdateToolCall, ToolCall: ToolCall{
			ToolCallID: s.calls.begin(e),
			Title:      title(e.Action),
			Kind:       toolKind(e.Action),
			Status:     StatusInProgress,
			Locations:  locations(e),
			RawInput:   describe(e.Action),
		}})
	case interp.EventCommandEnd:
		status := StatusCompleted
		if e.Status != 0 {
			status = StatusFailed
		}
		s.finish(e, status, map[string]any{"exitStatus": e.Status}, "")
	case interp.EventDenied:
		s.finish(e, StatusFailed, nil, "refused: "+title(e.Action))
	case interp.EventError:
		text := ""
		if e.Err != nil {
			text = e.Err.Error()
		}
		s.finish(e, StatusFailed, nil, text)
	case interp.EventAccess:
		if !s.reportable(e) {
			return
		}
		s.finish(e, StatusCompleted, nil, "")
	default:
		// A kind this shell has not seen. Reported rather than dropped,
		// which is the event contract's own rule for a name a consumer does
		// not know: record it and carry on. ActionSignal arriving after the
		// other five is the worked example — a consumer that silently
		// ignored what it did not recognize would have shown a client a
		// shell that never signaled anything.
		s.finish(e, StatusCompleted, map[string]any{"event": e.Kind.String()}, "")
	}
}

// reportable decides whether an access is worth a client's attention.
//
// An open and a signal are: they are the shell reaching outside itself at a
// path or a process somebody chose. A stat and a directory read are not, by
// default — one PATH search stats every candidate and one glob reads every
// directory it descends, so reporting them turns the session log into a
// syscall trace and hides the commands inside it. Verbose says otherwise, and
// the audit log in internal/event has all of them regardless: that is the
// record, and this is the view.
func (s *session) reportable(e interp.Event) bool {
	if s.agent.Verbose {
		return true
	}
	switch e.Action.Kind {
	case interp.ActionOpen, interp.ActionSignal:
		return true
	}
	return false
}

// finish closes the tool call an event ends, opening one first where nothing
// opened it — a refusal or a failure with no start before it still has to
// reach the client.
func (s *session) finish(e interp.Event, status string, raw map[string]any, text string) {
	var content []ToolCallContent
	if text != "" {
		content = []ToolCallContent{TextContent(text)}
	}
	if id, ok := s.calls.end(e); ok {
		s.notify(Notice{SessionUpdate: UpdateToolCallUpdate, ToolCall: ToolCall{
			ToolCallID: id,
			Status:     status,
			Content:    content,
			RawOutput:  raw,
		}})
		return
	}
	id := s.calls.begin(e)
	// Opened and closed in one message, which is what an action nobody
	// announced is: an access that needed no permission, or an error before
	// anything started.
	s.calls.end(e)
	s.notify(Notice{SessionUpdate: UpdateToolCall, ToolCall: ToolCall{
		ToolCallID: id,
		Title:      title(e.Action),
		Kind:       toolKind(e.Action),
		Status:     status,
		Content:    content,
		Locations:  locations(e),
		RawInput:   describe(e.Action),
		RawOutput:  raw,
	}})
}

// Close ends every session, running each shell's EXIT trap once.
func (a *Agent) Close(ctx context.Context) {
	a.mu.Lock()
	sessions := make([]*session, 0, len(a.sessions))
	for _, s := range a.sessions {
		sessions = append(sessions, s)
	}
	a.sessions = map[string]*session{}
	a.mu.Unlock()
	for _, s := range sessions {
		s.shell.Close(ctx)
	}
}
