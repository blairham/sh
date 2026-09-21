// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/blairham/sh/internal/jsonrpc"
	"github.com/blairham/sh/internal/termhost"
)

// The five verbs, and why they are five.
//
// A shell command is four things at once — a stream, an exit status, two
// output channels, and possibly standard input — and an MCP tool call is one
// request and one result. #1338 records the three honest mappings and the
// maintainer's decision between them, on 2026-09-21: **mirror ACP's terminal
// model**. A long-running command is a handle the client polls or is notified
// about, and "return the whole output as a string" is rejected, because it is
// the answer that looks fine until somebody runs `tail -f` — it either blocks
// forever or truncates, and truncating a command's output while returning a
// success status is the silent-wrong-answer class this repository treats as
// its worst failure.
//
// So these are `terminal/create`, `terminal/output`, `terminal/wait_for_exit`,
// `terminal/kill` and `terminal/release` under names MCP's own rules allow,
// over one implementation with the ACP client — internal/termhost — rather
// than a second copy of the verbs.
//
// Underscores rather than the dots the naming rules also permit. Dots are
// legal in this revision and were not always, and a client that rejects a tool
// name it cannot parse rejects it silently, leaving a server that connects and
// serves nothing. Underscores have never not been allowed.
const (
	ToolCreate  = "terminal_create"
	ToolOutput  = "terminal_output"
	ToolWait    = "terminal_wait_for_exit"
	ToolKill    = "terminal_kill"
	ToolRelease = "terminal_release"
)

// Instructions is what a client tells a model about this server.
//
// It says the shape rather than the verbs, because the verbs are in the tool
// descriptions and a model that reads both should not be told two versions of
// one thing. What it has to say is the part no single tool description can: a
// command is a handle, and reading its output is a second call.
const Instructions = "This is a shell. " +
	"Starting a command returns a terminal id rather than the command's output: " +
	"call " + ToolWait + " to wait for it, " + ToolOutput + " to read what it wrote, " +
	ToolKill + " to stop it and " + ToolRelease + " when you are finished with it. " +
	"Pass a progress token on " + ToolWait + " to be sent output as it is produced. " +
	"Commands run under whatever policy this shell was started with, and one may be refused."

// progressInterval is how often a wait reports what a command has written.
//
// A tenth of a second: fast enough that `tail -f` reaches a person as it
// happens, slow enough that a command writing a byte at a time does not become
// one notification per byte. The revision asks both sides to rate-limit and
// this is ours; a notification is only sent when there is something new, so an
// idle command costs nothing at all.
const progressInterval = 100 * time.Millisecond

// Tools is what this server advertises, in a fixed order.
//
// Deterministic, which the revision asks for so that a client can cache the
// list: the order is the order a command is driven in, which is also the order
// somebody reading it would want.
func Tools() []Tool {
	object := func(props map[string]Property, required ...string) Schema {
		return Schema{Type: "object", Properties: props, Required: required}
	}
	terminal := object(map[string]Property{
		"terminalId": {Type: "string", Description: "The id " + ToolCreate + " returned."},
	}, "terminalId")
	// The empty object an acknowledgement is. Written once because all three
	// acknowledging tools answer it, and three copies of `{}` is three places
	// for one of them to grow a field the others do not have.
	no := false
	acknowledged := Schema{Type: "object", AdditionalProperties: &no}
	exitStatus := map[string]Property{
		"exitCode": {
			Type:        []string{"integer", "null"},
			Description: "The status the command exited with, or null if a signal ended it.",
		},
		"signal": {
			Type:        []string{"string", "null"},
			Description: "The signal that ended the command, or null if it exited.",
		},
	}
	return []Tool{
		{
			Name:  ToolCreate,
			Title: "Run a command",
			Description: "Start a shell command and return a terminal id for it. " +
				"The command does not have to have finished — read its output with " +
				ToolOutput + " and wait for it with " + ToolWait + ". " +
				"By default `command` is a command line this shell interprets, so " +
				"pipelines, redirection and variables all work; pass `args` instead " +
				"to run one program with exactly those arguments and no shell.",
			InputSchema: object(map[string]Property{
				"command": {Type: "string", Description: "The command line to run, or the program when args is given."},
				"args": {
					Type:        "array",
					Items:       &Property{Type: "string"},
					Description: "Arguments to run the program with. Giving any means command is a program rather than a command line.",
				},
				"cwd": {Type: "string", Description: "An absolute directory to run in. Defaults to the shell's own."},
				"env": {Type: "object", Description: "Environment variables, written over the shell's own."},
				"outputByteLimit": {
					Type:        "integer",
					Description: "How much output to retain. Oldest is dropped past it. No limit by default.",
				},
			}, "command"),
			OutputSchema: schema(object(map[string]Property{
				"terminalId": {Type: "string", Description: "Name this in every other terminal tool."},
			}, "terminalId")),
		},
		{
			Name:  ToolOutput,
			Title: "Read a command's output",
			Description: "What a command has written so far, and how it ended if it has. " +
				"Both of its output streams, as they would have appeared on a screen.",
			InputSchema: terminal,
			OutputSchema: schema(object(map[string]Property{
				"output":    {Type: "string", Description: "What the command has written."},
				"truncated": {Type: "boolean", Description: "Whether an output byte limit dropped anything."},
				"exitStatus": {
					Type:        []string{"object", "null"},
					Description: "How the command ended, or null while it is still running.",
				},
			}, "output", "truncated")),
		},
		{
			Name:  ToolWait,
			Title: "Wait for a command to finish",
			Description: "Block until a command ends, and answer how it ended. " +
				"Send a progress token with this call to be notified of what the " +
				"command writes as it writes it; read the whole of it with " + ToolOutput + ".",
			InputSchema:  terminal,
			OutputSchema: schema(object(exitStatus)),
		},
		{
			Name:         ToolKill,
			Title:        "Stop a command",
			Description:  "Kill a running command. Its terminal stays, so its output can still be read.",
			InputSchema:  terminal,
			OutputSchema: schema(acknowledged),
		},
		{
			Name:  ToolRelease,
			Title: "Finish with a command",
			Description: "Forget a terminal, killing its command if it is still running. " +
				"Its output cannot be read afterwards.",
			InputSchema:  terminal,
			OutputSchema: schema(acknowledged),
		},
	}
}

