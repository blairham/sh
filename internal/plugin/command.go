// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blairham/sh/internal/jsonrpc"
	"github.com/blairham/sh/interp"
)

// The command role: the plugin declares names, the host registers a stub for
// each, and the stub turns a call into a request.
//
// What a script can see of this is deliberately nothing. `type foo` says
// `builtin`, `enable -n foo` switches it off, and a shell function shadows it,
// because a plugin's command is registered through interp.Runner.Register and
// sits exactly where a Go builtin sits in name resolution — after functions,
// before PATH. That is what keeps a prelude winning outright over a plugin
// rather than by convention, and it is why interp gains no field, no interface
// and no knowledge that plugins exist.

// call is one invocation in flight: the runner it belongs to, and the three
// streams it is pointing at.
//
// The streams are captured once here, on the builtin's own goroutine, rather
// than asked of the Runner when a message about them arrives. Two reasons and
// both are load-bearing. A Runner is not safe for concurrent use, and the
// host's read loop is a different goroutine from the one running the command.
// And a builtin's streams do not change while it runs — redirections are
// applied around the call — so a snapshot is not a stale copy of anything.
type call struct {
	id  string
	r   *interp.Runner
	out io.Writer
	err io.Writer
	in  io.Reader

	// done says the builtin has returned, so nothing may touch the Runner on
	// this call's behalf any more. Checked under state, where it is
	// authoritative, and read without it on the paths that must not block.
	done atomic.Bool

	// state serializes the host methods that touch the Runner, and is what
	// end waits on. Two things, and the second one is the reason it exists.
	//
	// A well-behaved plugin asks for one thing at a time; two concurrent
	// shell/setVar calls for one command would race the Runner's maps, and a
	// data race reachable from another process is a data race somebody else
	// chooses.
	//
	// And a host method's handler runs on a goroutine of its own, so it can
	// land *after* the invoke response was delivered and the builtin returned
	// — at which point the interpreter has carried on and is using the Runner
	// itself. That is not hypothetical: a fixture that sends shell/setVar
	// straight after its own invoke response set a variable in a shell that
	// had moved on, and the race detector found it. See end.
	state sync.Mutex
	// input serializes reads, separately from state. A read can block for as
	// long as the input does, and a plugin waiting on stdin must not thereby
	// be unable to ask what the working directory is — nor hold up end, which
	// is the same fact from the other side.
	input sync.Mutex
}

// live reports whether this call may still touch the Runner, and holds state
// while the caller does. The caller unlocks.
//
// The check and the lock are one operation on purpose: a handler that tested
// liveness and then took the lock would be testing something that could stop
// being true in between, which is the whole bug this exists to prevent.
//
// That ordering is a requirement the tests do **not** hold. Reversing the two
// lines is a mutant that survives — the window is a few instructions wide and
// no fixture reaches it reliably — so this comment is what is guarding it.
func (c *call) live() bool {
	c.state.Lock()
	if c.done.Load() {
		c.state.Unlock()
		return false
	}
	return true
}

// Register installs a stub for each of the plugin's declared commands.
//
// Composed into driver.Shell.Register by the front end, after the dialect's
// own registrations, so a plugin may replace a builtin a dialect installed.
// docs/design/plugins.md flags that as the maintainer's to reverse; it follows
// from the conservation rule, which says the plugin surface is Register's
// surface, and Register permits replacement.
//
// Called once per Runner, at startup, before the first command word is
// resolved. That timing is not an optimization either: a name that appeared
// partway through a run would make `type foo` answer differently depending on
// what had already run.
func (h *Host) Register(r *interp.Runner) {
	for _, name := range h.commands {
		r.Register(name, h.stub(name))
	}
}

// stub is the interp.Builtin that stands in for one of the plugin's commands.
func (h *Host) stub(name string) interp.Builtin {
	return func(r *interp.Runner, ctx context.Context, args []string) int {
		return h.invoke(ctx, r, name, args)
	}
}

