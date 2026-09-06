// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/jsonrpc"
)

// The bounded waits in this file, and why each one is not a deadline on a call.
//
// docs/design/plugins.md forbids a per-call deadline and gives the reason:
// #493 concluded that a deadline which "silently allows or refuses reports
// something other than what happened", and a plugin builtin may legitimately
// run for hours, so a default deadline would break correct plugins to catch
// broken ones. Cancellation is the way out of a call, not a clock.
//
// A bound is honest in the two places where there is nothing left to cancel.
// handshakeWait is the launch: a plugin that has not said what it is cannot
// be waited on, because nothing has been asked of it yet and the invocation
// cannot proceed without the answer. shutdownWait is the end: the plugin has
// been given EOF, and what follows is a kill either way — the bound only
// decides whether it gets to exit on its own first.
//
// cancelWait and flushWait are the same kind as shutdownWait rather than new
// kinds. Each names a moment where the thing being waited for is already lost
// if the wait expires, so the bound is not picking between two answers: it is
// deciding how long to let a plugin finish before the host stops waiting. The
// answer reported is the truth in both branches.
const (
	handshakeWait = 10 * time.Second
	shutdownWait  = 2 * time.Second
	cancelWait    = 2 * time.Second
	// flushWait is how long the observer feed gets to put the records it has
	// already numbered on the wire before the shell stops waiting for it. Same
	// kind as shutdownWait: the shell is ending, there is nothing left to
	// cancel, and the answer reported is the truth in both branches — the
	// plugin either received them or the stream stopped where it stopped.
	flushWait = 2 * time.Second
	// drainWait is how long the read loop gets to finish consuming what an
	// exiting plugin already wrote before the host takes its output away. It
	// exists because the last thing a plugin writes is usually the response to
	// the call that is about to end, and those bytes are in the pipe when the
	// process disappears.
	drainWait = time.Second
	// readChunk is what shell/read hands back when a plugin does not say how
	// much it wants.
	readChunk = 32 * 1024
)

// Options is what launching a plugin needs.
type Options struct {
	// Path is the plugin executable, and it must be absolute. See Launch.
	Path string

	// Bound is the shell's gate and audit sink, for the one access launching a
	// plugin makes: the exec. A zero Boundary allows everything and records
	// nothing, which is what a shell without a policy is.
	Bound boundary.Boundary

	// Stderr is where the plugin's own standard error is relayed, prefixed.
	// Nil discards it, which no shipped caller does — see the relay.
	Stderr io.Writer
}

// Host is one launched plugin: its declared surface, and the connection its
// builtins are calls on.
//
// Safe for concurrent use, which is not optional. A background job and each
// half of a pipeline run on their own goroutines, so two calls into one plugin
// are the ordinary case rather than a stress test — the same warning
// interp.Gate carries, where "the first gate written against this package was
// a closure appending to a slice, and it had a data race the moment a script
// said `&`."
type Host struct {
	path     string
	name     string
	commands []string

	cmd  *exec.Cmd
	conn *jsonrpc.Conn

	// in is the plugin's standard input, which is also the write half of the
	// transport. Closing it is how shutdown says "no more calls".
	in *os.File
	// out and errs are the read halves of the plugin's two output streams,
	// held here rather than by os/exec so that Close can take them away. A
	// plugin that left a grandchild holding the write end would otherwise
	// keep the read loop blocked forever, which is #690 exactly.
	out, errs *os.File

	stderr io.Writer
	// relayMu serializes the relay's writes against each other. It does not
	// and cannot serialize them against the shell's own writes to the same
	// stream; nothing can, and a plugin's diagnostics landing in the middle of
	// a script's output is what the prefix is for.
	relayMu sync.Mutex

	// obs is the observer role's feed, or nil for a plugin that did not
	// declare it. Written once during the handshake, before Launch returns and
	// therefore before anything else can hold this host, and only read
	// afterwards — see observer.go.
	obs *observer

	serveDone chan struct{}
	relayDone chan struct{}
	procDone  chan struct{}

	closeOnce sync.Once

	mu       sync.Mutex
	dead     error
	nextCall uint64
	calls    map[string]*call
}

// ErrRefused is what Launch returns when the gate refuses to run the plugin.
// Distinguishable because a caller says a different sentence about it: a
// policy that does not name the plugin is a policy doing its job, and a
// plugin that crashed at startup is a plugin that is broken.
var ErrRefused = errors.New("refused by the policy")