func schema(s Schema) *Schema { return &s }

func (s *Server) listTools(modern bool) ListToolsResult {
	return ListToolsResult{ResultType: resultType(modern), Tools: Tools()}
}

// resultType is the discriminator a modern result carries and a legacy one
// does not.
//
// Written once rather than at each constructor, and it is a real distinction
// rather than a nicety: a legacy client validating a result against its own
// revision's schema has no `resultType` in it, and a modern one requires the
// field.
func resultType(modern bool) string {
	if modern {
		return ResultComplete
	}
	return ""
}

// callTool runs one tool and wraps what it produced.
func (s *Server) callTool(ctx context.Context, params json.RawMessage, m meta, modern bool) (any, error) {
	var req CallToolRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "%s: %v", MethodToolsCall, err)
	}
	out, err := s.call(ctx, req.Name, req.Arguments, m)
	if err != nil {
		var rpcErr *jsonrpc.Error
		if errors.As(err, &rpcErr) {
			// A protocol error: an unknown tool, or arguments that are not the
			// shape the schema describes. The revision keeps these as JSON-RPC
			// errors because a model is unlikely to fix one by retrying.
			return nil, err
		}
		// A tool execution error: the gate refused, the terminal is not there,
		// the program does not exist. The revision wants these *in the result*
		// with isError set, because they are exactly what a model can act on —
		// and a client is told to hand them to the model for that reason.
		return CallToolResult{
			ResultType: resultType(modern),
			Content:    TextContent(err.Error()),
			IsError:    true,
		}, nil
	}
	return s.result(out, modern)
}

// call is the dispatch.
//
// A switch over named constants rather than a table of closures, so that
// internal/gateguard can read which tool reaches which handler out of the
// source — the guard that makes "this tool is gated" a checked statement
// rather than a remembered one. See gateguard_test.go in this package.
func (s *Server) call(ctx context.Context, name string, args json.RawMessage, m meta) (any, error) {
	switch name {
	case ToolCreate:
		return s.createTerminal(ctx, args)
	case ToolOutput:
		return s.terminalOutput(args)
	case ToolWait:
		return s.waitForExit(ctx, args, m)
	case ToolKill:
		return s.killTerminal(ctx, args)
	case ToolRelease:
		return s.releaseTerminal(ctx, args)
	}
	return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "unknown tool: %s", name)
}

// result renders a tool's answer as both a text block and structured content.
//
// Both, for the reason CallToolResult gives: the revision says a tool
// returning structured content should also return the serialized JSON in a
// text block, and a client that reads only one of the two is a client we broke
// for no reason.
func (s *Server) result(out any, modern bool) (any, error) {
	text, err := json.Marshal(out)
	if err != nil {
		return nil, jsonrpc.Errorf(jsonrpc.CodeInternalError, "%v", err)
	}
	return CallToolResult{
		ResultType:        resultType(modern),
		Content:           TextContent(string(text)),
		StructuredContent: out,
	}, nil
}

