// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// End to end, through the flag a person types, and through run() rather than
// through a copy of what main does — the mistake this file exists to catch is
// wiring that is present and not reached.
//
// The plugin itself is a POSIX shell script written by the test. That is the
// claim being checked as much as the wiring: a plugin needs no library, no
// code generator and no toolchain, so if this needed a Go program built first
// the design goal would not have been met.

// writePlugin writes an executable plugin and answers its absolute path.
func writePlugin(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "aplugin")
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// greeter is a whole plugin. Everything below the shebang is the protocol:
// read a line, write a line.
const greeter = `#!/bin/sh
exec 3>&1
send() { printf '%s\n' "$1" >&3; }
while IFS= read -r line; do
	id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
	case $line in
	*'"method":"initialize"'*)
		send "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"protocolVersion\":1,\"name\":\"greeter\",\"commands\":[\"hail\"]}}"
		;;
	*'"method":"command/invoke"'*)
		call=$(printf '%s' "$line" | sed -n 's/.*"call":"\([^"]*\)".*/\1/p')
		arg=$(printf '%s' "$line" | sed -n 's/.*"args":\["\([^"]*\)".*/\1/p')
		data=$(printf 'hail %s from a shell script\n' "$arg" | base64 | tr -d '\n')
		send "{\"jsonrpc\":\"2.0\",\"method\":\"command/output\",\"params\":{\"call\":\"$call\",\"stream\":\"out\",\"data\":\"$data\"}}"
		send "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"status\":0}}"
		;;
	esac
done
`

// The whole point, in one test: a command written in something that is not Go
// runs as a builtin of this shell.
func TestAPluginCommandRunsFromTheFlag(t *testing.T) {
	path := writePlugin(t, greeter)
	got := sandboxed(t, "posix", "-plugin", path, "-c", "hail world")
	if got.code != 0 {
		t.Fatalf("status = %d, err = %q", got.code, got.errs)
	}
	if want := "hail world from a shell script\n"; got.out != want {
		t.Errorf("out = %q, want %q", got.out, want)
	}
}

// A plugin's command is a builtin, which a script can see the ordinary way
// and cannot distinguish from any other.
func TestAPluginCommandIsABuiltinToAScript(t *testing.T) {
	path := writePlugin(t, greeter)
	got := sandboxed(t, "posix", "-plugin", path, "-c", "type hail")
	if got.code != 0 {
		t.Fatalf("status = %d, err = %q", got.code, got.errs)
	}
	if !strings.Contains(got.out, "builtin") {
		t.Errorf("out = %q, want `type` to call it a builtin", got.out)
	}
}

// A shell function shadows it, so a prelude beats a plugin by construction
// rather than by convention.
func TestAPreludeFunctionBeatsAPluginCommand(t *testing.T) {
	path := writePlugin(t, greeter)
	got := sandboxed(t, "posix", "-plugin", path, "-c", "hail() { printf 'mine\\n'; }; hail world")
	if got.out != "mine\n" {
		t.Errorf("out = %q, want the function to have won", got.out)
	}
}

// Repeatable, and the commands compose.
func TestTwoPluginsBothRegister(t *testing.T) {
	first := writePlugin(t, greeter)
	second := writePlugin(t, strings.ReplaceAll(strings.ReplaceAll(greeter,
		`\"hail\"`, `\"salute\"`), "hail %s", "salute %s"))
	got := sandboxed(t, "posix", "-plugin", first, "-plugin", second, "-c", "hail a; salute b")
	if got.code != 0 {
		t.Fatalf("status = %d, err = %q", got.code, got.errs)
	}
	for _, want := range []string{"hail a from", "salute b from"} {
		if !strings.Contains(got.out, want) {
			t.Errorf("out = %q, want %q", got.out, want)
		}
	}
}

// A plugin that does not come up is a fatal invocation error, status 2, and
// the shell does not run. The alternative is a shell that resolves that name
// from PATH instead, running something other than what the invocation asked
// for.
func TestAPluginThatWillNotStartIsFatal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-here")
	got := sandboxed(t, "posix", "-plugin", path, "-c", "printf 'ran anyway\\n'")
	if got.code != exitFailure {
		t.Errorf("status = %d, want %d", got.code, exitFailure)
	}
	if strings.Contains(got.out, "ran anyway") {
		t.Errorf("out = %q, want the shell not to have run", got.out)
	}
	if !strings.Contains(got.errs, "plugin") || !strings.Contains(got.errs, path) {
		t.Errorf("err = %q, want it to name the plugin and its path", got.errs)
	}
}

