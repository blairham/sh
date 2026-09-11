// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package plugin_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blairham/sh/internal/plugin"
	"github.com/blairham/sh/interp"
)

// watching is a shell with the plugin's commands registered and its observer
// role, if it has one, installed as the Sink.
//
// Both halves, because a plugin may take both roles and the interesting cases
// are the ones where it has: the whole point of an interp.Sink is that it is
// the seam a front end already has, so a test that wired the observer anywhere
// but Runner.Events would be grading something the shell does not do.
func watching(t *testing.T, h *plugin.Host) *shell {
	t.Helper()
	sh := newShell(t, h)
	sh.r.Events = h.Sink()
	return sh
}

// The role, end to end: the shell runs a command and a plugin in another
// language is told what it did.
//
// Through Runner.Events and through a real command rather than a synthesized
// event, because the claim is that the plugin sees the shell's own stream. The
// plugin writes what it saw to its standard error, which the host relays — a
// route that already existed and that a real observer plugin would use.
func TestAnObserverPluginIsToldWhatTheShellDid(t *testing.T) {
	t.Parallel()
	relayed := &syncBuffer{}
	h := launch(t, "watcher", plugin.Options{Stderr: relayed})
	if h.Sink() == nil {
		t.Fatal("Sink() is nil for a plugin that declared the observer role")
	}
	if got := h.Commands(); len(got) != 0 {
		t.Errorf("Commands() = %v, want none: this plugin declared only the observer role", got)
	}
	sh := watching(t, h)
	if code := sh.run(t, "/bin/echo hello"); code != 0 {
		t.Fatalf("status = %d: %s", code, sh.errs.String())
	}
	// A command the shell ran produces a start and an end, and both name the
	// exec. Waited for rather than asserted straight away: the plugin is
	// another process and the feed is a goroutine, so this is a message in
	// flight and not a return value.
	for _, want := range []string{"saw command-start exec", "saw command-end exec"} {
		if !eventually(t, func() bool { return strings.Contains(relayed.String(), want) }) {
			t.Errorf("relayed = %q, want %q", relayed.String(), want)
		}
	}
}

// A plugin that did not ask for the event stream does not get one.
//
// The event stream is every command the shell runs, every path it touches and
// every refusal a policy made. That is a disclosure, and the thing that
// authorizes it is the plugin having declared the role — which is checked
// here from the outside, at the only place a front end can see it. A host that
// fed every plugin would be handing a command plugin the whole of a person's
// session because they wanted one word implemented in Python.
func TestAPluginThatDidNotAskForTheEventStreamHasNoSink(t *testing.T) {
	t.Parallel()
	h := launch(t, "greet", plugin.Options{})
	if s := h.Sink(); s != nil {
		t.Errorf("Sink() = %v for a plugin that declared only commands, want nil", s)
	}
}

// The observer role is one-way, and this is the bypass written as a test.
//
// Every host method carries a call id and the ids are small decimal counters,
// so a plugin can guess them without effort. This one does, on the first record
// it is shown, for calls 0 through 3 — and every one has to be refused, because
// a plugin that declared no commands has no call in the table for the whole of
// its life. Watching a shell is not a route into it.
func TestAnObserverCannotReachTheShellItIsWatching(t *testing.T) {
	t.Parallel()
	relayed := &syncBuffer{}
	h := launch(t, "sneak", plugin.Options{Stderr: relayed})
	sh := watching(t, h)
	if code := sh.run(t, "/bin/echo one"); code != 0 {
		t.Fatalf("status = %d: %s", code, sh.errs.String())
	}
	if !eventually(t, func() bool {
		return strings.Count(relayed.String(), "is not in flight") >= 4
	}) {
		t.Fatalf("relayed = %q, want all four guesses refused", relayed.String())
	}
	if got, ok := sh.r.GetVar("SNEAKED"); ok {
		t.Errorf("SNEAKED = %q: an observer set a variable in the shell it was watching", got)
	}
	// And the shell carried on, which is the other half of a refusal being the
	// right answer: nothing about being asked ends anything.
	if code := sh.run(t, "/bin/echo two"); code != 0 {
		t.Errorf("status = %d, want the shell to have carried on", code)
	}
}

