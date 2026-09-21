// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package termhost runs commands for a protocol peer and keeps them as
// handles.
//
// It is the machinery behind ACP's `terminal/create`, `terminal/output`,
// `terminal/wait_for_exit`, `terminal/kill` and `terminal/release`, and behind
// the MCP tools that mean the same five things. **One implementation with two
// front doors**, which is the constraint the MCP work was accepted under: two
// protocol front ends over one seam is this repository's duplicate-rule
// hazard, and a copy of the terminal verbs is how they drift. The front ends
// hold their own wire shapes and nothing else; everything below the dispatch
// is here.
//
// The seam itself is older than either of them. docs/design/acp.md opens by
// saying the gate and the event stream "were built for three consumers at
// once: a sandbox, an AI assistant, and an agent protocol"; this is what the
// third and fourth consumers reach it through.
//
// # What a terminal is for
//
// An agent that cannot ask a shell to run something runs it itself, with its
// own fork and its own exec, and no gate anywhere sees the argv — which
// defeats the point of putting a shell's policy on a coding agent at all.
// Serving these verbs is how the agent's commands are pulled inside the
// boundary: a create is an interp.ActionExec through the same gate a script's
// command passes, into the same audit trail, and a refusal is a command that
// never started.
//
// What the command then does is its own, exactly as approving `cat /secret`
// approves *starting* cat. Containment is docs/design/sandboxing.md.
package termhost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"unicode/utf8"

	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/internal/boundary"
)

// The three ways a terminal method fails that a front end has to word in its
// own protocol's vocabulary. Everything else is the operating system's own
// error, wrapped and passed through.
var (
	// ErrNoTerminal is an id this host never issued, or one already released.
	// A front end answers it as bad parameters rather than as an internal
	// failure: the peer asked about something that does not exist.
	ErrNoTerminal = errors.New("no such terminal")

	// ErrRefused is the gate saying no. A refused create is a command that
	// never started, and the peer is told so rather than handed a handle
	// naming nothing — which it would then poll for output that will never
	// come.
	ErrRefused = errors.New("permission denied")

	// ErrNoCommand is a create with nothing to run.
	ErrNoCommand = errors.New("no command")
)

// Command is one command *line* a peer asked this shell to run, as a create
// described it: the line itself, the directory it asked for (empty is the
// host's own), and the environment it is to run with, already merged over the
// process's.
type Command struct {
	Line string
	Dir  string
	Env  []string
}

// ExitStatus is how a command ended: a code, or a signal, and never both.
//
// Both are pointers because the wire shapes of both protocols make both
// nullable and the distinction is real — an exit code of zero is not the
// absence of one.
type ExitStatus struct {
	ExitCode *int    `json:"exitCode"`
	Signal   *string `json:"signal"`
}

// Output is what a command has written so far, and how it ended if it has.
type Output struct {
	Output     string      `json:"output"`
	Truncated  bool        `json:"truncated"`
	ExitStatus *ExitStatus `json:"exitStatus,omitempty"`
}

// Request is one command to start.
//
// Cwd is absolute where it is given at all, and empty means the host's own
// directory. Env is `NAME=VALUE` entries written *over* the process's own
// environment rather than replacing it: a command started with only a peer's
// few variables has no PATH, so every create would fail for a reason nothing
// on the wire explains.
//
// OutputByteLimit is how much output to retain, and a pointer because zero is
// a limit — "keep nothing" — and absent is no limit at all.
type Request struct {
	Command         string
	Args            []string
	Env             []string
	Cwd             string
	OutputByteLimit *int
}

// Host is the register of commands this shell is running for a peer.
//
// Keyed by the id it minted and by nothing else: a peer names a terminal by
// that id alone in every verb after create, so a second index would be a thing
// to keep in step for no question it answers.
type Host struct {
	// Boundary is the gate and the sink a command passes through. The zero
	// value allows everything and records nothing, which is what a shell
	// without a policy is.
	Boundary boundary.Boundary

	// Interpret runs a command *line* the way this shell runs one, writing
	// everything it produces to out, and returning its exit status.
	//
	// It is what a create uses when the peer sent no args, which is what every
	// agent measured does when it means a shell command. Nil exec's the
	// command as a filename instead, which is what the ACP client did before
	// #1782 and which no measured agent could get a command out of.
	//
	// A func rather than something this package does, because interpreting
	// shell is the whole of the rest of this program and a protocol-support
	// package that reached into it would be the dependency pointing the wrong
	// way. Interpreter builds the one every front end here wants.
	Interpret func(ctx context.Context, cmd Command, out io.Writer) int

	// mu guards the terminals. They are reached from more than one goroutine
	// by construction: every inbound request is handled on its own, so a peer
	// may be creating one while it waits on another.
	mu      sync.Mutex
	running map[string]*Terminal
	next    int
	asked   int
}