// A relative path is refused, and the diagnostic says why rather than just
// failing to find it. A searched plugin is arbitrary code chosen by whoever
// set the variable.
func TestARelativePluginPathIsRefused(t *testing.T) {
	got := sandboxed(t, "posix", "-plugin", "aplugin", "-c", "true")
	if got.code != exitFailure {
		t.Errorf("status = %d, want %d", got.code, exitFailure)
	}
	if !strings.Contains(got.errs, "absolute") {
		t.Errorf("err = %q, want it to say the path must be absolute", got.errs)
	}
}

func TestPluginRequiresAPath(t *testing.T) {
	got := sandboxed(t, "posix", "-plugin")
	if got.code != exitFailure {
		t.Errorf("status = %d, want %d", got.code, exitFailure)
	}
	if !strings.Contains(got.errs, "-plugin requires") {
		t.Errorf("err = %q, want it to say what the flag needs", got.errs)
	}
}

// Under a default-deny policy a plugin does not start unless the policy names
// it, because launching one is an exec and passes the gate.
func TestAPolicyThatDoesNotNameThePluginRefusesIt(t *testing.T) {
	path := writePlugin(t, greeter)
	policy := writePolicy(t, "allow read /**")
	got := sandboxed(t, "posix", "-policy", policy, "-plugin", path, "-c", "hail world")
	if got.code != exitFailure {
		t.Errorf("status = %d, want the invocation refused", got.code)
	}
	if !strings.Contains(got.errs, "refused") {
		t.Errorf("err = %q, want the refusal named", got.errs)
	}
	if strings.Contains(got.out, "hail world") {
		t.Errorf("out = %q, want the plugin never to have run", got.out)
	}
}

// And a policy that does name it lets it through, so the refusal above is the
// policy deciding rather than the flag being broken.
func TestAPolicyThatNamesThePluginAllowsIt(t *testing.T) {
	path := writePlugin(t, greeter)
	// The plugin is a shell script, so the kernel runs /bin/sh on it: the
	// interpreter is what gets exec'd and the script is what gets read. Both
	// are the *plugin's* own accesses, outside the boundary — but the exec of
	// the plugin path is ours and is what the policy has to name.
	policy := writePolicy(t,
		"allow read /**",
		"allow exec "+path,
		"allow exec /bin/**",
		"allow exec /usr/bin/**",
	)
	got := sandboxed(t, "posix", "-policy", policy, "-plugin", path, "-c", "hail world")
	if got.code != 0 {
		t.Fatalf("status = %d, err = %q", got.code, got.errs)
	}
	if want := "hail world from a shell script\n"; got.out != want {
		t.Errorf("out = %q, want %q", got.out, want)
	}
}

// The exec that launched the plugin is in the audit trail, under the same run
// as everything the script did. A record naming no run is a record nothing can
// be joined to, and the plugin's launch happens before the front end has
// picked an identity.
func TestThePluginsLaunchIsAuditedUnderTheRunsOwnSession(t *testing.T) {
	path := writePlugin(t, greeter)
	log := filepath.Join(t.TempDir(), "audit.jsonl")
	// The script has to make an audited access of its own, or the trail holds
	// only the plugin's exec and "one session" is trivially true. Found by
	// mutation: dropping the session assignment in launchPlugins left this
	// test green.
	read := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(read, []byte("hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := sandboxed(t, "posix", "-audit", log, "-plugin", path, "-c", "hail world; read v < "+read)
	if got.code != 0 {
		t.Fatalf("status = %d, err = %q", got.code, got.errs)
	}
	body, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("reading the audit stream: %v", err)
	}
	sessions := map[string]bool{}
	records := 0
	sawPlugin := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(body)), "\n") {
		if line == "" {
			continue
		}
		var r struct {
			Event   string `json:"event"`
			Action  string `json:"action"`
			Path    string `json:"path"`
			Session string `json:"session"`
			ActonID string `json:"actionId"`
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("record %q: %v", line, err)
		}
		sessions[r.Session] = true
		records++
		if r.Action == "exec" && r.Path == path {
			sawPlugin = true
			if r.ActonID == "" {
				t.Error("the plugin's exec carries no action id")
			}
		}
	}
	if !sawPlugin {
		t.Errorf("the plugin's own exec is not in the trail:\n%s", body)
	}
	if len(sessions) != 1 {
		t.Errorf("the trail holds %d sessions, want one per run: %v", len(sessions), sessions)
	}
	if records < 2 {
		t.Errorf("the trail holds %d records, want the plugin's exec and the script's own access — "+
			"one session is trivially true for one record", records)
	}
}

