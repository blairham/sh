// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"unicode/utf8"

	"github.com/blairham/sh/interp"
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
// What is still outside it is unchanged and worth restating: what the command
// then does is its own, exactly as approving `cat /secret` approves *starting*
// cat. Containment is docs/design/sandboxing.md.

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
type TerminalOutputResponse struct {
	Output     string              `json:"output"`
	Truncated  bool                `json:"truncated"`
	ExitStatus *TerminalExitStatus `json:"exitStatus,omitempty"`
}

// TerminalExitStatus is how a command ended: a code, or a signal, and never
// both. Both are pointers because the schema makes both nullable and the
// distinction is real — an exit code of zero is not the absence of one.
type TerminalExitStatus struct {
	ExitCode *int    `json:"exitCode"`
	Signal   *string `json:"signal"`
}

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

// terminal is one command the client is running for an agent.
type terminal struct {
	cmd   *exec.Cmd
	limit int

	mu        sync.Mutex
	output    []byte
	truncated bool
	exit      *TerminalExitStatus

	// done is closed once the command has been reaped, which is what
	// terminal/wait_for_exit waits on. A channel rather than a condition
	// variable because the wait must also end when the request's context does.
	done chan struct{}
}

// Write collects what the command produced.
//
// Both of the command's streams arrive here, because a terminal has one: an
// agent asking for output is asking what it would have seen on a screen, and
// separating them would be inventing a distinction the protocol does not have.
func (t *terminal) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.output = append(t.output, p...)
	t.trim()
	return len(p), nil
}

// trim keeps the retained output within the limit, dropping the oldest.
//
// The cut lands on a character boundary, which the protocol requires in so
// many words: an agent handed a string starting halfway through a rune has
// been handed something that is not text. Staying slightly under the limit is
// the price and is what the rule asks for.
func (t *terminal) trim() {
	if t.limit < 0 || len(t.output) <= t.limit {
		return
	}
	cut := len(t.output) - t.limit
	for cut < len(t.output) && !utf8.RuneStart(t.output[cut]) {
		cut++
	}
	// Copied rather than resliced. A reslice leaves the dropped prefix alive
	// in the array underneath, so a long-running command with a limit would
	// hold every byte it ever wrote — which is the one thing the limit exists
	// to prevent.
	t.output = append([]byte(nil), t.output[cut:]...)
	t.truncated = true
}

// reap waits for the command and records how it ended.
//
// Wait's error is deliberately not read: a non-zero exit is not a failure of
// anything here, and the process state says what happened in the protocol's
// own terms. What is left of an error it could return — the command could not
// be started — is answered at create, where there is a request to fail.
func (t *terminal) reap() {
	_ = t.cmd.Wait()
	status := exitStatus(t.cmd)
	t.mu.Lock()
	t.exit = &status
	t.mu.Unlock()
	close(t.done)
}

// snapshot is the output and the exit status as they stand.
func (t *terminal) snapshot() TerminalOutputResponse {
	t.mu.Lock()
	defer t.mu.Unlock()
	return TerminalOutputResponse{
		Output:     string(t.output),
		Truncated:  t.truncated,
		ExitStatus: t.exit,
	}
}

// exitStatus reads how a command ended: a code, or a signal, never both.
func exitStatus(cmd *exec.Cmd) TerminalExitStatus {
	state := cmd.ProcessState
	if state == nil {
		// Nothing to read it from, which is a failure of ours rather than of
		// the command. Reported as a code so that the agent is told something
		// definite rather than a null pair it has to guess about.
		code := -1
		return TerminalExitStatus{ExitCode: &code}
	}
	if ws, ok := state.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		name := signalName(ws.Signal())
		return TerminalExitStatus{Signal: &name}
	}
	code := state.ExitCode()
	return TerminalExitStatus{ExitCode: &code}
}

// signalName is the protocol's string for a signal.
//
// The shell's own signal vocabulary lives in interp and stays there; this is a
// *presentation* of a number for one field of one protocol, so it is a short
// table with a numeric fallback rather than a second copy of that vocabulary.
// A signal with no name here is reported by number rather than dropped, on the
// same rule as an event kind a consumer does not know.
func signalName(s syscall.Signal) string {
	switch s {
	case syscall.SIGHUP:
		return "SIGHUP"
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGQUIT:
		return "SIGQUIT"
	case syscall.SIGILL:
		return "SIGILL"
	case syscall.SIGABRT:
		return "SIGABRT"
	case syscall.SIGFPE:
		return "SIGFPE"
	case syscall.SIGKILL:
		return "SIGKILL"
	case syscall.SIGSEGV:
		return "SIGSEGV"
	case syscall.SIGPIPE:
		return "SIGPIPE"
	case syscall.SIGALRM:
		return "SIGALRM"
	case syscall.SIGTERM:
		return "SIGTERM"
	}
	return "SIG" + strconv.Itoa(int(s))
}

// The client's register of running terminals.
//
// Keyed by the id this client minted and not by session: an agent names a
// terminal by that id alone in every method after create, so a second index by
// session would be a thing to keep in step for no question it answers.

func (c *Client) addTerminal(t *terminal) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running == nil {
		c.running = map[string]*terminal{}
	}
	c.nextTerminal++
	id := "term-" + strconv.Itoa(c.nextTerminal)
	c.running[id] = t
	return id
}

