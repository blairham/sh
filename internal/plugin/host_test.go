// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package plugin_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/plugin"
	"github.com/blairham/sh/interp"
)

func TestHandshakeCarriesTheDeclaredSurface(t *testing.T) {
	t.Parallel()
	h := launch(t, "greet", plugin.Options{})
	if h.Name() != "greet" {
		t.Errorf("Name() = %q, want the name the plugin declared", h.Name())
	}
	if got := h.Commands(); len(got) == 0 || got[0] != "greet" {
		t.Errorf("Commands() = %v, want the names it claimed in the order it claimed them", got)
	}
	if err := h.Err(); err != nil {
		t.Errorf("Err() = %v, want a live plugin", err)
	}
	if h.Path() == "" {
		t.Error("Path() is empty, so a diagnostic could not say which plugin")
	}
}

// A relative path is refused before anything is spawned. A searched name is a
// plugin whose identity depends on a variable, and PATH is a variable a script
// sets — so a searched plugin is arbitrary code chosen by whoever set it.
func TestARelativePathIsRefused(t *testing.T) {
	t.Parallel()
	_, err := plugin.Launch(t.Context(), plugin.Options{Path: "testdata/greet"})
	if err == nil {
		t.Fatal("Launch accepted a relative path")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("err = %v, want it to say the path must be absolute", err)
	}
}

// The handshake is one of the two places a bound is honest: nothing has been
// asked of the plugin yet, so there is nothing to cancel, and the invocation
// cannot proceed without an answer.
//
// The context the caller gives Launch is the outer limit, which is what this
// uses rather than waiting ten seconds for the host's own bound.
func TestAPluginThatNeverSpeaksIsALaunchFailure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	start := time.Now()
	_, err := plugin.Launch(ctx, plugin.Options{Path: fixture(t, "hang")})
	if err == nil {
		t.Fatal("Launch returned a host for a plugin that never answered")
	}
	if !strings.Contains(err.Error(), "handshake") {
		t.Errorf("err = %v, want it to name the handshake", err)
	}
	// And it gave up rather than being killed by the test framework, which is
	// the difference between a bound and a hang.
	if took := time.Since(start); took > 30*time.Second {
		t.Errorf("Launch took %v, want it bounded", took)
	}
}

func TestAPluginThatDiesAtStartupIsALaunchFailure(t *testing.T) {
	t.Parallel()
	var relayed syncBuffer
	_, err := plugin.Launch(t.Context(), plugin.Options{Path: fixture(t, "crash"), Stderr: &relayed})
	if err == nil {
		t.Fatal("Launch returned a host for a plugin that exited")
	}
	// What it wrote on the way out is the whole reason it is worth relaying:
	// the message a plugin dies with is the one that says why.
	if !strings.Contains(relayed.String(), "I cannot start") {
		t.Errorf("relayed = %q, want the plugin's own complaint", relayed.String())
	}
}

// A version mismatch is a refusal and not a negotiation. A protocol
// half-understood produces a builtin that does something other than what the
// script asked, which is a different risk from a log with an unknown record in
// it — the policy format's `version 1` rule read at the protocol layer.
func TestAnUnknownProtocolVersionIsRefused(t *testing.T) {
	t.Parallel()
	_, err := plugin.Launch(t.Context(), plugin.Options{Path: fixture(t, "oldversion")})
	if err == nil {
		t.Fatal("Launch accepted a version this host does not speak")
	}
	if !strings.Contains(err.Error(), "99") {
		t.Errorf("err = %v, want the version it offered named", err)
	}
}

func TestADeclaredSurfaceThisShellCannotInstallIsRefused(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, want string }{
		{"nocommands", "no commands"},
		{"notaname", "not a command name"},
	} {
		_, err := plugin.Launch(t.Context(), plugin.Options{Path: fixture(t, c.name)})
		if err == nil {
			t.Fatalf("%s: Launch accepted it", c.name)
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to say %q", c.name, err, c.want)
		}
	}
}

// One line that is not a message is one bad message. The transport is
// line-delimited exactly so that a stray line does not end the connection, and
// without that this plugin — which works — would be a launch failure.
func TestAStrayLineDoesNotEndTheHandshake(t *testing.T) {
	t.Parallel()
	h := launch(t, "garbage", plugin.Options{})
	if got := h.Commands(); len(got) != 1 || got[0] != "still-here" {
		t.Errorf("Commands() = %v, want the plugin to have handshaken anyway", got)
	}
}