// The flag is listed where a person looks for it. A flag that works and is
// undocumented is a flag nobody uses.
func TestUsageMentionsThePluginFlag(t *testing.T) {
	got := sandboxed(t, "", "-h")
	if !strings.Contains(got.out, "-plugin PATH") {
		t.Errorf("usage does not mention -plugin:\n%s", got.out)
	}
}

// The plugin's registrations are composed with the dialect's rather than
// replacing them, so a plugin does not cost a shell its own builtins.
//
// Found by mutation: dropping the dialect's Register from the composition left
// every other test in this file green, because the tests that assert about a
// builtin all use one the core provides.
func TestAPluginDoesNotDisplaceTheDialectsOwnBuiltins(t *testing.T) {
	path := writePlugin(t, greeter)
	// `shopt` is bash's and is registered by the dialect, not by the core.
	got := sandboxed(t, "bash", "-plugin", path, "-c", "shopt -q nullglob; printf '%s\\n' \"$?\"; hail world")
	if got.code != 0 {
		t.Fatalf("status = %d, err = %q", got.code, got.errs)
	}
	if !strings.Contains(got.out, "hail world from a shell script") {
		t.Errorf("out = %q, want the plugin's command to have run", got.out)
	}
	if strings.Contains(got.errs, "shopt") {
		t.Errorf("err = %q, want the dialect's own builtin to still be there", got.errs)
	}
}

// farewell answers the handshake and then says something on its error stream
// when its input ends, which is how a plugin sees an orderly shutdown.
const farewell = `#!/bin/sh
exec 3>&1
send() { printf '%s\n' "$1" >&3; }
while IFS= read -r line; do
	id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
	case $line in
	*'"method":"initialize"'*)
		send "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"protocolVersion\":1,\"name\":\"farewell\",\"commands\":[\"bye\"]}}"
		;;
	esac
done
printf 'shut down cleanly\n' >&2
`

// A plugin that fails to launch takes down the ones already up.
//
// The invocation asked for all of them, so a shell that ran with the first two
// of three would be running something other than what was asked for — and the
// two it started would be processes outliving a shell that never began.
func TestAFailedLaunchClosesThePluginsAlreadyStarted(t *testing.T) {
	good := writePlugin(t, farewell)
	bad := filepath.Join(t.TempDir(), "not-here")
	got := sandboxed(t, "posix", "-plugin", good, "-plugin", bad, "-c", "true")
	if got.code != exitFailure {
		t.Fatalf("status = %d, want %d", got.code, exitFailure)
	}
	if !strings.Contains(got.errs, "shut down cleanly") {
		t.Errorf("err = %q, want the plugin that did start to have been closed", got.errs)
	}
}

// watcher is a whole observer plugin, and it is shorter than the greeter: it
// declares no commands at all, takes the event stream, and writes what it saw
// to its own standard error, which the host relays.
//
// The records it receives are event.Records, the same JSON Lines the audit file
// holds. That is the point of the role costing one method: internal/event was
// written as a shared contract, and a consumer of an audit stream is already a
// consumer of this.
const watcher = `#!/bin/sh
exec 3>&1
send() { printf '%s\n' "$1" >&3; }
while IFS= read -r line; do
	case $line in
	*'"method":"observer/event"'*)
		ev=$(printf '%s' "$line" | sed -n 's/.*"event":"\([^"]*\)".*/\1/p')
		act=$(printf '%s' "$line" | sed -n 's/.*"action":"\([^"]*\)".*/\1/p')
		path=$(printf '%s' "$line" | sed -n 's/.*"path":"\([^"]*\)".*/\1/p')
		printf 'saw %s %s %s\n' "$ev" "$act" "$path" >&2
		continue
		;;
	esac
	id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
	case $line in
	*'"method":"initialize"'*)
		send "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"protocolVersion\":1,\"name\":\"watcher\",\"commands\":[],\"observer\":true}}"
		;;
	esac
done
`

// The observer role, through the flag a person types: a plugin in another
// language is told what this shell ran.
func TestAnObserverPluginIsToldWhatTheShellRan(t *testing.T) {
	path := writePlugin(t, watcher)
	got := sandboxed(t, "posix", "-plugin", path, "-c", "/bin/echo hello")
	if got.code != 0 {
		t.Fatalf("status = %d, err = %q", got.code, got.errs)
	}
	if got.out != "hello\n" {
		t.Errorf("out = %q, want the command's own output untouched", got.out)
	}
	// Records the shell emitted while running the command, relayed back
	// through the plugin's standard error and prefixed with its name.
	for _, want := range []string{
		"sh: plugin watcher: saw command-start exec",
		"sh: plugin watcher: saw command-end exec",
	} {
		if !strings.Contains(got.errs, want) {
			t.Errorf("errs = %q, want %q", got.errs, want)
		}
	}
}

