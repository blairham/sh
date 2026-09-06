// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acp"
	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/event"
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
// A person answers here, where a person actually is. A permission request the
// agent makes is put to them at the terminal, and so is an elicitation — the
// protocol's own way of asking a human for something that is not permission.
// -acp-allow still answers everything without asking, and a run with no
// terminal refuses everything, because nobody to ask is a denial.

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

	// One reader for the whole session: the prompt loop and every question an
	// agent asks a person take their lines from it, and never at the same
	// time. Two scanners on one descriptor would each buffer whatever they
	// read, so a line meant for one would sit unread inside the other.
	in := bufio.NewScanner(os.Stdin)
	tty := repl.IsTerminal(os.Stdin) && repl.IsTerminal(os.Stderr)

	client := &acp.Client{
		Info: acp.Implementation{Name: "sh", Title: "sh", Version: version},
		// The point of the exercise: the agent's file access is ours to
		// gate, and only if we offer to do it for them.
		Files: true,
		// And the sharper half of it. An agent not told it can ask us to run
		// something runs it itself, and there is no argv for any gate to see.
		Terminals: true,
		// The same gate and sink a shell here would have been given, so
		// -deny and -trace-events reach the agent's accesses exactly as they
		// reach a script's.
		//
		// The session is the front end's, and this route makes its own: nothing
		// here goes through driver, which is where a shell's run is normally
		// named, so without one every record of what the agent was allowed
		// would belong to no run.
		Boundary: boundary.Boundary{Gate: sh.Gate, Events: sh.Events, Session: runID(sh)},
		Update:   renderUpdate,
		// The person is at this end of the connection, and this is the one
		// reader they answer on — the prompt loop's own, shared rather than
		// duplicated. See answerer for why that is safe.
		Answer: answerer(allow, tty, in),
		Elicit: form(tty, in),
		// Nil where this process has no terminal, which is also what withholds
		// the capability: an agent is told we can run a terminal login only
		// where we can.
		Relaunch: terminalAuth(argv, os.Stdin, os.Stdout),
	}
	client.Connect(fromAgent, toAgent)
	go func() { _ = client.Serve(ctx) }()

	status := talk(ctx, client, authMethod, in)
	_ = toAgent.Close()
	_ = agent.Wait()
	return status
}