// named reads the two fields every terminal method after create carries, and
// finds the terminal. An id this client never issued is invalid params rather
// than an internal error: the agent asked about something that does not exist.
func (c *Client) named(params json.RawMessage, method string) (*terminal, error) {
	var req TerminalRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, Errorf(CodeInvalidParams, "%s: %v", method, err)
	}
	c.mu.Lock()
	t, ok := c.running[req.TerminalID]
	c.mu.Unlock()
	if !ok {
		return nil, Errorf(CodeInvalidParams, "%s: no terminal %q", method, req.TerminalID)
	}
	return t, nil
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
		return nil, Errorf(CodeMethodNotFound, "this client does not serve %s", MethodCreateTerminal)
	}
	var req CreateTerminalRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, Errorf(CodeInvalidParams, "%s: %v", MethodCreateTerminal, err)
	}
	if req.Command == "" {
		return nil, Errorf(CodeInvalidParams, "%s: no command", MethodCreateTerminal)
	}
	argv := append([]string{req.Command}, req.Args...)
	if !c.Boundary.Exec(ctx, req.Command, argv) {
		// A refused create is a command that never started, and the agent is
		// told so rather than handed a terminal id naming nothing — which it
		// would then poll for output that will never come.
		return nil, Errorf(CodeInvalidParams, "%s: permission denied", req.Command)
	}
	cmd := exec.CommandContext(ctx, req.Command, req.Args...)
	cmd.Dir = req.Cwd
	// Inherited and then written over rather than replaced. A command started
	// with only the agent's few variables has no PATH, so every create would
	// fail for a reason nothing on the wire explains.
	cmd.Env = os.Environ()
	for _, e := range req.Env {
		cmd.Env = append(cmd.Env, e.Name+"="+e.Value)
	}
	t := &terminal{cmd: cmd, limit: -1, done: make(chan struct{})}
	if req.OutputByteLimit != nil {
		t.limit = *req.OutputByteLimit
	}
	// One buffer for both, because a terminal has one stream: an agent asking
	// for output is asking what a person would have seen on a screen.
	cmd.Stdout, cmd.Stderr = t, t
	if err := cmd.Start(); err != nil {
		c.failed(ctx, interp.Action{Kind: interp.ActionExec, Path: req.Command, Args: argv}, err)
		return nil, Errorf(CodeInvalidParams, "%s: %v", req.Command, err)
	}
	go t.reap()
	return CreateTerminalResponse{TerminalID: c.addTerminal(t)}, nil
}

// terminalOutput is what the command has written so far.
func (c *Client) terminalOutput(params json.RawMessage) (any, error) {
	t, err := c.named(params, MethodTerminalOutput)
	if err != nil {
		return nil, err
	}
	return t.snapshot(), nil
}

// waitForExit blocks until the command ends.
//
// Or until the connection does, which is the only other way out: there is no
// deadline here for the same reason there is none on a permission request. A
// wait that gave up and reported an exit that had not happened would tell the
// agent something untrue about a process that is still running.
func (c *Client) waitForExit(ctx context.Context, params json.RawMessage) (any, error) {
	t, err := c.named(params, MethodWaitForExit)
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, Errorf(CodeCancelled, "%s: %v", MethodWaitForExit, ctx.Err())
	case <-t.done:
	}
	status := t.snapshot().ExitStatus
	return WaitForTerminalExitResponse{ExitCode: status.ExitCode, Signal: status.Signal}, nil
}

// killTerminal ends the command and keeps the terminal, so that its output can
// still be read.
//
// Gated as a signal, which is what it is: the agent reaching a running process
// it chose to reach. That is the ActionSignal case exactly, and it is the half
// of this that release is not.
func (c *Client) killTerminal(ctx context.Context, params json.RawMessage) (any, error) {
	t, err := c.named(params, MethodKillTerminal)
	if err != nil {
		return nil, err
	}
	pid := 0
	if t.cmd.Process != nil {
		pid = t.cmd.Process.Pid
	}
	if !c.Boundary.Signal(ctx, pid, syscall.SIGKILL) {
		return nil, Errorf(CodeInvalidParams, "%s: permission denied", MethodKillTerminal)
	}
	t.kill()
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
	var req TerminalRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, Errorf(CodeInvalidParams, "%s: %v", MethodReleaseTerminal, err)
	}
	c.mu.Lock()
	t, ok := c.running[req.TerminalID]
	delete(c.running, req.TerminalID)
	c.mu.Unlock()
	if !ok {
		return nil, Errorf(CodeInvalidParams, "%s: no terminal %q", MethodReleaseTerminal, req.TerminalID)
	}
	if t.cmd.Process != nil {
		// Recorded through the boundary rather than beside it, so that the
		// record carries the run and the action it belongs to exactly as a
		// gated one does. A trace line with no id is a line nothing joins to.
		c.Boundary.Record(ctx, interp.Action{
			Kind: interp.ActionSignal, PID: t.cmd.Process.Pid, Signal: syscall.SIGKILL,
		})
	}
	t.kill()
	return ReleaseTerminalResponse{}, nil
}

// kill ends the command if it has not ended already. A process that has been
// reaped is gone, and Kill on it is an error about nothing.
func (t *terminal) kill() {
	select {
	case <-t.done:
		return
	default:
	}
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
}