// A command plugin is not shown the event stream, and this is the bypass
// written the way the gate checks are: the plugin says so out loud if it is
// ever handed one.
//
// What is at stake is a disclosure rather than a capability. The stream is
// every command the shell ran and every path it touched; a person who wanted
// one word implemented in another language did not thereby ask for a copy of
// their session to be sent to it. The role has to be declared, and this is the
// declaration being load-bearing rather than decorative.
func TestAPluginThatDidNotAskIsNotShownTheEventStream(t *testing.T) {
	nosy := strings.Replace(greeter,
		"\tcase $line in\n\t*'\"method\":\"initialize\"'*)",
		"\tcase $line in\n\t*'\"method\":\"observer/event\"'*)\n\t\tprintf 'LEAKED\\n' >&2\n\t\t;;\n\t*'\"method\":\"initialize\"'*)", 1)
	if nosy == greeter {
		t.Fatal("the fixture was not rewritten: this test would pass without checking anything")
	}
	path := writePlugin(t, nosy)
	got := sandboxed(t, "posix", "-plugin", path, "-c", "/bin/echo one; hail two")
	if got.code != 0 {
		t.Fatalf("status = %d, err = %q", got.code, got.errs)
	}
	if !strings.Contains(got.out, "hail two from a shell script") {
		t.Fatalf("out = %q, want the plugin to have been running at all", got.out)
	}
	if strings.Contains(got.errs, "LEAKED") {
		t.Errorf("errs = %q: a plugin that declared only commands was sent the event stream", got.errs)
	}
}

// An observer plugin is composed onto the record the invocation already asked
// for, never in place of it.
//
// A person who adds a plugin to a shell that was already keeping a trace did
// not ask for the trace to stop, and an audit trail a plugin can silence is not
// an audit trail. Both have to be there in one run.
func TestAnObserverDoesNotSilenceTheRecordTheShellWasAlreadyKeeping(t *testing.T) {
	path := writePlugin(t, watcher)
	got := sandboxed(t, "posix", "-trace-events", "-plugin", path, "-c", "/bin/echo hello")
	if got.code != 0 {
		t.Fatalf("status = %d, err = %q", got.code, got.errs)
	}
	if !strings.Contains(got.errs, "trace: command-start exec") {
		t.Errorf("errs = %q, want the trace the invocation asked for", got.errs)
	}
	if !strings.Contains(got.errs, "sh: plugin watcher: saw command-start exec") {
		t.Errorf("errs = %q, want the plugin to have been told as well", got.errs)
	}
}

// No plugin is shown another plugin's launch.
//
// Every -plugin is launched before any observer sink is composed, so the exec
// that starts one is recorded to the trace and to the audit file and to no
// plugin. The alternative would make what a plugin can see depend on where its
// flag sat in the argument list, which is a disclosure rule nobody could read
// off a command line — and it would mean that adding a second plugin quietly
// widened what the first one is told.
func TestNoPluginIsShownAnotherPluginsLaunch(t *testing.T) {
	// Both orders, because the rule is that the order does not matter and a
	// test that fixed one would only hold half of it: an observer named second
	// is not yet composed when the first plugin starts *by accident of
	// sequence*, and an observer named first is the case where composing as
	// each launch happened would tell it about the next one.
	for _, order := range []struct {
		name           string
		command, watch string
	}{
		{"the observer named second", greeter, watcher},
		{"the observer named first", watcher, greeter},
	} {
		first := writePlugin(t, order.command)
		second := writePlugin(t, order.watch)
		got := sandboxed(t, "posix", "-plugin", first, "-plugin", second, "-c", "/bin/echo hi")
		if got.code != 0 {
			t.Fatalf("%s: status = %d, err = %q", order.name, got.code, got.errs)
		}
		if !strings.Contains(got.errs, "saw command-start exec /bin/echo") {
			t.Fatalf("%s: errs = %q, want the observer to have been watching at all", order.name, got.errs)
		}
		for _, path := range []string{first, second} {
			if strings.Contains(got.errs, path) {
				t.Errorf("%s: errs = %q: an observer was shown the exec that launched %s",
					order.name, got.errs, path)
			}
		}
	}
}