// runID is what this run of the shell is called in the record.
//
// A caller that named one is left alone, which is the same courtesy driver
// extends; otherwise one is minted here, by the same generator, so that an
// audit stream from `-acp-connect` joins on the same field as one from a
// script.
func runID(sh driver.Shell) string {
	if sh.Session != "" {
		return sh.Session
	}
	return event.NewID(time.Now())
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
func talk(ctx context.Context, client *acp.Client, authMethod string, in *bufio.Scanner) int {
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

	for in.Scan() {
		askedBefore, saidBefore := client.Commands()
		stop, err := client.Prompt(ctx, session, in.Text())
		if err != nil {
			return fail1("session/prompt: %v", err)
		}
		if stop != acp.StopEndTurn {
			fmt.Fprintf(os.Stderr, "sh: turn ended: %s\n", stop)
		}
		asked, said := client.Commands()
		fmt.Fprint(os.Stderr, commandCoverage(asked-askedBefore, said-saidBefore))
	}
	if err := in.Err(); err != nil && err != io.EOF {
		return fail1("reading a prompt: %v", err)
	}
	return 0
}

// commandCoverage is what to tell a person about the half of this they did not
// get, and it is empty whenever they got all of it.
//
// The claim `-acp-connect` makes is that a shell's policy reaches a coding
// agent, and it does — for everything the agent *asks this shell for*. A
// command it runs in its own process is its own fork and its own exec, and no
// gate anywhere sees the argv. Nothing here can change that: an agent outside
// our boundary is outside it in exactly the way any allowed exec is once it
// has started.
//
// What can change is whether anyone is told. Measured against the published
// adapters, two of the three run commands themselves and call no client method
// at all, so this is the ordinary case rather than the corner: a person who
// read "under the same policy" and got file access alone should not have to
// find that out by reading a trace.
//
// Both numbers are things this client saw, and neither is inferred from the
// other — there is no id joining an agent's tool call to a terminal it asked
// us for, and #719 already declined to invent one. So the counts are reported
// and the reader draws the conclusion. Silence where the agent asked for at
// least as many as it reported: there is nothing to warn about, and a notice
// that fires on a clean run is a notice people learn to skip.
func commandCoverage(asked, announced int) string {
	if announced <= asked {
		return ""
	}
	if asked == 0 {
		return fmt.Sprintf(
			"sh: the agent reported %d command(s) this turn and asked this shell to run none.\n"+
				"sh: what an agent runs in its own process passes no gate — the policy covered\n"+
				"sh: the files it asked for, and not the commands it ran.\n", announced)
	}
	return fmt.Sprintf(
		"sh: the agent reported %d command(s) this turn and asked this shell to run %d.\n"+
			"sh: what an agent runs in its own process passes no gate.\n", announced, asked)
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

// answerer decides who settles the agent's permission requests.
//
// Three arrangements, in the order they are preferred, and the last is the one
// that has to be the default: nobody to ask is a denial.
//
//  1. -acp-allow answers allow-once to everything without asking. The question
//     and the answer still go to standard error, so a run made this way leaves
//     the record a person would have been shown.
//  2. A terminal: the person is asked, and their answer is one of the options
//     the agent offered.
//  3. Neither: reject-once to everything.
//
// The reader is the prompt loop's own scanner, shared rather than duplicated,
// and that is safe by the shape of a turn rather than by luck: talk reads a
// line, hands it to session/prompt, and blocks there until the turn ends. A
// permission request only arrives *during* a turn, so the scanner is idle
// exactly when a question needs it. Two readers on one descriptor would race
// for bytes and lose lines to whichever won.
//
// It is a line read rather than repl's line editor, which is worth saying
// because this shell has one. What repl exports is Shell.Run — a whole prompt
// loop that owns the terminal, the history and the shell it drives — and there
// is no single-line entry point to borrow. Taking the terminal into raw mode
// for a one-word answer, in the middle of a turn whose output is still
// arriving on the same screen, would be the worse answer rather than the
// better one.
func answerer(allow, tty bool, in *bufio.Scanner) func(context.Context, acp.RequestPermissionRequest) (acp.PermissionOutcome, error) {
	if allow || !tty {
		return fixedAnswer(allow)
	}
	return func(_ context.Context, req acp.RequestPermissionRequest) (acp.PermissionOutcome, error) {
		fmt.Fprintf(os.Stderr, "\nsh: the agent asks to: %s\n", callName(req.ToolCall))
		for _, o := range req.Options {
			fmt.Fprintf(os.Stderr, "sh:   %s  %s\n", letter(o.Kind), o.Name)
		}
		fmt.Fprint(os.Stderr, "sh: your answer, or nothing to refuse: ")
		if !in.Scan() {
			// The input ended with the question outstanding, which is not an
			// answer. Everything that is not an explicit allow is a denial.
			fmt.Fprintln(os.Stderr, "\nsh: no answer — refused")
			return refusal(), nil
		}
		id, ok := chosen(strings.TrimSpace(in.Text()), req.Options)
		if !ok {
			fmt.Fprintln(os.Stderr, "sh: not one of the options — refused")
			return refusal(), nil
		}
		return acp.PermissionOutcome{Outcome: acp.OutcomeSelected, OptionID: id}, nil
	}
}

// letter is what a person types for an option, derived from its *kind* rather
// than from its id.
//
// The ids in a request are the agent's, not ours: this shell's own agent side
// spells them allow-once and reject-always, and another agent may spell them
// anything at all. The kind is the protocol's own vocabulary and is the same
// four values for everyone, so keying on it is what makes one keystroke mean
// the same thing whichever agent asked.
func letter(kind string) string {
	switch kind {
	case acp.KindAllowOnce:
		return "a"
	case acp.KindAllowAlways:
		return "A"
	case acp.KindRejectOnce:
		return "r"
	case acp.KindRejectAlway:
		return "R"
	}
	return "?"
}

// chosen turns what a person typed into one of the options that were offered.
//
// A letter, or the option id itself for anybody who prefers to type it. An
// answer that matches neither is not an answer, and the caller refuses.
//
// Nothing is typed is checked first and once, rather than beside the letter
// comparison where it would be dead — no kind maps to the empty string, so
// only the *id* comparison can be reached by an empty answer. It can be: an
// agent that offered an option with an empty id would have it chosen by
// somebody who pressed return, which is the "an option id we never offered"
// rule failing from the other end.
func chosen(typed string, options []acp.PermissionOption) (string, bool) {
	if typed == "" {
		return "", false
	}
	for _, o := range options {
		if typed == o.OptionID || typed == letter(o.Kind) {
			return o.OptionID, true
		}
	}
	return "", false
}

// refusal is what this client sends when nobody said yes.
//
// reject-once rather than reject-always: a refusal that was not chosen must not
// be remembered as though it had been.
func refusal() acp.PermissionOutcome {
	return acp.PermissionOutcome{Outcome: acp.OutcomeSelected, OptionID: acp.OptionRejectOnce}
}

// form puts an elicitation to the person, one line per field.
//
// Nil where there is no terminal, and that nil is what withholds the
// capability: an agent is told this client can collect a form only where it
// can. The same rule the terminal login and the file methods are held to.
func form(tty bool, in *bufio.Scanner) func(context.Context, acp.CreateElicitationRequest) (acp.CreateElicitationResponse, error) {
	if !tty {
		return nil
	}
	return func(_ context.Context, req acp.CreateElicitationRequest) (acp.CreateElicitationResponse, error) {
		fmt.Fprintf(os.Stderr, "\nsh: the agent asks: %s\n", req.Message)
		content := map[string]any{}
		if req.RequestedSchema == nil {
			return acp.CreateElicitationResponse{Action: acp.ElicitAccept}, nil
		}
		for name, p := range req.RequestedSchema.Properties {
			value, ok := field(in, name, p)
			if !ok {
				// A field the person would not or could not fill in. Whether
				// that ends the form depends on whether the agent said it had
				// to be there.
				if required(req.RequestedSchema, name) {
					fmt.Fprintln(os.Stderr, "sh: declined")
					return acp.CreateElicitationResponse{Action: acp.ElicitDecline}, nil
				}
				continue
			}
			content[name] = value
		}
		return acp.CreateElicitationResponse{Action: acp.ElicitAccept, Content: content}, nil
	}
}

// required reports whether the agent said a field has to be there.
func required(schema *acp.ElicitationSchema, name string) bool {
	return slices.Contains(schema.Required, name)
}

// field asks for one value and reads it back as the type the schema declared.
//
// A value that will not read as its type is asked for again rather than
// refused: a mistyped number is a slip and not a decision, and turning it into
// a decline would answer the agent something the person did not mean. An empty
// line, or the end of input, is the decision — and it is the only one that
// leaves the field unset.
func field(in *bufio.Scanner, name string, p acp.ElicitationProperty) (any, bool) {
	label := name
	if p.Title != "" {
		label = p.Title
	}
	for {
		fmt.Fprintf(os.Stderr, "sh:   %s%s: ", label, hint(p))
		if !in.Scan() {
			fmt.Fprintln(os.Stderr)
			return nil, false
		}
		typed := strings.TrimSpace(in.Text())
		if typed == "" {
			return nil, false
		}
		value, err := coerce(typed, p)
		if err == nil {
			return value, true
		}
		fmt.Fprintf(os.Stderr, "sh:   %v\n", err)
	}
}

// hint says what a field will take, where that is not obvious from its name.
func hint(p acp.ElicitationProperty) string {
	if len(p.Enum) > 0 {
		return " (" + strings.Join(p.Enum, "/") + ")"
	}
	switch p.Type {
	case acp.PropertyBoolean:
		return " (yes/no)"
	case acp.PropertyInteger, acp.PropertyNumber:
		return " (a number)"
	}
	return ""
}

// coerce reads a typed line as the property's declared type.
//
// The multi-select case — an array property — is deliberately absent: a
// terminal line is a poor multi-select, and this client does not claim to
// serve one.
func coerce(typed string, p acp.ElicitationProperty) (any, error) {
	if len(p.Enum) > 0 {
		if !slices.Contains(p.Enum, typed) {
			return nil, fmt.Errorf("one of %s", strings.Join(p.Enum, ", "))
		}
		return typed, nil
	}
	switch p.Type {
	case acp.PropertyBoolean:
		switch strings.ToLower(typed) {
		case "y", "yes", "true", "1":
			return true, nil
		case "n", "no", "false", "0":
			return false, nil
		}
		return nil, errors.New("yes or no")
	case acp.PropertyInteger:
		n, err := strconv.Atoi(typed)
		if err != nil {
			return nil, errors.New("a whole number")
		}
		return n, nil
	case acp.PropertyNumber:
		f, err := strconv.ParseFloat(typed, 64)
		if err != nil {
			return nil, errors.New("a number")
		}
		return f, nil
	}
	// Anything else is taken as written, which is what a string is and what an
	// unknown type is best treated as: a client that refused a type it had not
	// seen would fail on a schema that grows.
	return typed, nil
}
