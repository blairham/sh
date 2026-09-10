// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package acp speaks the Agent Client Protocol, in both directions.
//
// The gate and the event stream are the shell's permission-and-audit surface,
// and docs/design/acp.md is the design this implements. ACP has two roles and
// this shell is both: an editor drives it as an *Agent*, and it drives a
// coding agent as a *Client*. Gate consultations become permission requests
// and the event stream becomes session updates one way round; the agent's own
// requests pass through the same gate the other way round.
//
// **Nothing in this package has a side.** JSON-RPC is symmetric — a
// jsonrpc.Conn answers and calls out, whichever end of a connection it is —
// and one party
// writes the message shapes the other reads. Turning a permission option into
// a decision is the same function whoever chose the option. The role-specific
// part is only which methods a peer answers and which capabilities are its to
// claim, and that lives above this.
//
// The revision is **protocol version 1**, wire schema v1, over stdio —
// newline-delimited JSON-RPC 2.0. Every shape in this file was taken from the
// protocol's published machine-readable schema; the design document records
// which release and why v2, which is an alpha, is not targeted yet.
//
// Only the subset a shell can honestly serve is here. There is no model, so
// there is no plan, no token budget and no thought stream; there is no
// transcript that restoring would restore a shell from, so no session/load.
// What is absent is absent on purpose and the design document says why for
// each one.
package acp

import (
	"encoding/json"
	"fmt"
)

// Version is the ACP protocol revision this package speaks. It is bumped only
// for breaking changes to the protocol; everything else is negotiated through
// the capabilities below.
const Version = 1

// The methods, spelled once. The agent ones arrive; the client ones are
// called out.
const (
	MethodInitialize        = "initialize"
	MethodAuthenticate      = "authenticate"
	MethodNewSession        = "session/new"
	MethodPrompt            = "session/prompt"
	MethodCancel            = "session/cancel"
	MethodSessionUpdate     = "session/update"
	MethodRequestPermission = "session/request_permission"
)

// Implementation names a side of the connection. Optional in v1 and sent
// anyway: a client that logs which agent it is talking to is a client that can
// report a bug against the right one.
type Implementation struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

// InitializeRequest is what the client opens with.
//
// The capabilities it advertises are read and mostly declined: a shell does
// not call fs/read_text_file or the terminal methods even where they are
// offered, because an access made through the client is an access the gate
// never saw. See docs/design/acp.md.
type InitializeRequest struct {
	ProtocolVersion int                `json:"protocolVersion"`
	ClientInfo      *Implementation    `json:"clientInfo,omitempty"`
	Capabilities    ClientCapabilities `json:"clientCapabilities"`
}

// ClientCapabilities is the part of the client's advertisement this agent
// reads. The rest is ignored rather than rejected, which is what a protocol
// that grows by capability requires of both sides.
type ClientCapabilities struct {
	FS       FileSystemCapabilities `json:"fs"`
	Terminal bool                   `json:"terminal"`
	Auth     AuthCapabilities       `json:"auth"`
	// Elicitation is omitted rather than written empty, which is the schema's
	// own way of saying "not supported": omitted and null both mean no, and
	// supplying an object is the claim. A struct written always would claim
	// the capability and then name no mode inside it, which is a shape an
	// agent has to guess about.
	Elicitation *ElicitationCapabilities `json:"elicitation,omitempty"`
}

// AuthCapabilities is what a client can do about being authenticated, and it
// governs which *kinds* of authentication method an agent may offer back.
//
// There is one, and it is opt-in for a reason worth stating: a terminal method
// is not a message but a relaunch of the agent's own program on a terminal a
// person types into, so a client that cannot do that must not claim it. An
// agent told otherwise offers a login that goes nowhere.
type AuthCapabilities struct {
	Terminal bool `json:"terminal"`
}

// FileSystemCapabilities is the client's offer to read and write files on the
// agent's behalf.
type FileSystemCapabilities struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

// InitializeResponse answers with the version this agent will speak and what
// it can do.
type InitializeResponse struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities"`
	AuthMethods       []AuthMethod      `json:"authMethods"`
	AgentInfo         *Implementation   `json:"agentInfo,omitempty"`
}

// AgentCapabilities is what an agent claims. What *this* shell claims when it
// is the agent is deliberately little.
//
// loadSession is false because a shell's session is its variables, functions,
// working directory and descriptors, and none of that is in the update stream:
// restoring the transcript would restore the appearance of a session. The
// prompt capabilities are the baseline — text and resource links — because
// there is nothing a shell does with an image.
type AgentCapabilities struct {
	LoadSession        bool               `json:"loadSession"`
	PromptCapabilities PromptCapabilities `json:"promptCapabilities"`
	MCPCapabilities    MCPCapabilities    `json:"mcpCapabilities"`
}