// Asked is how many creates this host served, allowed or refused.
//
// Counted about the *route* rather than about the outcome: a peer that asks
// and is refused took the route, and a refusal is already a record of its own.
// The ACP client reports this beside what an agent *announced* having run, and
// the gap between the two is #786.
func (h *Host) Asked() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.asked
}

// Create runs a command for the peer, through the gate, and returns the id of
// the terminal holding it.
//
// The context is the connection's rather than one request's, and that is the
// right lifetime rather than an oversight: the terminal outlives the call that
// made it — the peer comes back to read its output and wait for it — and it
// ends when the connection does, so a peer that disconnects mid-command does
// not leave one running.
func (h *Host) Create(ctx context.Context, req Request) (string, error) {
	if req.Command == "" {
		return "", ErrNoCommand
	}
	// Counted here rather than after the gate. See Asked.
	h.mu.Lock()
	h.asked++
	h.mu.Unlock()
	// No args means the peer sent a command *line* rather than an argv, and
	// interpreting it is both what it asked for and the whole of the thesis:
	// every exec, open and stat inside the line crosses this shell's boundary,
	// where an exec of the line as a filename crosses it once, as a name that
	// does not exist. Measured — Claude Code 0.16.2 sends `printf "%s" "$0"; ps
	// -o args= -p $$` as the command with no args at all, and every one of them
	// failed as "executable file not found" until this branch existed (#1782).
	//
	// The ambiguous case is a *filename* containing a space, sent with no args,
	// which a line reads as a command and its argument. Neither protocol gives
	// a way to say which was meant; a peer that means an argv can say so by
	// sending one, and every agent measured that means a line sends no args.
	// Interpreting is also the reading that fails safe: the gate sees more, not
	// less.
	if len(req.Args) == 0 && h.Interpret != nil {
		return h.interpret(ctx, req), nil
	}
	argv := append([]string{req.Command}, req.Args...)
	if !h.Boundary.Exec(ctx, req.Command, argv) {
		return "", fmt.Errorf("%s: %w", req.Command, ErrRefused)
	}
	cmd := exec.CommandContext(ctx, req.Command, req.Args...)
	cmd.Dir = req.Cwd
	cmd.Env = append(os.Environ(), req.Env...)
	t := newTerminal(req.OutputByteLimit)
	t.cmd = cmd
	// One buffer for both, because a terminal has one stream: a peer asking
	// for output is asking what a person would have seen on a screen.
	cmd.Stdout, cmd.Stderr = t, t
	if err := cmd.Start(); err != nil {
		h.Boundary.Failed(ctx, interp.Action{Kind: interp.ActionExec, Path: req.Command, Args: argv}, err)
		return "", fmt.Errorf("%s: %w", req.Command, err)
	}
	go t.reap()
	return h.add(t), nil
}

// interpret runs a command line as this shell runs one.
//
// The gate is deliberately *not* consulted here on the line, and that is the
// point rather than an omission: a line is not an exec, and asking the boundary
// to rule on `echo hi > f` as though it were a path would be inventing an
// access nothing performs. What the interpreter does instead is cross the
// boundary for each real exec, open and stat inside it — the same Gate and the
// same event sink this shell was built with, because Interpret is wired from
// the same driver.Shell. So a policy reaches *further* on this route than on
// the exec one, not less far: `-deny exec:/bin/rm` refuses the `rm` inside a
// pipeline that an argv-level gate would have seen only as `/bin/sh`.
func (h *Host) interpret(ctx context.Context, req Request) string {
	t := newTerminal(req.OutputByteLimit)
	// Cancellation rather than a signal, because there is no process of ours
	// to signal. The context is the connection's, so a terminal still dies
	// with the connection exactly as an exec'd one does.
	run, cancel := context.WithCancel(ctx)
	t.cancel = cancel
	env := append(os.Environ(), req.Env...)
	id := h.add(t)
	go func() {
		defer cancel()
		code := h.Interpret(run, Command{Line: req.Command, Dir: req.Cwd, Env: env}, t)
		t.finish(ExitStatus{ExitCode: &code})
	}()
	return id
}