// Launch starts a plugin, hands it the handshake, and returns a host holding
// whatever it declared.
//
// The path must be absolute and there is no PATH search. A searched name is a
// plugin whose identity depends on a variable, and PATH is a variable a script
// sets — so a searched plugin would be arbitrary code chosen by whoever set
// the variable, which is the failure the policy format's same requirement
// exists to prevent.
//
// Launching a plugin is an exec and it passes the exec gate, through
// internal/boundary, before the process exists. Under a default-deny policy a
// plugin does not start unless the policy names it, in the same way the policy
// must already permit reading the script. That is correct rather than
// inconvenient: the person who wrote the policy is the person who named the
// plugin.
//
// The gate is consulted once, here, and not once per call. A call is not an
// exec — no process is created by it — and a gate consultation per call would
// be a decision the policy has no new information to make.
//
// One residual is recorded rather than implied. Boundary.Exec answers and the
// caller then spawns, so between the answer and the exec the path could be
// replaced. That gap is exec's and not the plugin's: it is the same shape as
// every other ActionExec in this tree, and closing it needs an exec from a
// verified descriptor, which is not portable — there is no fexecve on darwin.
// #966 closed the equivalent gap for opens, where the kernel can be asked
// where a descriptor went; nothing here can ask the same question of an exec.
func Launch(ctx context.Context, o Options) (*Host, error) {
	if o.Path == "" {
		return nil, errors.New("no plugin path")
	}
	if !filepath.IsAbs(o.Path) {
		return nil, fmt.Errorf("%s: a plugin path must be absolute — there is no PATH search for one", o.Path)
	}
	// Cleaned so that the path the gate is asked about is the path that is
	// exec'd, rather than a spelling of it. A rule about /opt/x/plugin must
	// not be walked around by writing /opt/x/./plugin.
	path := filepath.Clean(o.Path)
	if !o.Bound.Exec(ctx, path, []string{path}) {
		return nil, fmt.Errorf("%s: %w", path, ErrRefused)
	}

	h := &Host{
		path:      path,
		name:      filepath.Base(path),
		stderr:    o.Stderr,
		serveDone: make(chan struct{}),
		relayDone: make(chan struct{}),
		procDone:  make(chan struct{}),
		calls:     map[string]*call{},
	}
	if err := h.start(path); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := h.handshake(ctx); err != nil {
		_ = h.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return h, nil
}

// start spawns the plugin and gets the loops running.
//
// The three streams are os.Pipe pairs rather than exec's own StdinPipe and
// friends, and that is deliberate: exec's pipes are closed by Wait, so a
// reader still draining one when the process exits reads from a closed file
// and the ordering becomes the caller's problem. Owning both ends means Close
// can take a stream away from a blocked read, which is the only unblocking
// event the host controls when a grandchild is holding the other end.
func (h *Host) start(path string) error {
	inR, inW, err := os.Pipe()
	if err != nil {
		return err
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		_, _ = inR.Close(), inW.Close()
		return err
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		_, _ = inR.Close(), inW.Close()
		_, _ = outR.Close(), outW.Close()
		return err
	}

	cmd := exec.Command(path)
	// The environment the process inherited, which is the process's and not
	// the shell's. A plugin that wants a shell variable asks for it — that is
	// what shell/getVar is for, and the difference is not cosmetic: the
	// environment is a snapshot from the moment of the launch and the shell's
	// variables change under it.
	cmd.Env = os.Environ()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = inR, outW, errW
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		_, _ = inR.Close(), inW.Close()
		_, _ = outR.Close(), outW.Close()
		_, _ = errR.Close(), errW.Close()
		return err
	}
	// The child holds its own ends now. Ours have to go, or the read loops
	// would never see EOF: a pipe is at end of input when every write end is
	// closed, and this process holding one is this process waiting for itself.
	_, _ = inR.Close(), outW.Close()
	_ = errW.Close()

	h.cmd, h.in, h.out, h.errs = cmd, inW, outR, errR
	h.conn = jsonrpc.NewConn(outR, inW, h)

	go h.reap()
	go h.relay()
	go func() {
		defer close(h.serveDone)
		// context.Background rather than a cancellable one, because a
		// canceled context does not interrupt a blocked read and pretending
		// otherwise is how a goroutine ends up parked. What ends this loop is
		// the stream ending, and Close is what guarantees that happens.
		err := h.conn.Serve(context.Background())
		h.die(fmt.Errorf("the connection ended: %w", err))
	}()
	return nil
}