// createTerminal starts a command, through the gate.
func (s *Server) createTerminal(ctx context.Context, args json.RawMessage) (any, error) {
	var a CreateArgs
	if err := decode(args, &a, ToolCreate); err != nil {
		return nil, err
	}
	id, err := s.host.Create(ctx, a.request())
	if err != nil {
		return nil, err
	}
	return CreateResult{TerminalID: id}, nil
}

// terminalOutput is what a command has written so far.
func (s *Server) terminalOutput(args json.RawMessage) (any, error) {
	a, err := terminalArgs(args, ToolOutput)
	if err != nil {
		return nil, err
	}
	return s.host.Output(a.TerminalID)
}

// waitForExit blocks until a command ends, streaming what it writes if the
// client asked to be told.
//
// The streaming is a *progress* notification and it rides this call, which is
// the whole of the decision on #1338: the revision allows a server to report
// progress only against a request that is in flight, so the wait is the
// request the stream belongs to. A client that sends no token gets the same
// answer with nothing in between, and `terminal_output` still has everything.
func (s *Server) waitForExit(ctx context.Context, args json.RawMessage, m meta) (any, error) {
	a, err := terminalArgs(args, ToolWait)
	if err != nil {
		return nil, err
	}
	if len(m.ProgressToken) > 0 {
		if t, ok := s.host.Lookup(a.TerminalID); ok {
			s.stream(ctx, t, m.ProgressToken)
		}
	}
	// Asked of the host either way, so that an id that is not there is
	// answered by the one place that knows — and so that a stream which ended
	// because the connection did still reports the connection rather than an
	// exit that did not happen.
	status, err := s.host.Wait(ctx, a.TerminalID)
	if err != nil {
		return nil, err
	}
	return status, nil
}

// stream reports what a command writes until it ends, and returns.
//
// It blocks rather than running on a goroutine of its own, and that is
// deliberate: the notifications may only reference a request that is still in
// flight, so the goroutine that must outlive them is the one handling the
// call. jsonrpc handles every inbound request on its own goroutine, so
// blocking here holds nothing else up.
func (s *Server) stream(ctx context.Context, t *termhost.Terminal, token json.RawMessage) {
	tick := time.NewTicker(progressInterval)
	defer tick.Stop()
	sent := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.Done():
			// One last report, because the command's final write and its exit
			// race: a tick that lands a moment early would leave the last line
			// of output to be found by terminal_output alone.
			s.report(t, token, &sent)
			return
		case <-tick.C:
			s.report(t, token, &sent)
		}
	}
}

// report sends one progress notification, if there is anything new to say.
//
// Nothing is sent when nothing was written, and that is required rather than
// polite: the revision says a progress value must *increase* with every
// notification, so a tick with no new bytes has no honest number to send.
func (s *Server) report(t *termhost.Terminal, token json.RawMessage, sent *int) {
	text, now := t.Since(*sent)
	if now <= *sent || s.conn == nil {
		return
	}
	*sent = now
	// Nothing useful to do about a failed notification except stop trying;
	// the connection is where a failure would have been reported.
	_ = s.conn.Notify(MethodProgress, ProgressNotification{
		ProgressToken: token,
		Progress:      float64(now),
		Message:       text,
	})
}

// killTerminal stops a command and keeps its terminal.
func (s *Server) killTerminal(ctx context.Context, args json.RawMessage) (any, error) {
	a, err := terminalArgs(args, ToolKill)
	if err != nil {
		return nil, err
	}
	if err := s.host.Kill(ctx, a.TerminalID); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

// releaseTerminal forgets a terminal, killing its command if it is running.
func (s *Server) releaseTerminal(ctx context.Context, args json.RawMessage) (any, error) {
	a, err := terminalArgs(args, ToolRelease)
	if err != nil {
		return nil, err
	}
	if err := s.host.Release(ctx, a.TerminalID); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

// terminalArgs reads the one argument every tool after create takes.
//
// An absent id is a protocol error rather than a tool error, because the
// schema requires it: a call without it did not satisfy the shape the client
// was given, which is the line the revision draws.
func terminalArgs(args json.RawMessage, tool string) (TerminalArgs, error) {
	var a TerminalArgs
	if err := decode(args, &a, tool); err != nil {
		return a, err
	}
	if a.TerminalID == "" {
		return a, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "%s: no terminalId", tool)
	}
	return a, nil
}

// decode reads a tool's arguments, tolerating a call that sent none.
func decode(args json.RawMessage, into any, tool string) error {
	if len(args) == 0 {
		return nil
	}
	if err := json.Unmarshal(args, into); err != nil {
		return jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "%s: %v", tool, err)
	}
	return nil
}