// PromptCapabilities says which content beyond the baseline a prompt may
// carry. All false: text is the program and a shell has no use for the rest.
type PromptCapabilities struct {
	Image           bool `json:"image"`
	Audio           bool `json:"audio"`
	EmbeddedContext bool `json:"embeddedContext"`
}

// MCPCapabilities says which MCP transports the agent can connect out over. A
// shell is not an MCP host, so neither.
type MCPCapabilities struct {
	HTTP bool `json:"http"`
	SSE  bool `json:"sse"`
}

// AuthMethod is one way of authenticating to an agent.
//
// The union is discriminated on `type`, and an **absent type means agent** —
// a default rather than an unknown, which is the schema's own wording and the
// reason Kind exists rather than a bare field read.
//
// The two kinds are not two spellings of one thing. *Agent* auth is a message:
// the agent is asked to authenticate itself and does whatever it does, which
// is usually a browser and a callback server of its own. *Terminal* auth is
// not a message at all — the client runs the agent's **own program again**,
// with these arguments appended and these environment variables set, on a
// terminal a person can type into, and a zero exit status is the whole of the
// answer. The schema is explicit that a terminal method must never be passed
// to `authenticate`, so the two are dispatched on Kind rather than tried in
// turn.
//
// Args and Env are therefore meaningful only for the terminal kind: they are
// the extra half of an invocation the client already knows how to make. A
// client that does not know how the agent was launched cannot make one, which
// is what the AuthCapabilities opt-in above is for.
type AuthMethod struct {
	Type        string            `json:"type,omitempty"`
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
}

// The authentication method kinds.
const (
	AuthAgent    = "agent"
	AuthTerminal = "terminal"
)

// Kind is the method's discriminator with the schema's default filled in.
//
// A method that names no type is an agent method, and that is worth a function
// rather than a field read because it is what the published agents actually
// send: measured, all four of Gemini's arrive with no `type` at all, and a
// client dispatching on the raw field would find "" and match neither kind.
func (m AuthMethod) Kind() string {
	if m.Type == "" {
		return AuthAgent
	}
	return m.Type
}

// AuthenticateRequest names which of the advertised methods to use.
type AuthenticateRequest struct {
	MethodID string `json:"methodId"`
}

// AuthenticateResponse is empty, and is an object rather than nothing for the
// same reason the write response is: a later revision with something to say
// there has somewhere to say it.
type AuthenticateResponse struct{}

// NewSessionRequest asks for a new session in a directory.
//
// mcpServers is required by the schema and ignored by this shell's agent side,
// which is why it is kept as raw JSON: decoding a shape we will not act on
// would be pretending to support it, and refusing a request that carries one
// would break clients that always send a list.
//
// It carries no omitempty, and that is not a style choice. An empty list and
// an absent field are the same thing to Go and are not the same thing to an
// agent: measured against the published Claude adapter, `{"cwd":"/tmp"}`
// answers `-32602` with `mcpServers: Required value is missing`, while
// `{"cwd":"/tmp","mcpServers":[]}` opens a session. A required field is
// written even when it is empty.
type NewSessionRequest struct {
	Cwd        string            `json:"cwd"`
	MCPServers []json.RawMessage `json:"mcpServers"`
}

// MarshalJSON writes the required list as an empty array rather than as null.
//
// The same hazard one step further in: a nil slice is not omitted once
// omitempty is gone, it is written as `null`, and an agent that requires a
// list is no happier with null than with nothing.
func (r NewSessionRequest) MarshalJSON() ([]byte, error) {
	type request NewSessionRequest
	out := request(r)
	if out.MCPServers == nil {
		out.MCPServers = []json.RawMessage{}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("acp: session/new: %w", err)
	}
	return b, nil
}

// NewSessionResponse names the session that was made.
type NewSessionResponse struct {
	SessionID string `json:"sessionId"`
}

// PromptRequest carries the turn's content. For a shell the text blocks,
// joined with newlines, are the program.
type PromptRequest struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

// PromptResponse ends the turn.
type PromptResponse struct {
	StopReason string `json:"stopReason"`
}

// The stop reasons this agent uses. A failing command is a turn that
// completed, so a non-zero exit status is still end_turn; max_tokens and
// max_turn_requests have no meaning without a model and are never sent.
const (
	StopEndTurn = "end_turn"
	// The protocol spells this with two Ls; it is a wire value and not prose,
	// so it stays as the schema has it.
	StopCancelled = "cancelled" //nolint:misspell // the protocol's spelling
	StopRefusal   = "refusal"
)