// The statuses a plugin builtin can fail with, both of them borrowed rather
// than invented.
//
// statusGone is 126, the status a refused exec gets, for the reason
// docs/design/sandboxing.md gives it there: a command that visibly did not run
// cannot honestly report otherwise. A plugin that died mid-call, a plugin that
// answered method-not-found, a plugin that never came back — all of them are
// commands that did not run, and none of them is a command that ran and
// failed.
const statusGone = 126

// invoke runs one command in the plugin and returns its status.
func (h *Host) invoke(ctx context.Context, r *interp.Runner, name string, args []string) int {
	if err := h.Err(); err != nil {
		// The plugin is already gone. Said every time rather than once,
		// because a script that runs the word in a loop is a script that gets
		// 126 in a loop, and a silent 126 is indistinguishable from a command
		// that ran and refused.
		//
		// The name is *not* withdrawn here, and that is a departure from
		// docs/design/plugins.md, which said the name should afterwards
		// resolve as if the plugin had never existed. It cannot be done
		// safely: Runner.clone shares the registration map with every
		// subshell, so calling Unregister from a builtin's goroutine writes a
		// map that a concurrent background job is reading. That is the exact
		// race interp.Gate's doc comment warns about, and it is worse than the
		// shadowing it would fix. Recorded in the design document as an open
		// item rather than left here.
		r.Diagnosef("%s: the plugin is gone: %v", name, err)
		return statusGone
	}

	c := h.begin(r)
	defer h.end(c)

	// The call's own context, deliberately not the command's. Conn.Call gives
	// up the moment the context it was given is done, and giving it the
	// command's context would throw away the answer of a plugin that *did*
	// respond to the cancellation — which is the whole reason to send one.
	// This is canceled when the host stops waiting, and watch is what decides
	// when that is.
	callCtx, giveUp := context.WithCancel(context.Background())
	defer giveUp()
	done := make(chan struct{})
	defer close(done)
	go h.watch(ctx, c.id, done, giveUp)

	var res invokeResult
	err := h.conn.Call(callCtx, MethodInvoke, invokeRequest{Call: c.id, Name: name, Args: args}, &res)
	if err != nil {
		return h.failed(r, name, err)
	}
	// A shell status is a byte. A plugin answering 256 has almost certainly
	// handed back a wait status, and treating that as 0 would report success
	// for a command that failed — so it is a protocol error and says so.
	if res.Status < 0 || res.Status > 255 {
		r.Diagnosef("%s: the plugin answered with status %d, which is not a status", name, res.Status)
		return statusGone
	}
	return res.Status
}

// failed turns a wire failure into a diagnostic and a status.
//
// A plugin that answers method-not-found to an invoke is not a plugin that
// declined this call; it is a plugin whose declared surface was not real.
// docs/design/acp.md's rule is the one being applied — degrade on the answer,
// not on the advertisement — and the degrade is the same one a crash produces,
// so there is one recovery path rather than two.
func (h *Host) failed(r *interp.Runner, name string, err error) int {
	var rpcErr *jsonrpc.Error
	switch {
	case errors.As(err, &rpcErr) && rpcErr.Code == jsonrpc.CodeMethodNotFound:
		h.die(fmt.Errorf("it declared %s and then did not serve %s", name, MethodInvoke))
		r.Diagnosef("%s: the plugin does not serve the command it declared", name)
	case errors.Is(err, jsonrpc.ErrClosed):
		h.die(errors.New("the connection ended mid-call"))
		r.Diagnosef("%s: the plugin ended mid-call", name)
	default:
		h.die(fmt.Errorf("a call failed: %w", err))
		r.Diagnosef("%s: %v", name, err)
	}
	return statusGone
}

