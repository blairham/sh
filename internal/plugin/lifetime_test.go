// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package plugin_test

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/plugin"
)

// #690 is the standing example of the class of bug this file exists to
// prevent: code expected an open to fail where a FIFO actually blocks
// forever, so a goroutine and an OS thread leaked per occurrence, in a
// package whose whole purpose is being embedded in a long-lived program. A
// plugin host is a richer version of the same hazard — a peer process, pipes
// in both directions, and a partner that can stop cooperating at any moment.

// A call whose plugin died fails visibly, with the status a command that did
// not run gets. Keeping the call waiting is the failure this is written
// against; reporting success is the worse one.
func TestACallWhosePluginDiesFailsVisibly(t *testing.T) {
	t.Parallel()
	h := launch(t, "exitmid", plugin.Options{})
	sh := newShell(t, h)
	if code := sh.run(t, "vanish"); code != 126 {
		t.Errorf("status = %d, want 126 — the command visibly did not run", code)
	}
	if got := sh.errs.String(); !strings.Contains(got, "vanish") {
		t.Errorf("err = %q, want the command named", got)
	}
	if err := h.Err(); err == nil {
		t.Error("the host still thinks the plugin is alive")
	}
}

// And the shell survives it: a plugin's exit never becomes the shell's exit,
// and the next command runs.
func TestAPluginsDeathIsNotTheShellsDeath(t *testing.T) {
	t.Parallel()
	h := launch(t, "exitmid", plugin.Options{})
	sh := newShell(t, h)
	if code := sh.run(t, "vanish; printf 'still here\\n'"); code != 0 {
		t.Fatalf("status = %d", code)
	}
	if got := sh.out.String(); !strings.Contains(got, "still here") {
		t.Errorf("out = %q, want the script to have carried on", got)
	}
}

// Every later call to a dead plugin fails the same way and says so every
// time. Silence would be indistinguishable from a command that ran and
// refused.
func TestEveryCallAfterTheDeathAlsoFailsAndSaysSo(t *testing.T) {
	t.Parallel()
	h := launch(t, "exitmid", plugin.Options{})
	sh := newShell(t, h)
	if code := sh.run(t, "vanish; vanish; vanish"); code != 126 {
		t.Errorf("status = %d, want 126 each time", code)
	}
	if n := strings.Count(sh.errs.String(), "vanish"); n < 3 {
		t.Errorf("the error stream names the command %d times, want one per call:\n%s", n, sh.errs.String())
	}
	// And it says the plugin is *gone* rather than that it ended mid-call: the
	// first call is the one that discovered the death and the later ones never
	// left the process. Asserted because mutation found it: skipping the
	// already-dead check sends every later call down the wire, which happens
	// to answer 126 as well, so the status alone does not hold the property.
	if n := strings.Count(sh.errs.String(), "the plugin is gone"); n < 2 {
		t.Errorf("the error stream says the plugin is gone %d times, want it for every call after the first:\n%s",
			n, sh.errs.String())
	}
}

// An advertised capability is not a working method. A plugin that declared a
// command and then answers method-not-found to it is a plugin whose declared
// surface was not real, and the degrade is the same one a crash produces so
// there is one recovery path rather than two.
func TestMethodNotFoundOnAnInvokeIsTreatedAsTheCrashItIs(t *testing.T) {
	t.Parallel()
	h := launch(t, "noinvoke", plugin.Options{})
	sh := newShell(t, h)
	if code := sh.run(t, "ghost"); code != 126 {
		t.Errorf("status = %d, want 126", code)
	}
	if got := sh.errs.String(); !strings.Contains(got, "does not serve the command it declared") {
		t.Errorf("err = %q, want it to say the surface was not real", got)
	}
	if err := h.Err(); err == nil {
		t.Error("the host still thinks the plugin is usable")
	}
}

