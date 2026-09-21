// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/blairham/sh/internal/jsonrpc"
	"github.com/blairham/sh/internal/termhost"
)

// terminal/* on the client side: a command the agent runs is a command *we*
// start.
//
// It is the fs/* argument one step further, and the sharper half of it. An
// agent that cannot ask a client to run something runs it itself, with its own
// fork and its own exec, and no gate anywhere sees the argv — which defeats
// the point of putting a shell's policy on a coding agent in the first place.
// Advertising these methods is how the agent's commands are pulled inside the
// boundary: `terminal/create` is an interp.ActionExec through the same gate a
// script's command passes, into the same audit trail, and a refusal is a
// command that never started.
//
// **The mechanism is not here.** internal/termhost holds it, because the MCP
// server front end serves the same five verbs and two copies of them is how
// they drift — see that package's doc comment and docs/design/mcp.md. What is
// left in this file is ACP's wire shapes and the dispatch, which is the only
// part that is this protocol's.

// The client methods an agent calls to run something. Answered here rather
// than called: this shell's agent side deliberately does not use a client's
// terminal, for the reason the client side implements one.
const (
	MethodCreateTerminal  = "terminal/create"
	MethodTerminalOutput  = "terminal/output"
	MethodWaitForExit     = "terminal/wait_for_exit"
	MethodKillTerminal    = "terminal/kill"
	MethodReleaseTerminal = "terminal/release"
)

// CreateTerminalRequest asks the client to run a command and keep it.
//
// Cwd is absolute where it is given at all, and absent means the client's own
// directory. OutputByteLimit is how much output to retain, and a pointer
// because zero is a limit — "keep nothing" — and absent is no limit at all.
type CreateTerminalRequest struct {
	SessionID       string        `json:"sessionId"`
	Command         string        `json:"command"`
	Args            []string      `json:"args,omitempty"`
	Env             []EnvVariable `json:"env,omitempty"`
	Cwd             string        `json:"cwd,omitempty"`
	OutputByteLimit *int          `json:"outputByteLimit,omitempty"`
}

// EnvVariable is one name and one value. A list rather than an object, which
// is the schema's shape and not ours to tidy.
type EnvVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// request is this protocol's create, said in the vocabulary the machinery
// takes. Only the environment changes shape — a list of pairs here, `NAME=VALUE`
// there, because that is what an exec and an interpreter both want.
func (r CreateTerminalRequest) request() termhost.Request {
	env := make([]string, 0, len(r.Env))
	for _, e := range r.Env {
		env = append(env, e.Name+"="+e.Value)
	}
	return termhost.Request{
		Command:         r.Command,
		Args:            r.Args,
		Env:             env,
		Cwd:             r.Cwd,
		OutputByteLimit: r.OutputByteLimit,
	}
}

// CreateTerminalResponse names the terminal that was made.
type CreateTerminalResponse struct {
	TerminalID string `json:"terminalId"`
}

// TerminalRequest is the shape of every terminal method after create: which
// session, which terminal. Output, wait, kill and release all take exactly
// this, so it is one type rather than four identical ones.
type TerminalRequest struct {
	SessionID  string `json:"sessionId"`
	TerminalID string `json:"terminalId"`
}

// TerminalOutputResponse is what the command has written so far, and how it
// ended if it has.
//
// An alias rather than a copy: the shape the machinery already answers in is
// the shape this protocol asks for, field for field, and a second struct with
// the same JSON tags is a second place for a tag to be wrong.
type TerminalOutputResponse = termhost.Output

// TerminalExitStatus is how a command ended: a code, or a signal, and never
// both. Both are pointers because the schema makes both nullable and the
// distinction is real — an exit code of zero is not the absence of one.
//
// An alias, for the reason above.
type TerminalExitStatus = termhost.ExitStatus

// WaitForTerminalExitResponse carries the same two fields, flat rather than
// nested. The schema spells it out separately and so does this, because the
// two shapes are not the same on the wire even though they say the same thing.
type WaitForTerminalExitResponse struct {
	ExitCode *int    `json:"exitCode"`
	Signal   *string `json:"signal"`
}

// KillTerminalResponse and ReleaseTerminalResponse are empty, and are objects
// rather than nothing for the reason the write response is.
type (
	KillTerminalResponse    struct{}
	ReleaseTerminalResponse struct{}
)

// A TerminalCommand is one command line an agent asked this client to run, as
// terminal/create described it. An alias of the machinery's own, so that an
// Interpret wired for one front end is the same func type as one wired for the
// other.
type TerminalCommand = termhost.Command