// watch is the cancellation path, and it is the only way out of a call that is
// not the plugin answering.
//
// There is no per-call deadline, on purpose: a plugin builtin may legitimately
// run for hours and a default deadline would break correct plugins to catch
// broken ones. What ends a call early is the shell giving up — a ^C, or the
// context the Runner was given being canceled — and this is what that turns
// into.
//
// Cooperatively first. The cancel notification gives a plugin the chance to
// stop cleanly and answer, and a plugin that does keeps its registrations and
// its process. Only a plugin that ignores it is killed, which matters more
// than it looks at a prompt: one ^C should not cost a person the plugin for
// the rest of the session.
func (h *Host) watch(ctx context.Context, id string, done <-chan struct{}, giveUp func()) {
	select {
	case <-done:
		return
	case <-ctx.Done():
	}
	// On its own goroutine, because writing to the plugin's standard input
	// blocks when the plugin has stopped reading it, and a canceller that can
	// block is a canceller that never reaches the kill. That goroutine's
	// reason to return is the host's: the kill below closes the pipe under it.
	go func() { _ = h.conn.Notify(MethodCancel, cancelParams{Call: id}) }()
	select {
	case <-done:
		// It answered. Nothing was lost and nothing is killed.
	case <-time.After(cancelWait):
		h.die(errors.New("it did not answer a cancellation"))
		killGroup(h.cmd)
		giveUp()
	}
}

// begin and end are the call table. It exists because every message about a
// call has to find the Runner the call belongs to, and a shell has more than
// one: a subshell has its own variables and its own directory, and the two
// halves of a pipeline are two Runners running at once.
func (h *Host) begin(r *interp.Runner) *call {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextCall++
	c := &call{
		id:  strconv.FormatUint(h.nextCall, 10),
		r:   r,
		out: r.Out(),
		err: r.Err(),
		in:  r.In(),
	}
	h.calls[c.id] = c
	return c
}

// end closes a call: nothing may touch the Runner on its behalf afterwards.
//
// The state lock is taken and given straight back, and it is not protecting
// anything here — it is *waiting* for a host method already inside the Runner
// to come out. Without it the window is real and small and the race detector
// finds it, because a handler dispatched just before the invoke response was
// delivered can still be running when the interpreter carries on.
//
// What it deliberately does not wait for is a read: that holds input rather
// than state, precisely so that a plugin blocked on the command's standard
// input cannot hold up the command's own return. The residual is that a read
// which passed its liveness check just before this can complete just after,
// consuming bytes from a stream the call no longer owns. That is a protocol
// violation by the plugin with a confined consequence — it touches no shared
// memory, so it is a wrong answer rather than a race — and the alternative is
// a shell a plugin can hang by asking to read.
func (h *Host) end(c *call) {
	c.done.Store(true)
	c.state.Lock()
	c.state.Unlock() //nolint:staticcheck // waiting for a handler to leave, not guarding
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.calls, c.id)
}

// find is the call a message names, or nothing.
//
// A message naming a call that has ended is dropped rather than answered with
// an error, in the request case it is answered with one — see Handle. Either
// way it does not end the connection: the call may have been abandoned a
// microsecond ago and the plugin cannot have known.
func (h *Host) find(id string) *call {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls[id]
}