// reap waits for the plugin, exactly once, and is the only Wait in this file.
//
// A shell that accumulates zombies is worse than one that leaks a goroutine,
// because the process table is shared. So the wait is unconditional and
// starts at launch rather than at shutdown: a plugin that crashes on its own
// is reaped when it crashes, and the host knows it is dead without being
// asked.
func (h *Host) reap() {
	err := h.cmd.Wait()
	close(h.procDone)
	if err != nil {
		h.die(fmt.Errorf("it exited: %w", err))
	} else {
		h.die(errors.New("it exited"))
	}
	// What the plugin already wrote is still in the pipe, and the last thing
	// it writes is usually the response to the call that is ending. So the
	// read loop gets a bounded chance to drain before its stream is taken
	// away. Taking it away at all is what stops a grandchild holding the write
	// end from parking the loop for the life of the shell.
	select {
	case <-h.serveDone:
	case <-time.After(drainWait):
		_ = h.out.Close()
	}
}

// relay puts the plugin's standard error on the shell's, prefixed.
//
// Discarding it was considered and rejected: a plugin dying with a message and
// the message being eaten is the worst failure mode a plugin author meets. The
// prefix keeps it attributable in the middle of a script's own output, and it
// names the plugin rather than the shell because the complaint is not the
// shell's.
//
// Line-oriented rather than a straight copy, so that the prefix lands once per
// line rather than once per read: a plugin writing two lines in one write
// would otherwise get one prefix and a bare second line.
func (h *Host) relay() {
	defer close(h.relayDone)
	buf := make([]byte, 0, 4096)
	chunk := make([]byte, 4096)
	for {
		n, err := h.errs.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			for {
				i := bytes.IndexByte(buf, '\n')
				if i < 0 {
					break
				}
				h.say(string(buf[:i]))
				buf = buf[i+1:]
			}
		}
		if err != nil {
			// A last line with no newline on it is still a line somebody
			// wrote, and it is usually the one that says why the plugin died.
			if len(buf) > 0 {
				h.say(string(buf))
			}
			return
		}
	}
}

// say writes one relayed line.
func (h *Host) say(line string) {
	if h.stderr == nil {
		return
	}
	h.relayMu.Lock()
	defer h.relayMu.Unlock()
	_, _ = fmt.Fprintf(h.stderr, "sh: plugin %s: %s\n", h.Name(), line)
}

// handshake asks the plugin what it is, and refuses anything it cannot use.
//
// The host speaks first, which is the ordering docs/design/plugins.md chose:
// a plugin that dies before saying anything is then a launch failure with a
// diagnostic, rather than a host blocked on a first message that will never
// come.
func (h *Host) handshake(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, handshakeWait)
	defer cancel()
	var res initializeResult
	if err := h.conn.Call(ctx, MethodInitialize, initializeRequest{ProtocolVersion: Version}, &res); err != nil {
		return fmt.Errorf("the handshake failed: %w", err)
	}
	// A version mismatch is a refusal and not a negotiation, and the two rules
	// are different: a capability mismatch is a fact a consumer degrades on,
	// while a *version* half-understood produces a builtin that does something
	// other than what the script asked. The policy format's `version 1` rule,
	// read at the protocol layer.
	if res.ProtocolVersion != Version {
		return fmt.Errorf("it speaks protocol version %d and this shell speaks %d", res.ProtocolVersion, Version)
	}
	if res.Name != "" {
		h.mu.Lock()
		h.name = res.Name
		h.mu.Unlock()
	}
	// A plugin somebody asked for that does nothing is a configuration error,
	// and silence is the failure mode — the same reason a failed launch is
	// fatal. This is the whole of what the observer role changed about the
	// refusal: it used to be "declares no commands", because commands were the
	// only surface there was.
	if len(res.Commands) == 0 && !res.Observer {
		return errors.New("it declared no surface at all: no commands, and not the observer role")
	}
	names, err := commandNames(res.Commands)
	if err != nil {
		return err
	}
	h.commands = names
	// After the surface is settled, so a plugin that is about to be refused
	// never has a goroutine started on its behalf: Launch closes the host on a
	// handshake failure, and a feed created here would be one more thing that
	// close has to be right about for no gain.
	//
	// The event stream is opt-in and this line is the whole of the opt. A
	// plugin that did not declare the role is not fed one, is not composed into
	// the shell's Sink by the front end, and answers false to Observes.
	if res.Observer {
		h.obs = newObserver(h)
		go h.obs.feed()
	}
	return nil
}