// terminals is the register of commands this client is running for the agent.
//
// Built on first use rather than in a constructor because a Client is a struct
// literal: Boundary and Interpret are set by the caller after the value exists
// and before anything is served, and a host built at Connect would be built
// before a test that calls Handle directly ever connects.
func (c *Client) terminals() *termhost.Host {
	c.once.Do(func() {
		c.host = &termhost.Host{Boundary: c.Boundary, Interpret: c.Interpret}
	})
	return c.host
}

// createTerminal runs a command for the agent, through the gate.
//
// The context is the connection's rather than the request's, and that is the
// right lifetime rather than an oversight: the terminal outlives the call that
// made it — the agent comes back to read its output and wait for it — and it
// ends when the connection does, so an agent that disconnects mid-command does
// not leave one running.
func (c *Client) createTerminal(ctx context.Context, params json.RawMessage) (any, error) {
	if !c.Terminals {
		return nil, jsonrpc.Errorf(jsonrpc.CodeMethodNotFound, "this client does not serve %s", MethodCreateTerminal)
	}
	var req CreateTerminalRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "%s: %v", MethodCreateTerminal, err)
	}
	id, err := c.terminals().Create(ctx, req.request())
	if err != nil {
		// Every one of these is the agent having asked for something it
		// cannot have — nothing to run, a program the policy refuses, a name
		// that is not a program — rather than a fault of ours, so all three
		// are invalid params. A refused create in particular is a command
		// that never started, and the agent is told so rather than handed a
		// terminal id naming nothing, which it would then poll for output
		// that will never come.
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "%s: %v", MethodCreateTerminal, err)
	}
	return CreateTerminalResponse{TerminalID: id}, nil
}

// named reads the terminal id every method after create carries. An id this
// client never issued is invalid params rather than an internal error: the
// agent asked about something that does not exist.
func named(params json.RawMessage, method string) (string, error) {
	var req TerminalRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return "", jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "%s: %v", method, err)
	}
	return req.TerminalID, nil
}

// terminalOutput is what the command has written so far.
func (c *Client) terminalOutput(params json.RawMessage) (any, error) {
	id, err := named(params, MethodTerminalOutput)
	if err != nil {
		return nil, err
	}
	out, err := c.terminals().Output(id)
	if err != nil {
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "%s: %v", MethodTerminalOutput, err)
	}
	return out, nil
}

// waitForExit blocks until the command ends.
//
// Or until the connection does, which is the only other way out: there is no
// deadline here for the same reason there is none on a permission request. A
// wait that gave up and reported an exit that had not happened would tell the
// agent something untrue about a process that is still running.
func (c *Client) waitForExit(ctx context.Context, params json.RawMessage) (any, error) {
	id, err := named(params, MethodWaitForExit)
	if err != nil {
		return nil, err
	}
	status, err := c.terminals().Wait(ctx, id)
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return nil, jsonrpc.Errorf(jsonrpc.CodeCancelled, "%s: %v", MethodWaitForExit, err)
	case err != nil:
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "%s: %v", MethodWaitForExit, err)
	}
	return WaitForTerminalExitResponse{ExitCode: status.ExitCode, Signal: status.Signal}, nil
}

// killTerminal ends the command and keeps the terminal, so that its output can
// still be read.
//
// Gated as a signal, which is what it is: the agent reaching a running process
// it chose to reach. That is the ActionSignal case exactly, and it is the half
// of this that release is not.
func (c *Client) killTerminal(ctx context.Context, params json.RawMessage) (any, error) {
	id, err := named(params, MethodKillTerminal)
	if err != nil {
		return nil, err
	}
	if err := c.terminals().Kill(ctx, id); err != nil {
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "%s: %v", MethodKillTerminal, err)
	}
	return KillTerminalResponse{}, nil
}

// releaseTerminal ends the command if it is still running and forgets it.
//
// Recorded and *not* gated, and the difference from kill has to be argued
// rather than assumed, because both end a process. Release is the protocol's
// only way for an agent to say "I am finished with this", and the signal
// inside it is the client ending something the client started — the same rule
// this package's boundary already draws, that an access is inside it when the
// thing was chosen by whoever the policy is about. Refusing a release would
// also leave this client holding a process forever with the agent given no
// other way out, which is a worse boundary than an honest record.
func (c *Client) releaseTerminal(ctx context.Context, params json.RawMessage) (any, error) {
	id, err := named(params, MethodReleaseTerminal)
	if err != nil {
		return nil, err
	}
	if err := c.terminals().Release(ctx, id); err != nil {
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "%s: %v", MethodReleaseTerminal, err)
	}
	return ReleaseTerminalResponse{}, nil
}