// Handle answers the host methods, which are the small part of the Runner a
// plugin genuinely cannot do without.
//
// Everything not below is deliberately absent, and each absence has a reason
// rather than being a gap somebody has not got to:
//
//   - Register and Unregister at call time. A plugin rewriting the command
//     table while a script runs is a shell whose `type` answer depends on what
//     has already run. Names are declared at the handshake and fixed.
//   - Expand. Expansion includes command substitution, so it is `eval` by
//     proxy: it re-enters the interpreter with input a foreign process chose,
//     while a builtin call from that process is in flight. Every re-entrancy
//     hazard in the interpreter, reachable from outside it, to save a plugin
//     author a getVar.
//   - LookPath, WriterForFd, NamedOption, FunctionText. Each exists for a
//     specific in-process builtin, none is needed to write a command, and a
//     surface is easier to widen later than to narrow.
//   - Anything that opens a file or runs a command. A plugin is already free
//     to do both with its own descriptors; a route through us would add inward
//     attack surface to the process holding the script's variables without
//     removing any freedom it had. This is where the design deliberately
//     differs from docs/design/acp.md, which offers an agent fs/read_text_file
//     precisely so the access comes through our gate — an ACP client is
//     constraining an agent, and there is nothing here to constrain.
//   - Answering a gate consultation. The rate is wrong by orders of magnitude
//     — a glob stats every entry it descends past — and there is no third
//     answer available: fail closed and a hung plugin bricks the shell, fail
//     open and `kill -9` on the plugin is a privilege escalation. An embedder
//     who wants a remote decision writes an interp.Gate in Go that calls out
//     however it likes, and owns its timeout.
//
// A method this host does not have is answered -32601, which is a fact and not
// a failure: the plugin records it and carries on. That is what makes the
// surface widenable without a version bump.
func (h *Host) Handle(_ context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case MethodGetVar:
		var p getVarParams
		c, err := h.callFor(method, params, &p, func() string { return p.Call })
		if err != nil {
			return nil, err
		}
		if !c.live() {
			return nil, ended(method, p.Call)
		}
		defer c.state.Unlock()
		v, ok := c.r.GetVar(p.Name)
		return getVarResult{Value: v, Set: ok}, nil

	case MethodSetVar:
		var p setVarParams
		c, err := h.callFor(method, params, &p, func() string { return p.Call })
		if err != nil {
			return nil, err
		}
		// Not gated, and that is a decision rather than an oversight. Setting
		// a variable does not leave the interpreter's own memory, and the
		// action vocabulary is deliberately the six kinds that do — adding a
		// seventh would be an interp change first, which is the rule
		// docs/design/sandboxing.md states about network rules applied here.
		if !c.live() {
			return nil, ended(method, p.Call)
		}
		defer c.state.Unlock()
		c.r.SetVar(p.Name, p.Value)
		return struct{}{}, nil

	case MethodDir:
		var p dirParams
		c, err := h.callFor(method, params, &p, func() string { return p.Call })
		if err != nil {
			return nil, err
		}
		if !c.live() {
			return nil, ended(method, p.Call)
		}
		defer c.state.Unlock()
		return dirResult{Dir: c.r.Dir}, nil

	case MethodRead:
		var p readParams
		c, err := h.callFor(method, params, &p, func() string { return p.Call })
		if err != nil {
			return nil, err
		}
		// Best-effort, deliberately: the read itself is under input rather
		// than state, so this narrows a window it cannot close. Removing the
		// check is a mutant that survives, and correctly — see end, which
		// records why the read path is the one place a call's end is not
		// airtight, and why a shell a plugin can hang by asking to read would
		// be the worse trade.
		if c.done.Load() {
			return nil, ended(method, p.Call)
		}
		return c.read(p.Max)

	case MethodDiagnose:
		var p diagnoseParams
		c, err := h.callFor(method, params, &p, func() string { return p.Call })
		if err != nil {
			return nil, err
		}
		if !c.live() {
			return nil, ended(method, p.Call)
		}
		defer c.state.Unlock()
		// Diagnosef and not a write to the error stream, because that is the
		// difference the seam was added for: a complaint written straight to
		// the stream carries no location and no name, which is how a
		// registered builtin ends up saying less than the one beside it. The
		// text arrives assembled — see diagnoseParams — so it is passed as an
		// argument and never as a format.
		c.r.Diagnosef("%s", p.Text)
		return struct{}{}, nil
	}
	return nil, jsonrpc.Errorf(jsonrpc.CodeMethodNotFound, "this shell does not serve %s", method)
}