// commandNames checks and de-duplicates a declared surface.
//
// Refused rather than filtered, because a plugin that declared a name this
// shell cannot install has misunderstood the protocol, and a shell that
// silently dropped one would resolve that word from PATH while the plugin
// believed it owned it.
//
// Declaring none is not an error here any more, and that is the observer
// role's arrival: a plugin that takes only that role has no commands and is a
// complete plugin. What is still refused is a plugin that declares no surface
// at all, and that check moved up to handshake, where both roles are visible.
func commandNames(declared []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(declared))
	for _, name := range declared {
		if name == "" {
			return nil, errors.New("it declared an empty command name")
		}
		// A command word is one word. Anything a shell would have parsed as
		// something else — a path, an assignment, two words — is a name no
		// script could reach as a builtin, so accepting it would install a
		// command nobody can run.
		if strings.ContainsAny(name, " \t\n\r/=") {
			return nil, fmt.Errorf("it declared %q, which is not a command name", name)
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out, nil
}

// Name is the plugin's declared name, or the base of its path until it has
// declared one.
func (h *Host) Name() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.name
}

// Path is the executable this host launched.
func (h *Host) Path() string { return h.path }

// Commands is every name the plugin claimed, in the order it claimed them.
func (h *Host) Commands() []string { return h.commands }

// die records the first reason this plugin stopped being usable, and is safe
// to call from anywhere: the reaper, the read loop, a canceled call, Close.
// The first reason wins, because the first one is the cause and the rest are
// consequences of it — a plugin that was killed then reports "the connection
// ended", which is true and is not what happened.
func (h *Host) die(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.dead == nil {
		h.dead = err
	}
}

// Err is why this plugin is no longer usable, or nil while it is.
func (h *Host) Err() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.dead
}

// Close shuts the plugin down and reaps it. Idempotent, and safe to call while
// calls are in flight — a call in flight is a call that is about to fail,
// which is the honest answer for a shell that is ending.
//
// The sequence is fixed and each step is here because the one before it does
// not always work:
//
//  1. Let the observer feed, if there is one, put the records it has already
//     numbered on the wire, bounded by flushWait; then close the plugin's
//     input. A cooperating plugin sees EOF and exits, which is how every
//     well-behaved one goes. The bound is what stops a plugin that has stopped
//     reading from choosing how long a shutdown takes, and closing the input
//     is what unblocks a feed already parked in a write to one.
//
//  2. Wait a bound for that to happen.
//
//  3. SIGKILL the process group. Not impatience — see killGroup. This is what
//     makes a blocked read on the plugin's output return, and killing the
//     group is what takes anything the plugin started with it.
//
//  4. Wait for the reaper, which is unbounded and can be: SIGKILL cannot be
//     caught, so the process goes.
//
//  5. Take the read streams away, which ends the two loops even if something
//     outside the group inherited a write end. Belt and braces, and **not**
//     covered: a write end held outside the process group needs a process
//     that left it, and there is no portable way to arrange one from a
//     fixture — `setsid` is not on darwin. It stays because the failure it
//     prevents is a goroutine parked for the life of the shell.
//
//     The two streams are not taken away together, and that asymmetry is
//     #1070. Standard error carries the message a plugin died with, and the
//     process is already reaped by this point, so the relay reaches end of
//     input on its own: it is given that chance before its stream goes, and
//     the close is what happens if it does not come. Doing both at once
//     discarded the diagnostic on 8 launches in 500 on Linux and 0 in 500 on
//     Darwin — the same bug on both, and only the odds differ.
func (h *Host) Close() error {
	h.closeOnce.Do(func() {
		h.die(errors.New("the shell shut it down"))
		// Before the stream goes, so the feed stops choosing new records to
		// write; and not instead of the stream going, because a feed already
		// blocked inside a write to a plugin that stopped reading cannot see
		// this. Closing the input below is what unblocks that one. Two reasons
		// to return, and between them they cover both places the goroutine can
		// be.
		if h.obs != nil {
			close(h.obs.closing)
			select {
			case <-h.obs.done:
			case <-time.After(flushWait):
			}
		}
		_ = h.in.Close()
		select {
		case <-h.procDone:
		case <-time.After(shutdownWait):
			killGroup(h.cmd)
		}
		<-h.procDone
		_ = h.out.Close()
		<-h.serveDone
		// The standard error stream is not taken away on the same breath,
		// because it is the one carrying a diagnostic. The plugin is reaped
		// by the line above, so its write end is gone and what it died saying
		// is sitting in the pipe at end of input: the relay reads it and
		// finishes on its own, in the time it takes to copy one buffer.
		// Closing first threw that message away — the read end went while the
		// bytes were still unread, and `Launch` then reported a launch
		// failure with nothing to say about why, which is #1070.
		//
		// Still bounded, and the bound is still worth having for the same
		// reason it is on the other stream: a grandchild outside the process
		// group holding the write end means no end of input is coming, and a
		// shell that waited for one would park here for good.
		select {
		case <-h.relayDone:
		case <-time.After(drainWait):
			_ = h.errs.Close()
			<-h.relayDone
		}
		if h.obs != nil {
			<-h.obs.done
		}
	})
	return nil
}
