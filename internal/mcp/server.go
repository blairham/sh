// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/blairham/sh/driver"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/event"
	"github.com/blairham/sh/internal/jsonrpc"
	"github.com/blairham/sh/internal/termhost"
)

// The shell as an MCP server: a coding agent launches it, asks it to run
// things, and everything it is asked for crosses this shell's boundary.
//
// This is a **fourth consumer of the same seam**, beside `-c`, a script file,
// the prompt and `-acp`. It adds no question to the interpreter — it answers
// the two that are already there. docs/design/acp.md opens by saying the gate
// and the event stream "were built for three consumers at once: a sandbox, an
// AI assistant, and an agent protocol", and this is the fourth arriving with
// no new machinery.
//
// The role-specific part is thin on purpose. Everything below the dispatch —
// starting a command, holding it as a handle, gating it, recording it — is
// internal/termhost, shared with the ACP client direction rather than written
// twice. See that package, and docs/design/mcp.md for the argument.

// Server answers MCP on one connection.
type Server struct {
	// Info is what this server calls itself to a client.
	Info Implementation

	conn *jsonrpc.Conn
	host *termhost.Host
}

// NewServer builds one on a shell template.
//
// The Shell is the invocation's own — its dialect, its gate, its event sink,
// its startup files — so a `--policy` on the same command line governs every
// command a client asks for, exactly as it governs a script. That is the whole
// of why an MCP server closes the gap #1334 records: an MCP server is
// configured with a *user-controlled command line*, where `$SHELL -c` has
// nowhere to put a flag.
func NewServer(sh driver.Shell, info Implementation) *Server {
	// A shell that is a protocol server must not stop being one. termhost's
	// Interpreter sets KeepProcess for the commands it runs, which is the half
	// that matters here: an `exec` inside a client's command line would
	// otherwise replace the process serving the connection.
	return &Server{
		Info: info,
		host: &termhost.Host{
			// The same gate and sink a shell here would have been given, so
			// `--policy` and `-trace-events` reach a client's commands exactly
			// as they reach a script's.
			//
			// The session id is the front end's, and this route makes its own
			// where the invocation carried none: nothing here goes through
			// driver's own session naming, so without one every record of what
			// a client was allowed would belong to no run.
			Boundary:  boundary.Boundary{Gate: sh.Gate, Events: sh.Events, Session: runID(sh)},
			Interpret: termhost.Interpreter(sh),
		},
	}
}

func runID(sh driver.Shell) string {
	if sh.Session != "" {
		return sh.Session
	}
	return event.NewID(time.Now())
}

// Serve reads MCP on r, writes it on w, and returns when the input ends.
//
// r and w are the process's standard input and output when this is a binary,
// and they are not the shell's: the revision says a server must write nothing
// to standard output that is not a protocol message, and a shell's whole job
// is writing to standard output. What a command writes is held in its terminal
// and reaches the client as a tool result or as a progress notification.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	s.conn = jsonrpc.NewConn(r, w, s)
	return s.conn.Serve(ctx)
}

// Close ends every terminal this server is still holding.
func (s *Server) Close() { s.host.Close() }

// Handle answers a request.
//
// There is no "initialize first" check, and its absence is the revision rather
// than an omission: a modern request is self-contained and carries its own
// version, so refusing one for arriving before a handshake would be enforcing
// a handshake that no longer exists. A legacy client sends `initialize` first
// because its own revision tells it to.
func (s *Server) Handle(ctx context.Context, method string, params json.RawMessage) (any, error) {
	m := readMeta(params)
	if m.ProtocolVersion != "" && m.ProtocolVersion != Version {
		// A modern client naming a revision we do not speak. The revision
		// requires this exact error with the list, because the client's next
		// move is to pick one off it and retry rather than to give up.
		return nil, unsupported(m.ProtocolVersion)
	}
	modern := m.ProtocolVersion != ""
	switch method {
	case MethodDiscover:
		return s.discover(), nil
	case MethodInitialize:
		return s.initialize(params)
	case MethodPing:
		// An empty result, which is the whole of what a ping is.
		return struct{}{}, nil
	case MethodToolsList:
		return s.listTools(modern), nil
	case MethodToolsCall:
		return s.callTool(ctx, params, m, modern)
	}
	// Everything else, deliberately. Resources, prompts, completion, logging
	// and subscriptions are capabilities this server does not claim, and a
	// server that answered a method it never advertised would be telling the
	// client something untrue about what it can rely on.
	return nil, jsonrpc.Errorf(jsonrpc.CodeMethodNotFound, "%s is not served by a shell", method)
}

// Notify takes a notification, which has no answer and cannot fail.
//
// Both of the two that arrive are taken and dropped, and that is the honest
// state rather than a stub. `notifications/initialized` is a legacy client
// saying the handshake is over, and there is nothing this server was waiting
// to do. The cancellation notification names a request id, and this transport
// hands a handler the *connection's* context rather than one per request — so
// there is nothing here that could be canceled, and a client that wants a
// command stopped has a tool for exactly that. See docs/design/mcp.md.
func (s *Server) Notify(_ context.Context, method string, _ json.RawMessage) {
	switch method {
	case MethodInitialized, MethodCancelled:
		// Both are named rather than swept up by the fallthrough, so that a
		// reader can see they arrive and are answered with nothing on
		// purpose. A silent default would read the same for a notification
		// nobody thought about.
	}
}

// readMeta reads the request metadata out of any params, including none.
func readMeta(params json.RawMessage) meta {
	var m withMeta
	if len(params) == 0 {
		return meta{}
	}
	if err := json.Unmarshal(params, &m); err != nil {
		// Params this server cannot read at all are answered by whichever
		// handler tries to read them properly, in that method's own words.
		// Reading no metadata out of them is the legacy reading, which is the
		// one that then fails loudly rather than the one that refuses a
		// version nobody named.
		return meta{}
	}
	return m.Meta
}

// unsupported is the revision's own error for a version this server does not
// speak, carrying what it does.
func unsupported(requested string) error {
	data, err := json.Marshal(map[string]any{
		"supported": []string{Version},
		"requested": requested,
	})
	if err != nil {
		return jsonrpc.Errorf(jsonrpc.CodeInternalError, "%v", err)
	}
	return &jsonrpc.Error{
		Code:    CodeUnsupportedProtocolVersion,
		Message: "Unsupported protocol version",
		Data:    data,
	}
}

// discover answers the modern probe.
func (s *Server) discover() DiscoverResult {
	return DiscoverResult{
		ResultType:        ResultComplete,
		SupportedVersions: []string{Version},
		Capabilities:      Capabilities{Tools: &ToolsCapability{}},
		Instructions:      Instructions,
		Meta:              map[string]any{MetaServerInfo: s.Info},
	}
}

// initialize answers the legacy handshake.
//
// It answers with the revision *this server* speaks rather than echoing the
// client's, which is what the legacy lifecycle asks for: the client then either
// proceeds or disconnects. Measured against Claude Code 2.1.267, which asks
// for 2025-11-25 and proceeds happily against a server answering something
// else — so echoing would have been untestable agreement rather than
// agreement.
func (s *Server) initialize(params json.RawMessage) (any, error) {
	var req InitializeRequest
	if len(params) > 0 {
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "initialize: %v", err)
		}
	}
	return InitializeResult{
		ProtocolVersion: LegacyVersion,
		Capabilities:    Capabilities{Tools: &ToolsCapability{}},
		ServerInfo:      s.Info,
		Instructions:    Instructions,
	}, nil
}
