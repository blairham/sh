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
	"strings"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acp"
	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/repl"
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
// says comes back on standard output.
//
// Authentication is the first thing that happens rather than the last: two of
// the three published agents refuse a session with -32000 until it has, so
// `-acp-auth ID` names one of the methods the agent advertised and settles it
// between the handshake and the session. Which method is a person's choice and
// there is no default, because a client that picked a credential path on
// somebody's behalf would be guessing about their account.
//
// Terminal auth is the half a shell is unusually well placed to serve, and it
// is not a message: the client runs the agent's own command *again*, with the
// method's extra arguments, on the terminal this shell is already attached to.
// That is why it is offered only when this process has one — see terminalAuth.
//
// What is deliberately still not here is the surface a person answers a
// permission request from: they are settled by -acp-allow rather than by
// asking. docs/design/acp.md has it as the next step.

// connectACP drives an agent and returns the status to exit with.
func connectACP(sh driver.Shell, allow bool, authMethod string, argv []string) int {
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
		// Nil where this process has no terminal, which is also what withholds
		// the capability: an agent is told we can run a terminal login only
		// where we can.
		Relaunch: terminalAuth(argv, os.Stdin, os.Stdout),
	}
	client.Connect(fromAgent, toAgent)
	go func() { _ = client.Serve(ctx) }()

	status := talk(ctx, client, authMethod)
	_ = toAgent.Close()
	_ = agent.Wait()
	return status
}

// listAuth writes out what the agent said it would accept.
//
// The kind is named beside the id, because it decides what a person has to do:
// an agent method opens something of the agent's own and a terminal method
// hands them a login here. Naming the flag is the other half — a list of ids
// with no way to use one is a diagnostic that stops short.
func listAuth(methods []acp.AuthMethod) {
	if len(methods) == 0 {
		fmt.Fprintln(os.Stderr, "sh:   it advertised no authentication methods")
		return
	}
	for _, m := range methods {
		fmt.Fprintf(os.Stderr, "sh:   %s [%s] (%s): %s\n", m.ID, m.Kind(), m.Name, m.Description)
	}
	fmt.Fprintf(os.Stderr, "sh: choose one with -acp-auth %s\n", methods[0].ID)
}

func fail1(format string, args ...any) int {
	fmt.Fprintf(os.Stderr, "sh: acp: "+format+"\n", args...)
	return exitFailure
}

// terminalAuth reproduces the agent's own invocation for a terminal login.
//
// Nil when in and out are not both a terminal, and that is the whole of the
// capability decision: the schema says a client should claim terminal
// authentication only where it can reproduce the agent's invocation
// interactively, and a shell reading its prompts from a pipe cannot. Claiming
// it anyway would have the agent offer a person a login that draws nothing.
//
// Both streams are checked rather than one. A login TUI reads keystrokes *and*
// draws, and a `sh -acp-connect … < script` has a terminal on exactly one of
// the two.
func terminalAuth(argv []string, in, out *os.File) func(context.Context, []string, map[string]string) error {
	if !repl.IsTerminal(in) || !repl.IsTerminal(out) {
		return nil
	}
	return func(ctx context.Context, args []string, env map[string]string) error {
		login := loginCommand(ctx, argv, args, env)
		// The person's own terminal, which is the point: a login TUI wants
		// keystrokes and this is the process holding them.
		login.Stdin, login.Stdout, login.Stderr = in, out, os.Stderr
		fmt.Fprintf(os.Stderr, "sh: starting %s to log in\n", strings.Join(login.Args, " "))
		// A zero exit status signals success and any other termination signals
		// failure, which is exactly what a non-nil error from Run is.
		return login.Run()
	}
}

// loginCommand is the second invocation a terminal method asks for: the same
// program with the same arguments, plus the method's, and its environment over
// the top.
//
// A second process rather than something done to the running agent — the one
// on the wire keeps its stdio, which is the protocol — and the agent's own
// arguments are kept, because the method's are described as additional to the
// configured invocation rather than as a replacement for it.
func loginCommand(ctx context.Context, argv, args []string, env map[string]string) *exec.Cmd {
	line := append(append([]string{}, argv[1:]...), args...)
	login := exec.CommandContext(ctx, argv[0], line...)
	// Appended after the inherited environment, because os/exec keeps the last
	// of a repeated name — which is what "these values override same-named
	// variables in the base launch configuration" asks for.
	login.Env = os.Environ()
	for k, v := range env {
		login.Env = append(login.Env, k+"="+v)
	}
	return login
}

// talk does the handshake, authenticates if asked to, and then relays prompts
// until the input ends.
func talk(ctx context.Context, client *acp.Client, authMethod string) int {
	info, err := client.Initialize(ctx)
	if err != nil {
		return fail1("initialize: %v", err)
	}
	name := "the agent"
	if info.AgentInfo != nil {
		name = info.AgentInfo.Name + " " + info.AgentInfo.Version
	}
	fmt.Fprintf(os.Stderr, "sh: connected to %s, protocol %d\n", name, info.ProtocolVersion)

	if authMethod != "" {
		if err := client.Authenticate(ctx, authMethod); err != nil {
			fmt.Fprintf(os.Stderr, "sh: %s did not authenticate: %v\n", name, err)
			listAuth(info.AuthMethods)
			return exitFailure
		}
	}

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
			listAuth(info.AuthMethods)
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