// ContentBlock is one piece of content. The union is wider than this — image,
// audio, embedded resource — and the fields those variants need are absent
// because this agent advertises none of them.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	URI  string `json:"uri,omitempty"`
	Name string `json:"name,omitempty"`
}

// The content block types this agent reads or writes.
const (
	ContentText         = "text"
	ContentResourceLink = "resource_link"
)

// TextBlock is the content block a shell produces: what a command wrote.
func TextBlock(text string) ContentBlock {
	return ContentBlock{Type: ContentText, Text: text}
}

// CancelNotification asks for the session's running turn to stop. It is a
// notification, so the turn's own response is where the client learns that it
// did: session/prompt answers with StopCancelled.
type CancelNotification struct {
	SessionID string `json:"sessionId"`
}

// SessionNotification is the session/update envelope. Update is one of the
// update types below, each of which carries its own discriminator.
type SessionNotification struct {
	SessionID string `json:"sessionId"`
	Update    any    `json:"update"`
}

// The session update kinds this agent sends.
const (
	UpdateAgentMessageChunk = "agent_message_chunk"
	UpdateToolCall          = "tool_call"
	UpdateToolCallUpdate    = "tool_call_update"
)

// MessageChunk is a piece of what the shell wrote, as it was written.
//
// Both streams arrive as this kind, because v1 has no update that means
// "diagnostics": agent_thought_chunk means a model's reasoning and using it
// for stderr would be a lie about what the client is showing. Which stream a
// chunk came from is carried in _meta, which is the extension point the
// protocol reserves for exactly this.
type MessageChunk struct {
	SessionUpdate string         `json:"sessionUpdate"`
	Content       ContentBlock   `json:"content"`
	Meta          map[string]any `json:"_meta,omitempty"`
}

// The stream names carried in a chunk's _meta.
const (
	StreamStdout = "stdout"
	StreamStderr = "stderr"
)

// MetaStream is the _meta key naming which of the shell's streams a chunk came
// from. Namespaced, because _meta is shared with whatever else either side
// puts there.
const MetaStream = "sh.blairham.github.com/stream"

// Chunk builds a message chunk for text written to one of the shell's streams.
func Chunk(text, stream string) MessageChunk {
	return MessageChunk{
		SessionUpdate: UpdateAgentMessageChunk,
		Content:       TextBlock(text),
		Meta:          map[string]any{MetaStream: stream},
	}
}

// ToolCall describes one action of the shell's: a command it is running, a
// file it is opening, a signal it is sending.
//
// It is the same shape whether it is being announced, updated, or shown to a
// person in a permission request, which is the protocol's own arrangement:
// session/request_permission carries a tool call update, so the thing being
// asked about and the thing being reported are one object.
type ToolCall struct {
	ToolCallID string             `json:"toolCallId"`
	Title      string             `json:"title,omitempty"`
	Kind       string             `json:"kind,omitempty"`
	Status     string             `json:"status,omitempty"`
	Content    []ToolCallContent  `json:"content,omitempty"`
	Locations  []ToolCallLocation `json:"locations,omitempty"`
	RawInput   any                `json:"rawInput,omitempty"`
	RawOutput  any                `json:"rawOutput,omitempty"`
}

// Notice wraps a tool call as a session update. The kind is either
// UpdateToolCall, which announces one, or UpdateToolCallUpdate, which changes
// one that has already been announced.
//
// The embedded ToolCall is anonymous so that its fields are inlined beside the
// discriminator, which is how the schema has it: a tool call update is a flat
// object with a sessionUpdate field, not a nested one.
type Notice struct {
	SessionUpdate string `json:"sessionUpdate"`
	ToolCall
}

// The tool kinds a shell produces. The protocol's list is longer; these are
// the ones an action of ours can honestly be.
const (
	KindExecute = "execute"
	KindRead    = "read"
	KindEdit    = "edit"
	KindOther   = "other"
)

// Tool call statuses.
const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

// ToolCallContent is something a tool call produced. Only the content variant
// is used: the diff variant describes an edit this shell does not make on the
// client's behalf, and the terminal variant names a terminal the client owns.
type ToolCallContent struct {
	Type    string       `json:"type"`
	Content ContentBlock `json:"content"`
}

// TextContent wraps text as tool call content.
func TextContent(text string) ToolCallContent {
	return ToolCallContent{Type: "content", Content: TextBlock(text)}
}

// ToolCallLocation is a file a tool call touched, so that a client can follow
// along in its own editor.
type ToolCallLocation struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
}

