// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acp"
	"github.com/blairham/sh/internal/boundary"
)

// `sh -acp-connect CMD...`: the other direction. This shell launches a coding
// agent and is the environment it runs inside.
//
// The reason to do it this way round rather than let the agent read files for
// itself is the whole thesis of the gate: an agent under an editor opens what
// it likes and nobody sees it, and an agent under this shell can be told to
// ask — so `sh -deny /etc -acp-connect …` refuses the agent a directory, and
// `-trace-events` shows every file it was given. That is a policy on an agent,
// enforced by a shell, which is a thing no editor can offer.
//
// The agent is launched exactly as the registry says to launch it:
//
//	sh -acp-connect npx @google/gemini-cli --acp
//	sh -acp-connect npx @agentclientprotocol/claude-agent-acp
//	sh -acp-connect npx @agentclientprotocol/codex-acp
//
// Prompts are read from standard input, a line at a time, and what the agent
// says comes back on standard output. What is deliberately *not* here yet is
// the surface a person answers from: permission requests are settled by
// -acp-allow rather than by asking, and an agent that needs authenticating is
// reported rather than authenticated. docs/design/acp.md has both as the next
// step, and Terminal Auth — where the client relaunches the agent in an
// interactive terminal — is the one a shell is unusually well placed to serve.

// connectACP drives an agent and returns the status to exit with.
func connectACP(sh driver.Shell, allow bool, argv []string) int {
	if len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "sh: -acp-connect needs the command that starts an agent")
		return exitFailure
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agent := exec.CommandContext(ctx, argv[0], argv[1:]...)
	toAgent, err := agent.StdinPipe()
	if err != nil {
		return fail1("agent input: %v", err)
	}
	fromAgent, err := agent.StdoutPipe()
	if err != nil {
		return fail1("agent output: %v", err)
	}
	logs, err := agent.StderrPipe()
	if err != nil {
		return fail1("agent diagnostics: %v", err)
	}
	if err := agent.Start(); err != nil {
		return fail1("%s: %v", argv[0], err)
	}
	// Forwarded rather than ignored. An agent that will not start says why
	// there — measured, an agent that cannot authenticate writes its reason
	// to standard error and answers a bare code on the wire.
	go acp.Stderr(logs, os.Stderr, "agent: ")

	client := &acp.Client{
		Info: acp.Implementation{Name: "sh", Title: "sh", Version: version},
		// The point of the exercise: the agent's file access is ours to
		// gate, and only if we offer to do it for them.
		Files: true,
		// The same gate and sink a shell here would have been given, so
		// -deny and -trace-events reach the agent's accesses exactly as they
		// reach a script's.
		Boundary: boundary.Boundary{Gate: sh.Gate, Events: sh.Events},
		Answer:   fixedAnswer(allow),
		Update:   renderUpdate,
	}
	client.Connect(fromAgent, toAgent)
	go func() { _ = client.Serve(ctx) }()

	status := talk(ctx, client)
	_ = toAgent.Close()
	_ = agent.Wait()
	return status
}

func fail1(format string, args ...any) int {
	fmt.Fprintf(os.Stderr, "sh: acp: "+format+"\n", args...)
	return exitFailure
}

// talk does the handshake and then relays prompts until the input ends.
func talk(ctx context.Context, client *acp.Client) int {
	info, err := client.Initialize(ctx)
	if err != nil {
		return fail1("initialize: %v", err)
	}
	name := "the agent"
	if info.AgentInfo != nil {
		name = info.AgentInfo.Name + " " + info.AgentInfo.Version
	}
	fmt.Fprintf(os.Stderr, "sh: connected to %s, protocol %d\n", name, info.ProtocolVersion)

	wd, err := os.Getwd()
	if err != nil {
		return fail1("where are we: %v", err)
	}
	session, err := client.NewSession(ctx, wd)
	if err != nil {
		if acp.AuthRequired(err) {
			// Not a failure of ours, and the commonest first answer there
			// is: two of the three published agents refuse a session until
			// they have been authenticated. Naming the methods it offers is
			// the useful half, since which one applies decides what a person
			// has to do about it.
			fmt.Fprintf(os.Stderr, "sh: %s needs authenticating first: %v\n", name, err)
			for _, m := range info.AuthMethods {
				fmt.Fprintf(os.Stderr, "sh:   %s (%s): %s\n", m.ID, m.Name, m.Description)
			}
			return exitFailure
		}
		return fail1("session/new: %v", err)
	}

	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		stop, err := client.Prompt(ctx, session, in.Text())
		if err != nil {
			return fail1("session/prompt: %v", err)
		}
		if stop != acp.StopEndTurn {
			fmt.Fprintf(os.Stderr, "sh: turn ended: %s\n", stop)
		}
	}
	if err := in.Err(); err != nil && err != io.EOF {
		return fail1("reading a prompt: %v", err)
	}
	return 0
}

// fixedAnswer settles every permission request the same way.
//
// A placeholder for a person, and it is deliberately a bad one: allow-once for
// everything or reject-once for everything, chosen at the command line, with
// refusal the default. What it is not is a *silent* default — the question and
// the answer both go to standard error, so a session run this way still leaves
// the record that a person would have been shown.
func fixedAnswer(allow bool) func(context.Context, acp.RequestPermissionRequest) (acp.PermissionOutcome, error) {
	option := acp.OptionRejectOnce
	if allow {
		option = acp.OptionAllowOnce
	}
	return func(_ context.Context, req acp.RequestPermissionRequest) (acp.PermissionOutcome, error) {
		fmt.Fprintf(os.Stderr, "sh: agent asks: %s -> %s\n", callName(req.ToolCall), option)
		return acp.PermissionOutcome{Outcome: acp.OutcomeSelected, OptionID: option}, nil
	}
}

func callName(c acp.ToolCall) string {
	if c.Title != "" {
		return c.Title
	}
	return c.ToolCallID
}

// renderUpdate puts what the agent said where a person can read it.
//
// Content goes to standard output because it is the answer; everything else
// goes to standard error because it is about the answer. A kind this shell
// does not know is *shown* rather than dropped, which is the same rule the
// audit schema states for a name a consumer has not seen: the useful default
// is to record it and carry on.
func renderUpdate(n acp.SessionNotification) {
	raw, ok := n.Update.(json.RawMessage)
	if !ok {
		return
	}
	var u struct {
		SessionUpdate string           `json:"sessionUpdate"`
		Content       acp.ContentBlock `json:"content"`
		Title         string           `json:"title"`
		Status        string           `json:"status"`
		ToolCallID    string           `json:"toolCallId"`
	}
	if err := json.Unmarshal(raw, &u); err != nil {
		return
	}
	switch u.SessionUpdate {
	case acp.UpdateAgentMessageChunk:
		// Nothing useful to do about a failed write to the stream we would
		// report it on.
		_, _ = fmt.Fprint(os.Stdout, u.Content.Text)
	case acp.UpdateToolCall, acp.UpdateToolCallUpdate:
		name := u.Title
		if name == "" {
			name = u.ToolCallID
		}
		fmt.Fprintf(os.Stderr, "sh: agent %s: %s\n", u.Status, name)
	default:
		fmt.Fprintf(os.Stderr, "sh: agent %s\n", u.SessionUpdate)
	}
}