// An observer cannot slow the shell down, and this is the fixture that would
// if anything could: it handshakes and then never reads its input again, so
// the pipe fills and the host's feed blocks inside a write that nothing the
// plugin does will ever complete.
//
// Emit has to go on returning throughout, and the shell has to go on running
// commands. There is no deadline anywhere in this — a deadline would report
// something other than what happened, which is #493's rule — and there does
// not need to be one: what a full buffer costs is a record, and the gap in seq
// is how the consumer is told.
func TestAnObserverThatNeverReadsCannotStopTheShell(t *testing.T) {
	t.Parallel()
	h := launch(t, "deafobserver", plugin.Options{Stderr: &syncBuffer{}})
	sink := h.Sink()
	if sink == nil {
		t.Fatal("Sink() is nil for a plugin that declared the observer role")
	}
	// Far more than the buffer holds and far more than the pipe holds, so the
	// feed is certainly blocked in a write long before the last of these.
	ctx := t.Context()
	start := time.Now()
	for i := range 20000 {
		sink.Emit(ctx, interp.Event{
			Kind:   interp.EventAccess,
			Action: interp.Action{Kind: interp.ActionStat, Path: "/srv/x"},
			Line:   i,
		})
	}
	// A bound on the test rather than on the shell. Twenty thousand
	// conversions and channel sends are milliseconds of work; anything near
	// this number means Emit waited for the plugin, which is the bug.
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("20000 events took %v: Emit is waiting for the plugin", elapsed)
	}
	sh := watching(t, h)
	if code := sh.run(t, "/bin/echo still here"); code != 0 {
		t.Fatalf("status = %d: %s", code, sh.errs.String())
	}
	if !strings.Contains(sh.out.String(), "still here") {
		t.Errorf("out = %q, want the shell to have carried on", sh.out.String())
	}
	// And Close returns, against a feed parked inside a write to a plugin that
	// will never read again. Taking the stream away is what unblocks it.
	done := make(chan error, 1)
	go func() { done <- h.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Close() = %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Close did not return: the feed is parked inside a write")
	}
}

// A plugin may take both roles, and the obligation that comes with it is
// meetable in a shell script.
//
// A notification arrives whenever the host has one, including between a host
// method and its reply, so every read the plugin does has to let events past.
// The fixture's `count N` command blocks until it has seen N records — so it is
// reading the event stream from inside one of its own calls — and then asks a
// host method, which proves the reply is still found with records going by.
func TestAPluginMayTakeBothRolesAtOnce(t *testing.T) {
	t.Parallel()
	h := launch(t, "both", plugin.Options{Stderr: &syncBuffer{}})
	if got := h.Commands(); len(got) != 1 || got[0] != "count" {
		t.Fatalf("Commands() = %v, want the command role as well", got)
	}
	sink := h.Sink()
	if sink == nil {
		t.Fatal("Sink() is nil for a plugin that declared both roles")
	}
	sh := watching(t, h)
	// Fed from a goroutine of the test's own and spaced out, so that the call
	// is certainly in flight while at least some of them arrive: the point is
	// that the plugin is reading the event stream from inside a call, not that
	// it read it at some point.
	var feeding sync.WaitGroup
	feeding.Add(1)
	go func() {
		defer feeding.Done()
		for i := range 3 {
			sink.Emit(context.Background(), interp.Event{
				Kind:   interp.EventAccess,
				Action: interp.Action{Kind: interp.ActionStat, Path: "/srv/x"},
				Line:   i,
			})
			time.Sleep(20 * time.Millisecond)
		}
	}()
	code := sh.run(t, "GREETING=hello; count 3")
	feeding.Wait()
	if code != 0 {
		t.Fatalf("status = %d: %s", code, sh.errs.String())
	}
	if want := "3 hello\n"; sh.out.String() != want {
		t.Errorf("out = %q, want %q — the count it saw, and the variable it asked for", sh.out.String(), want)
	}
}

// A record outlives the Emit that made it, and nothing in interp.Sink ever
// promised that an Event stays readable afterwards.
//
// This is the first Sink in the repository that is asynchronous. event.Encoder
// marshals on the caller's own goroutine and cmd/sh's trace sink formats on it,
// so neither ever held a reference to the interpreter's memory past the call —
// and Args is the one field of a record that is a reference. Here the feed
// goroutine marshals it later, so the copy is what stands between a plugin and
// a data race in the shell.
//
// Graded by the race detector rather than by an assertion, which is why the
// loop is long: the window is between the send and the feed picking it up, and
// one attempt would pass on a host that has the bug simply by not hitting it.
func TestARecordDoesNotKeepTheInterpretersMemory(t *testing.T) {
	t.Parallel()
	h := launch(t, "watcher", plugin.Options{Stderr: &syncBuffer{}})
	sink := h.Sink()
	ctx := t.Context()
	for i := range 300 {
		args := []string{"/bin/echo", "one", "two"}
		sink.Emit(ctx, interp.Event{
			Kind:   interp.EventCommandStart,
			Action: interp.Action{Kind: interp.ActionExec, Path: args[0], Args: args},
			Line:   i,
		})
		// What the interpreter is entitled to do with a slice it still owns.
		args[1] = "changed"
		args[2] = "again"
	}
}

