// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// The gate and the event stream are the interpreter's, and this package is
// how a binary supplies them.
//
// Until these fields existed nothing that shipped ever set either one. The
// seam was unit-tested inside interp from its first commit and completely
// unreachable from outside: no binary mentioned a Gate, so the conformance
// harness and the wild sweep both ran ungated, and a hole in the boundary
// would have looked exactly like a shell that works. These tests are the
// coverage that was missing — not of what the gate decides, which is interp's,
// but of whether a shell built here is ever asked.

// recorder is a Gate and a Sink in one, with a rule about what to refuse.
//
// Guarded, because both are called from more than one goroutine: a background
// job asks from the goroutine running it, and so does each half of a
// pipeline. The first gate ever written against this seam was an unguarded
// closure appending to a slice, and it raced the moment a script said `&`.
type recorder struct {
	mu     sync.Mutex
	deny   func(interp.Action) bool
	asked  []interp.Action
	events []interp.Event
}

func (r *recorder) Allow(_ context.Context, a interp.Action) interp.Decision {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.asked = append(r.asked, a)
	if r.deny != nil && r.deny(a) {
		return interp.Deny
	}
	return interp.Allow
}

func (r *recorder) Emit(_ context.Context, e interp.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

// seen reports whether any event matched, and is the only way the tests read
// the slice — taking the lock, because the shell may still be finishing.
func (r *recorder) seen(match func(interp.Event) bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		if match(e) {
			return true
		}
	}
	return false
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

func denyKind(k interp.ActionKind) func(interp.Action) bool {
	return func(a interp.Action) bool { return a.Kind == k }
}

// TestEveryInvocationRouteRunsGated is the assertion the fields exist for.
//
// A Runner is built in one place so that the routes cannot disagree, and this
// is the same claim TestTheInvocationRouteDoesNotChangeTheAnswer makes about
// what a script prints, asked about the boundary instead: a gate installed for
// `-c` and missing at a prompt would be a hole nothing else could see.
func TestEveryInvocationRouteRunsGated(t *testing.T) {
	// A name that resolves to nothing is still an action, and the gate is
	// asked before the failure is reported — so this needs no program on
	// the machine to be a real refusal.
	const src = "nosuchprogram\necho status=$?\n"

	for _, tc := range []struct {
		name string
		run  func(t *testing.T, sh driver.Shell) (out, errs string)
	}{
		{"-c", func(t *testing.T, sh driver.Shell) (string, string) {
			out, errs, _ := runArgs(t, sh, "testsh", "-c", src)
			return out, errs
		}},
		{"a script file", func(t *testing.T, sh driver.Shell) (string, string) {
			out, errs, _ := runArgs(t, sh, "testsh", writeScript(t, src))
			return out, errs
		}},
		{"standard input", func(t *testing.T, sh driver.Shell) (string, string) {
			out, errs, _ := runPipedShell(t, sh, src, "testsh")
			return out, errs
		}},
		{"a prompt", func(t *testing.T, sh driver.Shell) (string, string) {
			out, errs, _ := runPipedShell(t, sh, src, "testsh", "-i")
			return out, errs
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{deny: denyKind(interp.ActionExec)}
			sh := shell()
			sh.Gate, sh.Events = rec, rec

			out, errs := tc.run(t, sh)

			// The refusal reached the script: a denied command is a command
			// that failed, with the status any command that would not start
			// carries.
			if !strings.Contains(out, "status=126") {
				t.Errorf("out = %q, want the refusal to have failed the command", out)
			}
			if !strings.Contains(errs, "refused") {
				t.Errorf("err = %q, want the refusal reported", errs)
			}
			// And the observer saw it, which is the half a status cannot
			// prove: a shell that failed the command for some other reason
			// would print the same line.
			if !rec.seen(func(e interp.Event) bool {
				return e.Kind == interp.EventDenied && e.Action.Kind == interp.ActionExec
			}) {
				t.Error("no denial reached the event stream")
			}
		})
	}
}

// TestNoGateMeansTheShellItAlwaysWas pins the default.
//
// Both fields are nil for every binary that has not asked for one, and nil has
// to mean today's behavior rather than a policy that happens to allow
// everything: the same script, the same status, the same diagnostic.
func TestNoGateMeansTheShellItAlwaysWas(t *testing.T) {
	const src = "nosuchprogram\necho status=$?\n"

	plainOut, plainErr, _ := runArgs(t, shell(), "testsh", "-c", src)

	// A gate that allows everything must not change anything either — if it
	// did, the fields would be observable to a script and the seam would be
	// a semantics axis rather than an attachment point.
	rec := &recorder{}
	sh := shell()
	sh.Gate, sh.Events = rec, rec
	gatedOut, gatedErr, _ := runArgs(t, sh, "testsh", "-c", src)

	if plainOut != gatedOut || plainErr != gatedErr {
		t.Errorf("an allow-everything gate changed the run:\nwithout: %q / %q\nwith:    %q / %q",
			plainOut, plainErr, gatedOut, gatedErr)
	}
	// The failure is still the one a shell gives for a name it cannot
	// resolve, not a refusal.
	if strings.Contains(plainErr, "refused") {
		t.Errorf("err = %q, want a not-found failure and not a refusal", plainErr)
	}
	if !strings.Contains(plainOut, "status=127") {
		t.Errorf("out = %q, want the status for a name that resolved to nothing", plainOut)
	}
}

// TestADeniedProbeLooksLikeAPathThatIsNotThere is the semantics the action
// vocabulary documents, asserted from the outside for the first time.
//
// A refused stat answers as a missing path does, quietly. It matters here
// rather than only in interp because a diagnostic leaking out of the front end
// would identify the refusal — a script could then tell "hidden by policy"
// from "not installed", which is exactly what the quiet deny exists to
// prevent.
func TestADeniedProbeLooksLikeAPathThatIsNotThere(t *testing.T) {
	victim := filepath.Join(t.TempDir(), "present")
	if err := os.WriteFile(victim, []byte("real\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := "if [ -f " + victim + " ]; then echo there; else echo missing; fi\n"

	// The control: the file is really there, so an ungated shell finds it.
	out, _, _ := runArgs(t, shell(), "testsh", "-c", src)
	if strings.TrimSpace(out) != "there" {
		t.Fatalf("ungated out = %q, want the file to be found", out)
	}

	rec := &recorder{deny: func(a interp.Action) bool {
		return a.Kind == interp.ActionStat && a.Path == victim
	}}
	sh := shell()
	sh.Gate, sh.Events = rec, rec
	out, errs, code := runArgs(t, sh, "testsh", "-c", src)

	if strings.TrimSpace(out) != "missing" {
		t.Errorf("out = %q, want the hidden path to test as absent", out)
	}
	if errs != "" {
		t.Errorf("err = %q, want a refused probe to say nothing", errs)
	}
	if code != 0 {
		t.Errorf("status = %d, want the run to succeed: a probe that answered is not a failure", code)
	}
	if !rec.seen(func(e interp.Event) bool {
		return e.Kind == interp.EventDenied && e.Action.Kind == interp.ActionStat
	}) {
		t.Error("the refusal was quiet to the script but must still reach the observer")
	}
}

// TestASignalARealShellSendsIsGatedHereToo is the boundary a shell binary
// crosses that a library cannot.
//
// `kill` is a builtin, so making it one moved `kill -9 1234` out of the
// boundary that an external kill's exec had put it in: the process really ends
// and nothing above the interpreter could refuse it. This is that path through
// a shell built here, with a real process on the other end — the only witness
// that can tell a refusal from a diagnostic printed after the fact.
func TestASignalARealShellSendsIsGatedHereToo(t *testing.T) {
	victim := exec.Command("/bin/sleep", "30")
	if err := victim.Start(); err != nil {
		t.Skipf("no process to signal: %v", err)
	}
	ended := make(chan struct{})
	go func() {
		_ = victim.Wait()
		close(ended)
	}()
	t.Cleanup(func() {
		_ = victim.Process.Kill()
		<-ended
	})
	src := "kill -TERM " + strconv.Itoa(victim.Process.Pid) + "\necho status=$?\n"

	rec := &recorder{deny: denyKind(interp.ActionSignal)}
	sh := shell()
	sh.Gate, sh.Events = rec, rec
	out, errs, _ := runArgs(t, sh, "testsh", "-c", src)

	select {
	case <-ended:
		t.Fatal("the process the gate refused to signal died anyway")
	case <-time.After(200 * time.Millisecond):
	}
	// EPERM-shaped, so the shell prints what it prints for a process that is
	// not ours and the script cannot tell which refused it.
	if !strings.Contains(errs, "Operation not permitted") {
		t.Errorf("err = %q, want the wording for a process that may not be signaled", errs)
	}
	if !strings.Contains(out, "status=1") {
		t.Errorf("out = %q, want the status a kill that was not permitted carries", out)
	}
	if !rec.seen(func(e interp.Event) bool {
		return e.Kind == interp.EventDenied && e.Action.Kind == interp.ActionSignal &&
			e.Action.PID == victim.Process.Pid && e.Action.Signal == syscall.SIGTERM
	}) {
		t.Error("no signal denial reached the event stream")
	}
}

// TestTheEventStreamRecordsWhatTheShellDid is the other half: the gate is
// asked, and the sink is told.
//
// One script, because the point is the stream rather than any one record — a
// command that started, the status it ended with, and a file it opened, all
// arriving at a Sink a binary supplied through this package.
func TestTheEventStreamRecordsWhatTheShellDid(t *testing.T) {
	out := filepath.Join(t.TempDir(), "written")
	rec := &recorder{}
	sh := shell()
	sh.Gate, sh.Events = rec, rec

	_, errs, code := runArgs(t, sh, "testsh", "-c", "/bin/echo hi > "+out)
	if code != 0 {
		t.Fatalf("status %d: %s", code, errs)
	}

	if !rec.seen(func(e interp.Event) bool {
		return e.Kind == interp.EventCommandStart && e.Action.Kind == interp.ActionExec &&
			strings.HasSuffix(e.Action.Path, "/echo")
	}) {
		t.Error("no command-start for the program that ran")
	}
	if !rec.seen(func(e interp.Event) bool {
		return e.Kind == interp.EventCommandEnd && e.Status == 0
	}) {
		t.Error("no command-end carrying the status")
	}
	// The redirect's open, with the direction on it: a record that did not
	// say which way a file was opened would be useless to an audit trail.
	if !rec.seen(func(e interp.Event) bool {
		return e.Kind == interp.EventAccess && e.Action.Kind == interp.ActionOpen &&
			e.Action.Path == out && e.Action.Write
	}) {
		t.Errorf("no access record for the file the redirect wrote to (%s)", out)
	}
	// Every event says where it came from, which for a command string is a
	// line and no file.
	if !rec.seen(func(e interp.Event) bool { return e.Line > 0 }) {
		t.Error("no event carried a source line")
	}
}

// TestAGateSuppliedHereIsAskedFromEveryGoroutine is the contract stated on
// interp.Gate, asserted through the field that supplies one.
//
// A background job and each half of a pipeline run on their own goroutine, so
// a policy installed by a binary is concurrent whether or not its author
// thought about it. Under -race this fails loudly if the seam ever stops
// holding; without it, the counts still say every branch was asked.
func TestAGateSuppliedHereIsAskedFromEveryGoroutine(t *testing.T) {
	rec := &recorder{}
	sh := shell()
	sh.Gate, sh.Events = rec, rec

	_, errs, code := runArgs(t, sh, "testsh", "-c",
		"/bin/echo one & /bin/echo two | /bin/cat; wait\n")
	if code != 0 {
		t.Fatalf("status %d: %s", code, errs)
	}
	if rec.count() == 0 {
		t.Fatal("nothing reached the sink from a background job or a pipeline")
	}
}