// A plugin that answers a cancellation keeps its process and its
// registrations, and the status it chose is the command's status.
//
// This matters more than it looks at a prompt: killing on every cancellation
// would cost a person their plugin for the rest of the session over one ^C.
// So the cooperative message is sent first and only a plugin that ignores it
// is killed.
func TestAPluginThatAnswersACancellationSurvivesIt(t *testing.T) {
	t.Parallel()
	h := launch(t, "obeycancel", plugin.Options{})
	sh := newShell(t, h)
	ctx, cancel := context.WithCancel(t.Context())
	// Canceled once the call is certainly in flight. The fixture waits for
	// exactly one message after the invoke, which is the cancellation.
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	if code := sh.runCtx(t, ctx, "patient"); code != 130 {
		t.Errorf("status = %d, want the 130 the plugin chose for a canceled call", code)
	}
	if err := h.Err(); err != nil {
		t.Errorf("Err() = %v, want a plugin that cooperated to still be alive", err)
	}
}

// A plugin that ignores a cancellation is killed, the call fails, and the
// shell gets its goroutine back. There is no third answer available: waiting
// forever is a shell that has stopped.
func TestAPluginThatIgnoresACancellationIsKilled(t *testing.T) {
	t.Parallel()
	h := launch(t, "ignorecancel", plugin.Options{})
	sh := newShell(t, h)
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	if code := sh.runCtx(t, ctx, "forever"); code != 126 {
		t.Errorf("status = %d, want 126", code)
	}
	// Bounded: the fixture sleeps for two minutes, so anything near that is a
	// host that waited for it.
	if took := time.Since(start); took > 30*time.Second {
		t.Errorf("the call took %v, want the cancellation bound to have ended it", took)
	}
	if err := h.Err(); err == nil {
		t.Error("the host still thinks a plugin it killed is alive")
	}
}

// A plugin that asks a host method about a call that has already ended is
// answered, not crashed on and not obeyed.
//
// Two things it must not do. Reaching through the empty entry in the call
// table would be a nil dereference in the process holding the script's
// variables, which is a plugin choosing when the shell dies. And *obeying* it
// would touch the Runner while the interpreter is using it — which is a data
// race, and this test is where the race detector found one: the host used to
// delete the call from the table without waiting for a handler already inside
// the Runner to come out, so a shell/setVar sent straight after an invoke
// response set a variable in a shell that had moved on.
func TestAHostMethodForACallThatEndedIsAnsweredRatherThanFatal(t *testing.T) {
	t.Parallel()
	var relayed syncBuffer
	h := launch(t, "latecall", plugin.Options{Stderr: &relayed})
	sh := newShell(t, h)
	if code := sh.run(t, "toolate"); code != 0 {
		t.Fatalf("status = %d: %s", code, sh.errs.String())
	}
	// The plugin sends the late host method a second after answering, so this
	// waits for its own report of what came back rather than for a duration.
	if !eventually(t, func() bool { return strings.Contains(relayed.String(), "answered with") }) {
		t.Fatalf("the plugin never reported an answer; it saw %q", relayed.String())
	}
	if got := relayed.String(); !strings.Contains(got, "answered with an error") {
		t.Errorf("the plugin saw %q, want an error naming the call that is not in flight", got)
	}
	if got, ok := sh.r.GetVar("LATE"); ok {
		t.Errorf("LATE = %q was set by a call that had ended", got)
	}
	// And the shell carried on, which a nil dereference in the handler would
	// have ended. Not a plugin command, so this runs whatever the plugin is
	// doing.
	if code := sh.run(t, "printf 'still here\n'"); code != 0 {
		t.Fatalf("status = %d", code)
	}
	if !strings.Contains(sh.out.String(), "still here") {
		t.Errorf("out = %q, want the shell to have carried on", sh.out.String())
	}
}