// chatterer is an observer plugin that writes to its standard error for as
// long as the shell is up, rather than once per event.
//
// The background loop is the point of the fixture. A relay that writes one
// line while a command happens to be writing another is what #1138 was: a
// window narrow enough that five runs on darwin all pass and an unrelated
// pull request's Linux job fails. Widening it is what makes the overlap a
// thing a test can assert instead of a thing CI reports at random.
const chatterer = `#!/bin/sh
exec 3>&1
send() { printf '%s\n' "$1" >&3; }
while IFS= read -r line; do
	case $line in
	*'"method":"observer/event"'*)
		continue
		;;
	esac
	id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
	case $line in
	*'"method":"initialize"'*)
		send "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"protocolVersion\":1,\"name\":\"chatterer\",\"commands\":[],\"observer\":true}}"
		(i=0; while [ $i -lt 200 ]; do printf 'chatter %s\n' "$i" >&2; i=$((i+1)); done) &
		;;
	esac
done
`

// overlapProbe is an io.Writer that reports whether two goroutines were ever
// inside Write at the same time.
//
// It answers the question directly rather than leaving it to `-race`, and that
// is deliberate: a race detector reports what it happened to observe on the
// scheduling it happened to get, so a test that relies on one is a test that
// passes on darwin and fails on somebody else's Linux job — which is the whole
// of #1138's cost. This fails on any run where the overlap occurs, with or
// without the detector.
//
// It is not itself racy: the wrapped buffer is written under the same lock
// that keeps the bookkeeping, so the probe never adds the fault it is looking
// for. What it reports is whether the *caller* serialized, which is the
// property under test.
type overlapProbe struct {
	mu         sync.Mutex
	inside     int
	overlapped bool
	buf        strings.Builder
	// hold is how long each write stays inside, which widens the window a
	// real overlap has to land in. Without it the two goroutines have to be
	// scheduled within microseconds of each other, which is exactly the
	// coin-flip this test exists to stop depending on.
	hold time.Duration
}

func (p *overlapProbe) Write(b []byte) (int, error) {
	p.mu.Lock()
	p.inside++
	if p.inside > 1 {
		p.overlapped = true
	}
	p.mu.Unlock()
	time.Sleep(p.hold)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inside--
	return p.buf.Write(b)
}

func (p *overlapProbe) report() (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buf.String(), p.overlapped
}

// A plugin's stderr relay and the shell's own writes never overlap on a
// writer the caller supplied.
//
// #1138: the relay runs on a goroutine of the host's for as long as the plugin
// lives, and an external command whose stderr is not an *os.File is copied
// there by a goroutine of os/exec's. Two writers, one io.Writer, and the race
// detector found the pair on Linux — a `fmt.Fprintf` from internal/plugin's
// relay against an `io.Copy` from os/exec — on a pull request whose diff
// touched no plugin code and no goroutine.
//
// The relay is *not* unjoined, which is worth saying because it looks like the
// obvious cause: Host.Close waits on relayDone and run() calls it before
// returning, so nothing reads the buffer while a goroutine is still writing
// it. The two writes are simultaneous in the middle of the run, and there is
// no ordering to restore — only an overlap to exclude, with a lock.
func TestAPluginsRelayDoesNotWriteTheShellsStreamAtTheSameTimeAsTheShell(t *testing.T) {
	path := writePlugin(t, chatterer)
	errs := &overlapProbe{hold: 200 * time.Microsecond}
	var out bytes.Buffer
	code := run([]string{
		"sh", "-dialect", "posix", "-plugin", path, "-c",
		"i=0; while [ $i -lt 6 ]; do /bin/sh -c 'j=0; while [ $j -lt 40 ]; do echo noise >&2; j=$((j+1)); done'; i=$((i+1)); done",
	}, &out, errs)
	text, overlapped := errs.report()
	if code != 0 {
		t.Fatalf("status = %d, err = %q", code, text)
	}
	if overlapped {
		t.Error("two goroutines were inside Write on the shell's stderr at once: " +
			"the plugin's relay is not serialized against the shell's own writes")
	}
	// The run has to have exercised both writers, or the assertion above is
	// about nothing at all.
	if !strings.Contains(text, "chatterer: chatter") {
		t.Errorf("errs = %q, want the plugin's relayed chatter in it", text)
	}
	if !strings.Contains(text, "noise") {
		t.Errorf("errs = %q, want the command's own standard error in it", text)
	}
}