// RequestPermissionRequest asks the person on the other end whether an action
// may proceed.
type RequestPermissionRequest struct {
	SessionID string             `json:"sessionId"`
	ToolCall  ToolCall           `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

// PermissionOption is one answer a person may give.
type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// The four permission option kinds, and the ids this agent gives them. The ids
// are its own — the protocol only requires that they come back unchanged — and
// they are stable so that a client which remembers a choice remembers
// something meaningful.
const (
	KindAllowOnce   = "allow_once"
	KindAllowAlways = "allow_always"
	KindRejectOnce  = "reject_once"
	KindRejectAlway = "reject_always"

	OptionAllowOnce    = "allow-once"
	OptionAllowAlways  = "allow-always"
	OptionRejectOnce   = "reject-once"
	OptionRejectAlways = "reject-always"
)

// PermissionOptions is the set an escalated action is offered with: all four
// kinds, always, so that a person can settle a repeated question once without
// anything having to guess which questions repeat. Written by whichever side
// is asking, read by whichever side is answering.
func PermissionOptions() []PermissionOption {
	return []PermissionOption{
		{OptionID: OptionAllowOnce, Name: "Allow", Kind: KindAllowOnce},
		{OptionID: OptionAllowAlways, Name: "Allow always", Kind: KindAllowAlways},
		{OptionID: OptionRejectOnce, Name: "Reject", Kind: KindRejectOnce},
		{OptionID: OptionRejectAlways, Name: "Reject always", Kind: KindRejectAlway},
	}
}

// Select answers a permission request with the option the *agent* offered for
// a kind, which is the only kind of answer the protocol has.
//
// The ids in a request belong to whoever sent it. This shell's agent side
// spells them allow-once and reject-always; Claude Code's adapter spells the
// same two allow and reject_always; another agent may spell them anything at
// all. An id that was not offered is not a selection, and an agent is within
// its rights to read one as no answer — which is what one does, so an answering
// client that reaches for a constant here is a client whose allow means reject.
//
// The kind is the protocol's own vocabulary and is the same four values for
// everyone, so a caller says what it means and this resolves it against what
// was offered. An agent that offered no option of that kind gets
// OutcomeCancelled: there is no id to send, and inventing one is the bug this
// exists to stop.
func Select(options []PermissionOption, kind string) PermissionOutcome {
	for _, o := range options {
		if o.Kind == kind {
			return PermissionOutcome{Outcome: OutcomeSelected, OptionID: o.OptionID}
		}
	}
	return PermissionOutcome{Outcome: OutcomeCancelled}
}

// A TerminalCommand is one command line an agent asked this client to run,
// as terminal/create described it: the line itself, the directory it asked for
// (empty is the client's own), and the environment it is to run with, already
// merged over the process's.
type TerminalCommand struct {
	Line string
	Dir  string
	Env  []string
}

// RequestPermissionResponse is what came back.
type RequestPermissionResponse struct {
	Outcome PermissionOutcome `json:"outcome"`
}

// PermissionOutcome is either a selection or a cancellation. The schema makes
// it a union discriminated on `outcome`; both variants are flat, so one struct
// reads both and an outcome that is neither leaves OptionID empty — which is
// refused, because an answer that is not one of the offered options is not an
// answer.
type PermissionOutcome struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}

// The outcomes.
const (
	OutcomeSelected  = "selected"
	OutcomeCancelled = "cancelled" //nolint:misspell // the protocol's spelling
)

// The client methods an agent calls back with, which this shell answers rather
// than calls: a file the agent asks *us* to read is a file we open through the
// gate and record, where a file it opens for itself is invisible to everyone.
// docs/design/acp.md argues the asymmetry — the agent side of this shell
// declines to call these for the same reason the client side implements them.
const (
	MethodReadTextFile  = "fs/read_text_file"
	MethodWriteTextFile = "fs/write_text_file"
)

// ReadTextFileRequest asks the client to read a file on the agent's behalf.
//
// Line and Limit are a window into it, counted in lines from one. Both are
// optional and absent means the whole file.
type ReadTextFileRequest struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
	Line      *int   `json:"line,omitempty"`
	Limit     *int   `json:"limit,omitempty"`
}

// ReadTextFileResponse carries what was read.
type ReadTextFileResponse struct {
	Content string `json:"content"`
}

// WriteTextFileRequest asks the client to write a file on the agent's behalf.
type WriteTextFileRequest struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
	Content   string `json:"content"`
}

// WriteTextFileResponse is empty, and is an object rather than nothing so that
// a client which later has something to say there can say it.
type WriteTextFileResponse struct{}

// AuthRequired is the code an agent answers when it will do nothing until it
// has been authenticated.
//
// It is in ACP's own reserved range rather than JSON-RPC's, and a client has
// to expect it from the *first* thing it tries after initialize: measured
// against the published agents, two of the three refuse session/new with it.
// Treating a successful initialize as "ready to work" is what fails on them.
const CodeAuthRequired = -32000
