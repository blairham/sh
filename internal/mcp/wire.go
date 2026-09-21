// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package mcp serves the Model Context Protocol, as a server, over stdio.
//
// A coding agent or assistant launches this shell as a subprocess and speaks
// JSON-RPC to it a line at a time. What it gets is the five terminal verbs —
// start a command, read what it has written, wait for it, kill it, let it go —
// each one an interp.Action through the shell's gate, into the shell's audit
// trail.
//
// # This shell is an MCP server and is still not an MCP host
//
// docs/design/acp.md:136 says `session/new` carries `mcpServers`, that ours is
// accepted and ignored, and that **a shell is not an MCP host**. That line is
// about *consuming* MCP servers and it is unchanged. Being one is the other
// direction, and it is #1338.
//
// # The revision implemented against
//
// **`2026-07-28`**, which is the current released specification. That revision
// is "modern" in its own words: there is no `initialize` handshake, every
// request declares its version in `_meta`, and `server/discover` is the probe.
//
// A **legacy** handshake at `2025-11-25` is served beside it, and that is a
// measurement rather than a hedge. Measured 2026-09-21 against Claude Code
// 2.1.267, which is the client population this work is for: it opens with
// `initialize` carrying `protocolVersion: "2025-11-25"`, sends no `_meta` at
// all, never probes with `server/discover`, and accepts a server that answers
// with a different revision than it asked for. A modern-only server would be
// correct against the specification and unreachable by the client the issue
// names. The specification provides for exactly this: see its "Backward
// Compatibility with Initialization-Based Versions", which names a server that
// serves both a **dual-era** server and says how one selects its behavior —
// from how the client opens, per request, which is what eraOf does.
//
// # What is served, and what is not
//
// Tools, and nothing else. No resources, no prompts, no completions, no
// logging, no subscriptions, and no extensions: each of those is a capability
// this shell would have to have something to put in, and it does not. What is
// absent is absent because it is empty, not because it is unfinished.
//
// Server-initiated requests are absent for a stronger reason. On stdio the
// 2026-07-28 revision says the server **MUST NOT** write JSON-RPC requests to
// standard output; a server that needs something from a person answers a tool
// call with an `InputRequiredResult` and is retried. So there is no
// elicitation-driven permission prompt here, and a policy is what refuses —
// which is the shape #1334 asked for anyway. docs/design/mcp.md argues it.
package mcp

import (
	"encoding/json"
	"slices"

	"github.com/blairham/sh/internal/termhost"
)

// Version is the protocol revision this server implements. Every shape in this
// file was taken from that revision's published specification.
const Version = "2026-07-28"

// LegacyVersion is the revision answered to a client that opens with the
// `initialize` handshake instead. See the package comment for the measurement
// that put it here, and for why it is not a hedge.
const LegacyVersion = "2025-11-25"

// The methods this server answers. The notifications are taken and dropped;
// everything else is refused by name.
const (
	MethodDiscover    = "server/discover"
	MethodInitialize  = "initialize"
	MethodPing        = "ping"
	MethodToolsList   = "tools/list"
	MethodToolsCall   = "tools/call"
	MethodInitialized = "notifications/initialized"
	// The protocol spells this with two Ls; it is a wire value and not prose,
	// so it stays as the schema has it.
	MethodCancelled = "notifications/cancelled" //nolint:misspell // the protocol's spelling
	MethodProgress  = "notifications/progress"
)

// The `_meta` keys the revision reserves that this server reads or writes.
// They are namespaced and the namespace is part of the key, so they are
// written out once here rather than composed at each use.
//
// `io.modelcontextprotocol/clientInfo` is the third of the trio and is not
// here: a modern request carries it, and this server does nothing with what a
// client calls itself. The revision says as much about the mirror-image field
// — serverInfo is self-reported, for display and logging, and a client should
// not change its behavior on it.
const (
	MetaProtocolVersion = "io.modelcontextprotocol/protocolVersion"
	MetaServerInfo      = "io.modelcontextprotocol/serverInfo"
)

// CodeUnsupportedProtocolVersion is what a modern request naming a revision
// this server does not speak is answered with. It is the revision's own code,
// in the application range JSON-RPC reserves, and the client is expected to
// retry with something from the `supported` list rather than to give up.
const CodeUnsupportedProtocolVersion = -32022

// ResultComplete is the discriminator every modern result carries. A legacy
// result carries none, which is why the field is omitempty and set per era
// rather than written into each constructor.
const ResultComplete = "complete"

// Implementation names a side of the connection.
type Implementation struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

// Capabilities is what this server claims. Tools and nothing else; see the
// package comment for why each absence is an absence rather than a gap.
type Capabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

// ToolsCapability says whether the tool list changes. Ours does not — the five
// verbs are the five verbs — so listChanged is false and no subscription is
// served.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// meta is the request metadata both eras may carry.
//
// ProtocolVersion is the era discriminator: a modern request declares it and a
// legacy one has no field for it at all. ProgressToken is the client asking to
// be told what a long-running call is doing, and it is `any` because the
// revision says a token is a string *or* a number and an id that comes back
// with its type changed is not the id that was sent.
type meta struct {
	ProtocolVersion string          `json:"io.modelcontextprotocol/protocolVersion"`
	ProgressToken   json.RawMessage `json:"progressToken,omitempty"`
}

// withMeta is any request's params, read for nothing but its metadata. Decoded
// separately from the request's own shape because it is read before the method
// is dispatched: the version has to be checked whatever was asked for.
type withMeta struct {
	Meta meta `json:"_meta"`
}