// Output is what the named command has written so far.
func (h *Host) Output(id string) (Output, error) {
	t, ok := h.Lookup(id)
	if !ok {
		return Output{}, fmt.Errorf("%q: %w", id, ErrNoTerminal)
	}
	return t.Snapshot(), nil
}

// Wait blocks until the named command ends.
//
// Or until the context does, which is the only other way out: there is no
// deadline here for the same reason there is none on a permission request. A
// wait that gave up and reported an exit that had not happened would tell the
// peer something untrue about a process that is still running.
func (h *Host) Wait(ctx context.Context, id string) (ExitStatus, error) {
	t, ok := h.Lookup(id)
	if !ok {
		return ExitStatus{}, fmt.Errorf("%q: %w", id, ErrNoTerminal)
	}
	select {
	case <-ctx.Done():
		return ExitStatus{}, ctx.Err()
	case <-t.Done():
	}
	s := t.Snapshot().ExitStatus
	return *s, nil
}

// Kill ends the named command and keeps the terminal, so that its output can
// still be read.
//
// Gated as a signal, which is what it is: the peer reaching a running process
// it chose to reach. That is the ActionSignal case exactly, and it is the half
// of this that Release is not.
func (h *Host) Kill(ctx context.Context, id string) error {
	t, ok := h.Lookup(id)
	if !ok {
		return fmt.Errorf("%q: %w", id, ErrNoTerminal)
	}
	pid := 0
	if t.cmd != nil && t.cmd.Process != nil {
		pid = t.cmd.Process.Pid
	}
	if !h.Boundary.Signal(ctx, pid, syscall.SIGKILL) {
		return fmt.Errorf("%q: %w", id, ErrRefused)
	}
	t.Kill()
	return nil
}

// Release ends the named command if it is still running and forgets it.
//
// Recorded and *not* gated, and the difference from Kill has to be argued
// rather than assumed, because both end a process. Release is the only way a
// peer has of saying "I am finished with this", and the signal inside it is the
// host ending something the host started — the same rule internal/boundary
// already draws, that an access is inside the boundary when the thing was
// chosen by whoever the policy is about. Refusing a release would also leave
// this host holding a process forever with the peer given no other way out,
// which is a worse boundary than an honest record.
func (h *Host) Release(ctx context.Context, id string) error {
	h.mu.Lock()
	t, ok := h.running[id]
	delete(h.running, id)
	h.mu.Unlock()
	if !ok {
		return fmt.Errorf("%q: %w", id, ErrNoTerminal)
	}
	if t.cmd != nil && t.cmd.Process != nil {
		// Recorded through the boundary rather than beside it, so that the
		// record carries the run and the action it belongs to exactly as a
		// gated one does. A trace line with no id is a line nothing joins to.
		//
		// Only for a process. Releasing an interpreted line cancels a context
		// and signals nothing, and writing a SIGKILL to pid 0 into the audit
		// trail would be recording an access that nobody performed — the
		// commands inside the line recorded themselves as they ran, which is
		// the whole reason that route exists.
		h.Boundary.Record(ctx, interp.Action{
			Kind: interp.ActionSignal, PID: t.cmd.Process.Pid, Signal: syscall.SIGKILL,
		})
	}
	t.Kill()
	return nil
}

// Lookup finds a terminal by id, left running. A front end that streams what a
// command is writing reads it through this rather than through Output, which
// copies the whole buffer each time.
func (h *Host) Lookup(id string) (*Terminal, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.running[id]
	return t, ok
}

// Close ends every terminal this host is holding. A front end calls it when
// its connection ends, so that a peer which disconnects mid-command leaves
// nothing running.
func (h *Host) Close() {
	h.mu.Lock()
	running := make([]*Terminal, 0, len(h.running))
	for _, t := range h.running {
		running = append(running, t)
	}
	h.running = nil
	h.mu.Unlock()
	for _, t := range running {
		t.Kill()
	}
}

func (h *Host) add(t *Terminal) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running == nil {
		h.running = map[string]*Terminal{}
	}
	h.next++
	id := "term-" + strconv.Itoa(h.next)
	h.running[id] = t
	return id
}

// Terminal is one command a host is running for a peer.
//
// Two shapes, one type. cmd is set when the peer handed us an argv and we
// exec'd it; it is nil when the peer handed us a *line* and this shell is
// interpreting it, where there is no process of our own to hold — the commands
// inside the line are processes and the interpreter holds those. cancel is how
// the second shape is ended, because there is nothing to signal.
type Terminal struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	limit  int

	mu        sync.Mutex
	output    []byte
	truncated bool
	// written is every byte the command has produced, including what trimming
	// has since dropped. It is what a progress report counts, because the
	// retained buffer's length is not monotonic and a progress value must be.
	written int
	exit    *ExitStatus

	// done is closed once the command has been reaped, which is what Wait
	// waits on. A channel rather than a condition variable because the wait
	// must also end when the caller's context does.
	done chan struct{}
}