// eventually polls a condition, for the cases where what is being waited on is
// a message from another process rather than a duration.
func eventually(t *testing.T, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// The assertion that makes the lifetime rules gradeable rather than asserted.
//
// Many plugin lifecycles in one process, against fixtures that misbehave in
// each specific way, and the goroutine count has to come back to where it
// started. Every goroutine the host starts has a reason to return that the
// host controls — the read loop and the relay return when their streams end,
// and Close is what guarantees the streams end — so a fixture that hangs at
// the handshake, one that exits mid-call, one that writes garbage and one that
// ignores a cancellation must all leave nothing behind.
func TestManyLifecyclesLeaveNoGoroutinesBehind(t *testing.T) {
	// Not parallel: it counts goroutines, and another test's would be counted
	// too.
	base := settled(t, 0)

	for range 3 {
		// watcher is here for the observer role's own goroutine: it declares
		// no commands, so its feed spends the whole lifecycle idle, waiting on
		// a channel nothing will ever put anything on again. Close is the only
		// thing that ends it, which is exactly the shape this test exists to
		// catch.
		for _, name := range []string{"greet", "garbage", "noisy", "exitmid", "noinvoke", "watcher"} {
			h, err := plugin.Launch(t.Context(), plugin.Options{Path: fixture(t, name), Stderr: &syncBuffer{}})
			if err != nil {
				t.Fatalf("Launch(%s): %v", name, err)
			}
			sh := newShell(t, h)
			for _, cmd := range h.Commands() {
				_ = sh.run(t, cmd)
			}
			if err := h.Close(); err != nil {
				t.Fatalf("Close(%s): %v", name, err)
			}
		}
		// The ones that never come up: a launch failure must clean up after
		// itself, and it is the path where nothing is holding a handle to the
		// host afterwards.
		for _, name := range []string{"oldversion", "nocommands", "notaname", "crash"} {
			if _, err := plugin.Launch(t.Context(), plugin.Options{Path: fixture(t, name), Stderr: &syncBuffer{}}); err == nil {
				t.Fatalf("Launch(%s) succeeded", name)
			}
		}
		// And one that never says anything, which is the handshake bound.
		ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
		if _, err := plugin.Launch(ctx, plugin.Options{Path: fixture(t, "hang"), Stderr: &syncBuffer{}}); err == nil {
			t.Fatal("Launch(hang) succeeded")
		}
		cancel()
	}

	if got := settled(t, base); got > base {
		t.Errorf("goroutines went from %d to %d and stayed there: something is parked", base, got)
	}
}

// settled is the goroutine count once it stops changing, or the last one seen.
//
// Polled rather than measured once, because a goroutine that is on its way out
// is not a leak: a read loop returning and a process being reaped are both
// asynchronous, and asserting on the instant after Close would fail on timing
// rather than on a leak.
func settled(t *testing.T, want int) int {
	t.Helper()
	got := runtime.NumGoroutine()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if got <= want || want == 0 {
			// For a baseline, one quiet reading is enough; for a comparison,
			// reaching the baseline is the answer.
			if want == 0 {
				time.Sleep(50 * time.Millisecond)
				if next := runtime.NumGoroutine(); next == got {
					return got
				}
			} else {
				return got
			}
		}
		time.Sleep(50 * time.Millisecond)
		got = runtime.NumGoroutine()
	}
	return got
}

// The window the check above cannot be tested through, tested directly.
//
// A message about a call sent in the instant *after* its response — no pause,
// so it lands while the interpreter is picking the run back up. Nothing may
// reach the Runner from there, and nothing may write to the streams the call
// was pointing at. Both would be data races on memory the interpreter owns,
// and both were: this is the fixture the race detector caught the host with.
//
// The assertion is memory safety rather than which of the two won, because
// which of them won is genuinely undecidable — the response and the message
// behind it are handed to two goroutines. What is decidable, and what this
// holds, is that the loser does not corrupt anything and the shell carries on.
func TestAMessageArrivingInTheInstantAfterACallIsNotARace(t *testing.T) {
	t.Parallel()
	h := launch(t, "racecall", plugin.Options{Stderr: &syncBuffer{}})
	sh := newShell(t, h)
	// Many times, because the window is small: one attempt would pass on a
	// host that has the bug simply by not hitting it.
	for i := range 40 {
		if code := sh.run(t, "racy"); code != 0 {
			t.Fatalf("attempt %d: status = %d: %s", i, code, sh.errs.String())
		}
		if code := sh.run(t, "printf 'ok\n'"); code != 0 {
			t.Fatalf("attempt %d: the shell did not carry on: status = %d", i, code)
		}
	}
	if got := strings.Count(sh.out.String(), "ok\n"); got != 40 {
		t.Errorf("the shell got through %d of 40 rounds", got)
	}
}