// DiscoverResult answers `server/discover`: what this server speaks, what it
// can do, and what it is.
//
// The revision requires every server to implement this, and it is also the
// stdio backward-compatibility probe — a dual-era client sends it first and
// falls back to `initialize` on any error that is not a modern one. So a
// server that answered it with "method not found" would be telling every
// modern client that it is a legacy server.
type DiscoverResult struct {
	ResultType        string         `json:"resultType"`
	SupportedVersions []string       `json:"supportedVersions"`
	Capabilities      Capabilities   `json:"capabilities"`
	Instructions      string         `json:"instructions,omitempty"`
	Meta              map[string]any `json:"_meta,omitempty"`
}

// InitializeRequest is how a legacy client opens.
//
// Capabilities is kept as raw JSON for the reason ACP keeps `mcpServers` that
// way: decoding a shape this server will not act on would be pretending to
// support it, and refusing a request that carries one would break every client
// that always sends it.
type InitializeRequest struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    json.RawMessage `json:"capabilities,omitempty"`
	ClientInfo      *Implementation `json:"clientInfo,omitempty"`
}

// InitializeResult is the legacy handshake's answer. No resultType: that field
// belongs to the modern era and a legacy client validating against its own
// schema has no place to put it.
type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    Capabilities   `json:"capabilities"`
	ServerInfo      Implementation `json:"serverInfo"`
	Instructions    string         `json:"instructions,omitempty"`
}

// Tool is one verb, as the client is told about it.
type Tool struct {
	Name         string  `json:"name"`
	Title        string  `json:"title,omitempty"`
	Description  string  `json:"description"`
	InputSchema  Schema  `json:"inputSchema"`
	OutputSchema *Schema `json:"outputSchema,omitempty"`
}

// Schema is the part of JSON Schema a tool of ours needs. The revision defaults
// an unmarked schema to draft 2020-12, which is what these are.
type Schema struct {
	Type                 string              `json:"type"`
	Properties           map[string]Property `json:"properties,omitempty"`
	Required             []string            `json:"required,omitempty"`
	AdditionalProperties *bool               `json:"additionalProperties,omitempty"`
}

// Property is one field of a schema.
//
// Type is `any` because a nullable field is spelled as a list of two type
// names, and an exit status that is genuinely null half the time is the whole
// reason this server has an output schema at all.
type Property struct {
	Type        any       `json:"type"`
	Description string    `json:"description,omitempty"`
	Items       *Property `json:"items,omitempty"`
}

// ListToolsResult answers `tools/list`.
type ListToolsResult struct {
	ResultType string `json:"resultType,omitempty"`
	Tools      []Tool `json:"tools"`
}

// CallToolRequest is one tool invocation.
type CallToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// CallToolResult is what a tool produced.
//
// Content is what a model reads; StructuredContent is the same answer typed,
// against the tool's output schema. Both, because the revision says a tool that
// returns structured content should also return the serialized JSON in a text
// block, and because a client that validates the one still shows the other.
//
// IsError is the *tool* failing rather than the protocol: a command refused by
// policy, a terminal id that is not there. Those are things a model can correct
// and retry, which is exactly the line the revision draws between a tool
// execution error and a JSON-RPC one.
type CallToolResult struct {
	ResultType        string    `json:"resultType,omitempty"`
	Content           []Content `json:"content"`
	StructuredContent any       `json:"structuredContent,omitempty"`
	IsError           bool      `json:"isError,omitempty"`
}

// Content is one piece of a tool's unstructured answer. Text is the only
// variant a shell produces: an image, an audio clip and an embedded resource
// are all things this server has nothing to put in.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// TextContent wraps text as tool content.
func TextContent(text string) []Content { return []Content{{Type: "text", Text: text}} }

// ProgressNotification is what a long-running call reports while it runs.
//
// This is the streaming answer, and it is the whole of it: a command is a
// handle, and what it writes reaches the client here rather than in the result
// of the call that started it. The revision requires Progress to increase with
// every notification, which is why it counts bytes the command has *ever*
// written rather than what is currently retained — a buffer with a limit on it
// shrinks, and a progress value may not.
type ProgressNotification struct {
	ProgressToken json.RawMessage `json:"progressToken"`
	Progress      float64         `json:"progress"`
	Total         *float64        `json:"total,omitempty"`
	Message       string          `json:"message,omitempty"`
}

// CreateArgs is what `terminal_create` takes.
//
// Command is a command *line* by default, as the shell would run one, and Args
// is there for a peer that means an argv. Which is which is the same rule the
// shared machinery states: no args means a line. See termhost.
type CreateArgs struct {
	Command         string            `json:"command"`
	Args            []string          `json:"args,omitempty"`
	Cwd             string            `json:"cwd,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	OutputByteLimit *int              `json:"outputByteLimit,omitempty"`
}

// request is this protocol's create, said in the vocabulary the shared
// machinery takes. Only the environment changes shape — an object here, because
// that is how an MCP tool argument spells a mapping, and `NAME=VALUE` there.
func (a CreateArgs) request() termhost.Request {
	env := make([]string, 0, len(a.Env))
	for name, value := range a.Env {
		env = append(env, name+"="+value)
	}
	// Sorted, because a map's order is not one and a command's environment
	// must not depend on it: two entries writing the same name would take
	// turns winning.
	slices.Sort(env)
	return termhost.Request{
		Command:         a.Command,
		Args:            a.Args,
		Env:             env,
		Cwd:             a.Cwd,
		OutputByteLimit: a.OutputByteLimit,
	}
}

// TerminalArgs is what every tool after create takes: which terminal.
type TerminalArgs struct {
	TerminalID string `json:"terminalId"`
}

// CreateResult names the terminal that was made.
type CreateResult struct {
	TerminalID string `json:"terminalId"`
}