// What is already numbered is delivered at shutdown, not abandoned.
//
// docs/design/plugins.md's lifetime rule says the write channel is "abandoned
// on shutdown rather than drained", and that rule is right about what it was
// written against — a plugin must not choose how long a shutdown takes. Taken
// literally it makes the role useless for the commonest shell there is: `sh -c
// cmd` emits every record it will ever emit and then exits, so what an
// abandoning host lost would be the records about the command that was
// actually run. So the drain is bounded, and this is the assertion that the
// drain exists.
//
// Fewer records than the buffer holds, so nothing is dropped and the total is
// a number rather than a range — and emitted in a tight loop, so that they are
// certainly still in the buffer when Close is called: a channel send is orders
// of magnitude cheaper than a marshal and a write, which is the whole reason
// Emit is the shape it is.
func TestWhatIsAlreadyNumberedIsDeliveredAtShutdown(t *testing.T) {
	t.Parallel()
	relayed := &syncBuffer{}
	h := launch(t, "counter", plugin.Options{Stderr: relayed})
	sink := h.Sink()
	const records = observerBufferForTest
	ctx := t.Context()
	for i := range records {
		sink.Emit(ctx, interp.Event{
			Kind:   interp.EventAccess,
			Action: interp.Action{Kind: interp.ActionStat, Path: "/srv/x"},
			Line:   i,
		})
	}
	if err := h.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	// Said at end of input, which the plugin only reaches once the host has
	// closed its standard input — so this line arriving at all means the
	// records went out first and the relay was given the chance to finish.
	if want := "total 1000"; !strings.Contains(relayed.String(), want) {
		t.Errorf("relayed = %q, want %q", relayed.String(), want)
	}
}

// A plugin still working through what it was sent is not killed part-way.
//
// This is #1906, and the mechanism turned out not to be the guess in the
// issue: the shutdown *does* drain the feed, and every record reaches the
// plugin's pipe before its input is closed. What it did not do is wait for
// the plugin to act on them — the bound ran from the moment the input was
// closed, so a plugin holding records it had not been given a processor for
// was killed with them unread. Measured before the fix with this fixture:
// one of three records handled, the other two gone with the plugin. On a
// loaded machine the ordinary fixture reached the same place with no sleep in
// it at all, which is the flake the issue was filed for.
//
// The fixture takes a second per record, which is longer than the whole bound
// — so this fails without the fix rather than merely becoming likely to.
func TestAPluginStillHandlingRecordsIsNotKilledPartWay(t *testing.T) {
	t.Parallel()
	relayed := &syncBuffer{}
	h := launch(t, "slowobserver", plugin.Options{Stderr: relayed})
	sink := h.Sink()
	if sink == nil {
		t.Fatal("Sink() is nil for a plugin that declared the observer role")
	}
	const records = 3
	ctx := t.Context()
	for i := range records {
		sink.Emit(ctx, interp.Event{
			Kind:   interp.EventAccess,
			Action: interp.Action{Kind: interp.ActionStat, Path: "/srv/x"},
			Line:   i,
		})
	}
	// A deadline on the test, because the failure this guards against in the
	// other direction is a shutdown that waits for a plugin forever: three
	// seconds of fixture, a ceiling of ten, and this well past both, so a
	// regression fails in seconds rather than hanging until the suite's own
	// timeout.
	done := make(chan error, 1)
	go func() { done <- h.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close() = %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Close did not return: a plugin that keeps writing is deciding when the shell leaves")
	}
	if got := strings.Count(relayed.String(), "handled a record"); got != records {
		t.Errorf("the plugin handled %d of %d records: %q", got, records, relayed.String())
	}
}

// And a plugin that has gone quiet is still killed, and now says so.
//
// The other half of the same bound, and the half that keeps it a bound: the
// clock restarts on a byte from the plugin, so a plugin writing nothing is
// exactly the hung plugin it was written for and goes as it always did. What
// is new is the line — a record that was put on the wire and died unread is a
// lost audit record, and one nobody is told about is the failure the observer
// role exists to prevent.
func TestAPluginKilledWithRecordsUnreadSaysSo(t *testing.T) {
	t.Parallel()
	relayed := &syncBuffer{}
	h := launch(t, "deafobserver", plugin.Options{Stderr: relayed})
	h.Sink().Emit(t.Context(), interp.Event{
		Kind:   interp.EventAccess,
		Action: interp.Action{Kind: interp.ActionStat, Path: "/srv/x"},
	})
	if err := h.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if !strings.Contains(relayed.String(), "anything it had not yet read is lost") {
		t.Errorf("relayed = %q, want the shell to have said the record was lost", relayed.String())
	}
}

// observerBufferForTest is one fewer than nothing in particular: it is a
// number chosen to be under the host's buffer so that no record is dropped,
// stated here rather than derived from the unexported constant because an
// external test asserting on an internal number would be asserting on the
// wrong thing. If the buffer ever shrinks below this, this test fails loudly
// rather than quietly measuring drops.
const observerBufferForTest = 1000