// Launching a plugin is an exec and it passes the exec gate. Under a
// default-deny policy a plugin does not start unless the policy names it, in
// the same way the policy must already permit reading the script.
func TestLaunchingAPluginPassesTheExecGate(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var seen []interp.Action
	g := interp.GateFunc(func(_ context.Context, a interp.Action) interp.Decision {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, a)
		return interp.Deny
	})
	sink := &recordingSink{}
	path := fixture(t, "greet")
	_, err := plugin.Launch(t.Context(), plugin.Options{
		Path:  path,
		Bound: boundary.Boundary{Gate: g, Events: sink, Session: "run-1"},
	})
	if !errors.Is(err, plugin.ErrRefused) {
		t.Fatalf("err = %v, want ErrRefused — a refused plugin must not start", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 {
		t.Fatalf("the gate saw %d action(s), want exactly one: %v", len(seen), seen)
	}
	a := seen[0]
	if a.Kind != interp.ActionExec {
		t.Errorf("kind = %v, want an exec — launching a plugin is one", a.Kind)
	}
	if a.Path != path {
		t.Errorf("path = %q, want the plugin's own path %q", a.Path, path)
	}
	if len(a.Args) == 0 || a.Args[0] != path {
		t.Errorf("args = %v, want the whole vector with argv[0] in it", a.Args)
	}
	if a.ID == "" {
		t.Error("the action carries no id, so nothing can be joined to it")
	}
	// And the refusal is in the audit trail under the run's own session: a
	// policy that refuses silently is a policy nobody can debug, and a record
	// naming no run is a record nothing can be joined to.
	if sink.count() == 0 {
		t.Error("the refusal was not recorded")
	}
	if s := sink.session(); s != "run-1" {
		t.Errorf("the record says session %q, want the run's own", s)
	}
}

// The gate is asked once, at launch, and never per call. A call creates no
// process, so a consultation per call would be a decision the policy has no
// new information to make — and at the rate a shell runs commands it would be
// the hot-path objection all over again.
func TestTheGateIsAskedOnceAndNotPerCall(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	execs := 0
	g := interp.GateFunc(func(_ context.Context, a interp.Action) interp.Decision {
		if a.Kind == interp.ActionExec {
			mu.Lock()
			execs++
			mu.Unlock()
		}
		return interp.Allow
	})
	h := launch(t, "greet", plugin.Options{Bound: boundary.Boundary{Gate: g}})
	sh := newShell(t, h)
	for range 3 {
		if code := sh.run(t, "greet world"); code != 0 {
			t.Fatalf("status %d", code)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if execs != 1 {
		t.Errorf("the gate saw %d execs, want exactly the launch", execs)
	}
}

// A plugin's standard error reaches the shell's, one prefix per line, and the
// last line survives with no newline after it — which is the line that usually
// says why it died.
func TestStandardErrorIsRelayedLineByLine(t *testing.T) {
	t.Parallel()
	var relayed syncBuffer
	h := launch(t, "noisy", plugin.Options{Stderr: &relayed})
	sh := newShell(t, h)
	if code := sh.run(t, "quiet"); code != 0 {
		t.Fatalf("status %d", code)
	}
	// Close first: the relay is a goroutine, and the unterminated line is
	// written when its stream ends.
	_ = h.Close()
	got := relayed.String()
	for _, want := range []string{
		"sh: plugin noisy: first thing\n",
		"sh: plugin noisy: second thing\n",
		"sh: plugin noisy: a word with no newline after it\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("relayed =\n%s\nwant a line %q", got, want)
		}
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	t.Parallel()
	h := launch(t, "greet", plugin.Options{})
	for range 3 {
		if err := h.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
	if err := h.Err(); err == nil {
		t.Error("Err() is nil after Close, so a call would be sent to a plugin that is gone")
	}
}

// A path with nothing at it is a launch failure that names the path, rather
// than a panic or a silent shell.
func TestAMissingPluginIsALaunchFailure(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nothing-here")
	_, err := plugin.Launch(t.Context(), plugin.Options{Path: path})
	if err == nil {
		t.Fatal("Launch succeeded for a path with nothing at it")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("err = %v, want it to name the path", err)
	}
}

// The path the gate is asked about is the path that is exec'd. A rule about
// /opt/x/plugin must not be walked around by spelling it /opt/x/./plugin.
func TestThePathIsCleanedBeforeTheGateSeesIt(t *testing.T) {
	t.Parallel()
	clean := fixture(t, "greet")
	dirty := filepath.Dir(clean) + "/./" + filepath.Base(clean)
	var seen string
	g := interp.GateFunc(func(_ context.Context, a interp.Action) interp.Decision {
		seen = a.Path
		return interp.Deny
	})
	_, _ = plugin.Launch(t.Context(), plugin.Options{Path: dirty, Bound: boundary.Boundary{Gate: g}})
	if seen != clean {
		t.Errorf("the gate was asked about %q, want the cleaned path %q", seen, clean)
	}
}

// A relay with nowhere to write is silent rather than fatal. No shipped caller
// passes nil, and a package that fell over on the zero value would be one an
// embedder had to be warned about.
func TestANilRelayIsSilentRatherThanFatal(t *testing.T) {
	t.Parallel()
	h, err := plugin.Launch(t.Context(), plugin.Options{Path: fixture(t, "noisy")})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	defer func() { _ = h.Close() }()
	sh := newShell(t, h)
	if code := sh.run(t, "quiet"); code != 0 {
		t.Errorf("status %d", code)
	}
}

// recordingSink counts what reached the audit stream, and the run it named.
type recordingSink struct {
	mu     sync.Mutex
	events []interp.Event
	run    string
}

func (s *recordingSink) Emit(_ context.Context, e interp.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	if e.Session != "" {
		s.run = e.Session
	}
}

func (s *recordingSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func (s *recordingSink) session() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.run
}