func newTerminal(limit *int) *Terminal {
	t := &Terminal{limit: -1, done: make(chan struct{})}
	if limit != nil {
		t.limit = *limit
	}
	return t
}

// Write collects what the command produced.
//
// Both of the command's streams arrive here, because a terminal has one: a
// peer asking for output is asking what it would have seen on a screen, and
// separating them would be inventing a distinction neither protocol has.
func (t *Terminal) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.output = append(t.output, p...)
	t.written += len(p)
	t.trim()
	return len(p), nil
}

// trim keeps the retained output within the limit, dropping the oldest.
//
// The cut lands on a character boundary, which ACP requires in so many words
// and which a JSON string requires of both: a peer handed a string starting
// halfway through a rune has been handed something that is not text. Staying
// slightly under the limit is the price and is what the rule asks for.
func (t *Terminal) trim() {
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
// be started — is answered at Create, where there is a request to fail.
func (t *Terminal) reap() {
	_ = t.cmd.Wait()
	t.finish(exitStatus(t.cmd))
}

// finish records how the command ended and wakes everything waiting on it.
// Called once, by whichever goroutine owns the command — the reaper for a
// process, the interpreting goroutine for a line.
func (t *Terminal) finish(status ExitStatus) {
	t.mu.Lock()
	t.exit = &status
	t.mu.Unlock()
	close(t.done)
}

// Snapshot is the output and the exit status as they stand.
func (t *Terminal) Snapshot() Output {
	t.mu.Lock()
	defer t.mu.Unlock()
	return Output{
		Output:     string(t.output),
		Truncated:  t.truncated,
		ExitStatus: t.exit,
	}
}

// Done is closed once the command has ended.
func (t *Terminal) Done() <-chan struct{} { return t.done }

// Since is what the command has written after the first offset bytes of its
// whole output, and how many bytes that whole output now stands at.
//
// The offset counts *everything ever written*, not the retained buffer, so it
// only ever grows — which is what a progress report needs, and what the
// buffer's own length is not once trimming starts. Where trimming has already
// dropped part of what the caller has not seen, what is left of it is returned
// and the gap is silent here: Snapshot's Truncated is where that is said, and
// saying it twice in two vocabularies is how the two come to disagree.
//
// The cut lands on a character boundary, for the reason trim's does.
func (t *Terminal) Since(offset int) (string, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if offset >= t.written {
		return "", t.written
	}
	want := t.written - offset
	if want > len(t.output) {
		want = len(t.output)
	}
	cut := len(t.output) - want
	for cut < len(t.output) && !utf8.RuneStart(t.output[cut]) {
		cut++
	}
	return string(t.output[cut:]), t.written
}

// Kill ends the command if it has not ended already. A process that has been
// reaped is gone, and Kill on it is an error about nothing.
//
// An interpreted line has no process of its own, so canceling is what stands
// in for the signal — it reaches the interpreter, which is what is running the
// commands inside the line and what stops them.
func (t *Terminal) Kill() {
	select {
	case <-t.done:
		return
	default:
	}
	if t.cancel != nil {
		t.cancel()
		return
	}
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
}

// exitStatus reads how a command ended: a code, or a signal, never both.
func exitStatus(cmd *exec.Cmd) ExitStatus {
	state := cmd.ProcessState
	if state == nil {
		// Nothing to read it from, which is a failure of ours rather than of
		// the command. Reported as a code so that the peer is told something
		// definite rather than a null pair it has to guess about.
		code := -1
		return ExitStatus{ExitCode: &code}
	}
	if ws, ok := state.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		name := SignalName(ws.Signal())
		return ExitStatus{Signal: &name}
	}
	code := state.ExitCode()
	return ExitStatus{ExitCode: &code}
}

// SignalName is a protocol's string for a signal.
//
// The shell's own signal vocabulary lives in interp and stays there; this is a
// *presentation* of a number for one field of two wire formats, so it is a
// short table with a numeric fallback rather than a second copy of that
// vocabulary. A signal with no name here is reported by number rather than
// dropped, on the same rule as an event kind a consumer does not know.
func SignalName(s syscall.Signal) string {
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