// callFor decodes a host method's parameters and finds the call they name.
//
// A call that has ended is an invalid-params error rather than a dropped
// message, because the plugin is waiting for an answer and silence would park
// it. It is not fatal to anything: the plugin learns the call is over, which
// is true.
func (h *Host) callFor(method string, params json.RawMessage, into any, id func() string) (*call, error) {
	if err := json.Unmarshal(params, into); err != nil {
		return nil, jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "%s: %v", method, err)
	}
	c := h.find(id())
	if c == nil {
		return nil, ended(method, id())
	}
	return c, nil
}

// ended is what a host method about a call that is over is answered with.
//
// An error rather than a dropped message, because the plugin is waiting for an
// answer and silence would park it. It is not fatal to anything: the plugin
// learns the call is over, which is true.
func ended(method, id string) error {
	return jsonrpc.Errorf(jsonrpc.CodeInvalidParams, "%s: call %q is not in flight", method, id)
}

// read hands back at most max bytes of the command's input.
//
// A pull rather than a push — see readParams for why, which is the lifetime
// rule rather than a preference. A short read is not the end of the stream, so
// EOF is its own field: a plugin that inferred the end from an empty chunk
// would truncate a pipeline whose writer had simply not got there yet.
func (c *call) read(maxBytes int) (any, error) {
	if maxBytes <= 0 || maxBytes > readChunk {
		maxBytes = readChunk
	}
	c.input.Lock()
	defer c.input.Unlock()
	buf := make([]byte, maxBytes)
	n, err := c.in.Read(buf)
	res := readResult{Data: encode(buf[:n])}
	if errors.Is(err, io.EOF) {
		res.EOF = true
	} else if err != nil && n == 0 {
		return nil, jsonrpc.Errorf(jsonrpc.CodeInternalError, "%s: %v", MethodRead, err)
	}
	return res, nil
}

// Notify takes what a plugin sends with nobody to answer: output.
//
// Handled on the read loop, in order, which is the transport's contract and is
// what makes `echo one; echo two` from a plugin arrive in that order. So this
// must not block on anything but the stream it is writing to — and blocking on
// that is correct, because it is exactly what an in-process builtin writing to
// a full pipe does.
//
// A notification for a call that has ended is dropped in silence. There is
// nobody to tell, which is what a notification means.
func (h *Host) Notify(_ context.Context, method string, params json.RawMessage) {
	if method != MethodOutput {
		// Unknown notifications are ignored rather than reported, which is
		// rule three of the event schema's stability rules adopted wholesale:
		// a peer that sends something newer than we understand has not done
		// anything wrong.
		return
	}
	var p outputParams
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}
	c := h.find(p.Call)
	if c == nil {
		// Dropped in silence: there is nobody to tell, which is what a
		// notification means.
		return
	}
	// Under the call's own lock, and the reason is a data race the detector
	// found rather than tidiness. A chunk that arrives in the instant after
	// the invoke response — dispatched by the read loop before the builtin's
	// goroutine has finished returning — would otherwise be written to a
	// stream the interpreter is already using again. An atomic check is not
	// enough: it can pass and then stop being true. So liveness and the write
	// are one operation, and end waits for it.
	//
	// This can block, on the stream, exactly as an in-process builtin writing
	// to a full pipe blocks. It holds the connection while it does, which is
	// the transport's stated cost of ordered notifications — and ordering is
	// the point: `echo one; echo two` from a plugin has to arrive that way.
	if !c.live() {
		return
	}
	defer c.state.Unlock()
	data, err := decode(p.Data)
	if err != nil {
		// Not silent, because a plugin whose output is quietly discarded looks
		// exactly like a command that printed nothing. It goes through the
		// relay so it reads like everything else the plugin said.
		h.say(fmt.Sprintf("%s: %v", MethodOutput, err))
		return
	}
	w := c.out
	if p.Stream == StreamErr {
		w = c.err
	}
	_, _ = w.Write(data)
}
