// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
	"github.com/blairham/sh/syntax"
)

// `--policy` and `--audit`, on the front end every binary is made of.
//
// This is the coverage #1826 is about. The sandbox sweep reaches the four
// dialects through `sh -dialect X`, which grades the *gate* and says nothing
// about whether the binary somebody actually invokes has a flag to put a
// policy in — and the flag was on `cmd/sh` and on nothing else, so `bash
// -policy p` answered `unknown option`. Everything below drives MainArgs,
// which is the whole of a dialect binary's main: `func main() {
// os.Exit(driver.Main(shell())) }`.
//
// The same failure has happened here once already in the other direction:
// `cmd/sh` learned to run a script file, the dialect binaries did not, and
// `make conformance-dialects` graded the drivers rather than the dialects.

// gatedShell is a shell with buffers for streams, built from the core vectors
// so that nothing here depends on which dialect is being.
func gatedShell(out, errs *strings.Builder) driver.Shell {
	return driver.Shell{
		Name:        "testsh",
		Dialect:     syntax.Core(),
		Semantics:   interp.PosixSemantics(),
		Diagnostics: interp.Diagnostics{},
		Stdout:      out,
		Stderr:      errs,
	}
}

// policyFile writes a policy that permits only ws, and returns its path.
//
// Beside the run rather than inside the workspace, so that a script which
// enumerates or writes the workspace cannot see the file deciding its own
// fate.
func policyFile(t *testing.T, dir, ws string, permitAll bool) string {
	t.Helper()
	lines := []string{"version 1", "default deny", "allow path " + ws + "/**", "allow signal"}
	if permitAll {
		lines = []string{"version 1", "default deny", "allow path /**", "allow signal"}
	}
	at := filepath.Join(dir, "rules.policy")
	if err := os.WriteFile(at, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return at
}

// The three runs, on the flag route rather than through `sh -dialect`.
//
// One run cannot decide this and that is the whole discipline `make sandbox`
// is built on: a route reporting "refused" from a shell that never started
// looks exactly like a sandbox that works. So the same write is attempted
// three times — with no policy, with one that forbids it, and with one that
// permits it — and only all three agreeing says the flag reached the gate.
func TestThePolicyFlagOnEveryBinaryGatesAWrite(t *testing.T) {
	for _, spelling := range []func(p string) []string{
		func(p string) []string { return []string{"--policy", p} },
		func(p string) []string { return []string{"--policy=" + p} },
	} {
		dir := t.TempDir()
		ws := filepath.Join(dir, "ws")
		if err := os.MkdirAll(ws, 0o755); err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(dir, "outside")
		script := "echo written > " + outside

		run := func(args ...string) (string, int) {
			_ = os.Remove(outside)
			var out, errs strings.Builder
			code := driver.MainArgs(gatedShell(&out, &errs),
				append(append([]string{"testsh"}, args...), "-c", script))
			return errs.String(), code
		}

		if _, code := run(); code != 0 {
			t.Fatalf("ungated status %d", code)
		}
		if _, err := os.Stat(outside); err != nil {
			t.Fatal("the ungated run wrote nothing, so the other two measure nothing")
		}

		denied := policyFile(t, dir, ws, false)
		errs, code := run(spelling(denied)...)
		if code == 0 {
			t.Errorf("denied status 0, want the write refused")
		}
		if _, err := os.Stat(outside); err == nil {
			t.Errorf("the file exists: %s said nothing to the gate", strings.Join(spelling(denied), " "))
		}
		if !strings.Contains(errs, "refused") {
			t.Errorf("stderr = %q, want the refusal reported", errs)
		}

		// And a policy that permits it lets it through again. Without this a
		// gate that refuses everything would score as well as one that works.
		allowed := policyFile(t, t.TempDir(), ws, true)
		if _, code := run(spelling(allowed)...); code != 0 {
			t.Errorf("allowed status %d, want the write to go through", code)
		}
		if _, err := os.Stat(outside); err != nil {
			t.Error("a policy permitting the whole tree refused the write anyway")
		}
	}
}

// The script operand is inside the boundary on this route too.
//
// The program a shell was pointed at is an access chosen by whoever invoked
// it, so the gate has to be installed before the operands are read. It is the
// one ordering in this change that is easy to get wrong and impossible to see:
// a shell that opened its script before installing the policy would refuse
// every line of it and pass every test that only ran `-c`.
func TestTheScriptOperandIsInsideTheBoundaryOnTheFlagRoute(t *testing.T) {
	dir := t.TempDir()
	ws := filepath.Join(dir, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "case.sh")
	if err := os.WriteFile(script, []byte("echo ran\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errs strings.Builder
	code := driver.MainArgs(gatedShell(&out, &errs),
		[]string{"testsh", "--policy", policyFile(t, dir, ws, false), script})
	if code == 0 {
		t.Errorf("status 0, want a script outside the policy refused")
	}
	if strings.Contains(out.String(), "ran") {
		t.Errorf("out = %q, want the script never read", out.String())
	}

	// The control: the same script under a policy that permits it runs.
	out.Reset()
	errs.Reset()
	if code := driver.MainArgs(gatedShell(&out, &errs),
		[]string{"testsh", "--policy", policyFile(t, t.TempDir(), ws, true), script}); code != 0 {
		t.Fatalf("allowed status %d: %s", code, errs.String())
	}
	if !strings.Contains(out.String(), "ran") {
		t.Errorf("out = %q, want the permitted script to run", out.String())
	}
}

// A policy that will not load ends the invocation, rather than leaving a
// shell that quietly runs unsandboxed. That is the failure the whole surface
// exists to prevent, and it is invisible from the inside: an unsandboxed run
// of a script that was meant to be sandboxed produces exactly the output the
// person asked for.
func TestABadPolicyStopsTheShellOnEveryBinary(t *testing.T) {
	for _, c := range []struct {
		name string
		argv []string
		want string
	}{
		{"a file that is not there", []string{"testsh", "--policy", "/nonexistent-policy", "-c", "echo RAN"}, "policy:"},
		{"no path at all", []string{"testsh", "--policy"}, "--policy requires a path"},
		{"no audit path at all", []string{"testsh", "--audit"}, "--audit requires a path"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errs strings.Builder
			if code := driver.MainArgs(gatedShell(&out, &errs), c.argv); code == 0 {
				t.Error("status 0, want the invocation refused")
			}
			if out.String() != "" {
				t.Errorf("ran %q, want nothing to run", out.String())
			}
			if !strings.Contains(errs.String(), c.want) {
				t.Errorf("stderr = %q, want it to mention %q", errs.String(), c.want)
			}
		})
	}
}

// The flags added here did not open the door to every other long option.
// `--rcfile`'s letters include a `c`, which is why an unknown long option is
// refused outright rather than read as a bundle, and that refusal has to
// survive two more names being matched before it.
func TestAnUnknownLongOptionIsStillRefused(t *testing.T) {
	var out, errs strings.Builder
	code := driver.MainArgs(gatedShell(&out, &errs), []string{"testsh", "--polic", "-c", "echo RAN"})
	if code == 0 {
		t.Error("status 0, want an unknown long option refused")
	}
	if out.String() != "" {
		t.Errorf("ran %q, want nothing to run", out.String())
	}
	if !strings.Contains(errs.String(), `unknown option "--polic"`) {
		t.Errorf("stderr = %q, want the option named", errs.String())
	}
}

// `--audit` writes the event stream, to a file or to standard error for a
// lone `-`, on every binary.
func TestTheAuditFlagOnEveryBinaryRecordsWhatHappened(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "audit.jsonl")
	var out, errs strings.Builder
	if code := driver.MainArgs(gatedShell(&out, &errs),
		[]string{"testsh", "--audit", log, "-c", "cat < /dev/null"}); code != 0 {
		t.Fatalf("status %d: %s", code, errs.String())
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"event":"command-start"`) {
		t.Errorf("audit = %q, want the command recorded", string(b))
	}

	// A lone `-` is the shell's own standard error, which is the stream a
	// trace already uses.
	out.Reset()
	errs.Reset()
	if code := driver.MainArgs(gatedShell(&out, &errs),
		[]string{"testsh", "--audit", "-", "-c", "cat < /dev/null"}); code != 0 {
		t.Fatalf("status %d: %s", code, errs.String())
	}
	if !strings.Contains(errs.String(), `"event":"command-start"`) {
		t.Errorf("stderr = %q, want the record on it", errs.String())
	}
	if strings.Contains(out.String(), `"event"`) {
		t.Errorf("out = %q, want the record kept off the shell's output stream", out.String())
	}
}

// Composition: a gate the embedder assigned and a gate the flag installed are
// both consulted, and any refusal refuses. Adding `--policy` to a shell that
// already had one must be a narrowing and never a replacement, or a front end
// embedding this package could have its boundary switched off by a flag.
func TestAFlagPolicyComposesWithAnEmbeddersGate(t *testing.T) {
	dir := t.TempDir()
	ws := filepath.Join(dir, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(ws, "f")

	var out, errs strings.Builder
	sh := gatedShell(&out, &errs)
	// Refuses the one path the policy file permits, so a run that writes it
	// proves the embedder's gate was dropped rather than composed.
	sh.Gate = &recorder{deny: func(a interp.Action) bool { return strings.HasSuffix(a.Path, "/f") }}
	code := driver.MainArgs(sh,
		[]string{"testsh", "--policy", policyFile(t, dir, ws, false), "-c", "echo x > " + inside})
	if code == 0 {
		t.Error("status 0, want the embedder's gate still consulted")
	}
	if _, err := os.Stat(inside); err == nil {
		t.Error("the file exists: the flag replaced the embedder's gate instead of composing with it")
	}
}

// The mark a gated session draws, and that it is drawn once.
//
// A shell under a policy refuses things an ungated one would do, worded as
// the failure it produces — a denied stat reads as a path that is not there —
// so without this there is no way to tell a sandboxed session from an
// ordinary one until something surprising happens.
func TestAGatedSessionSaysSoAtItsPrompt(t *testing.T) {
	allow := &recorder{}
	sh := driver.AddGate(driver.Shell{}, allow)
	if n := len(sh.PromptProviders); n != 1 {
		t.Fatalf("%d prompt providers, want one", n)
	}
	if got := sh.PromptProviders[0].Prompt(repl.PromptInfo{}); got != "(sandboxed) " {
		t.Errorf("the mark is %q, want %q", got, "(sandboxed) ")
	}
	// Not at the continuation prompt: the mark is about the session rather
	// than about the line, and repeating it would say the session became
	// sandboxed four times.
	if got := sh.PromptProviders[0].Prompt(repl.PromptInfo{Continued: true}); got != "" {
		t.Errorf("the continuation prompt is marked %q, want nothing", got)
	}
	// A second gate is a narrower session, not a second sandbox.
	if n := len(driver.AddGate(sh, &recorder{}).PromptProviders); n != 1 {
		t.Errorf("%d prompt providers after a second gate, want one", n)
	}
	// And a shell nobody pointed a policy at says nothing.
	if n := len(driver.AddGate(driver.Shell{}, nil).PromptProviders); n != 0 {
		t.Errorf("%d prompt providers with no gate, want none", n)
	}
}

// Gates intersect and Sinks fan out, which is the asymmetry the two
// composition types exist to state: two policies together allow only what
// both allow, while a trace to watch and an audit file to keep are different
// jobs and both get everything.
func TestGatesIntersectAndSinksFanOut(t *testing.T) {
	yes := &recorder{}
	no := &recorder{deny: func(interp.Action) bool { return true }}
	if got := (driver.Gates{yes, no}).Allow(context.Background(), interp.Action{}); got != interp.Deny {
		t.Errorf("a pair with one refusal decided %v, want Deny", got)
	}
	if got := (driver.Gates{yes, yes}).Allow(context.Background(), interp.Action{}); got != interp.Allow {
		t.Errorf("a pair that both allow decided %v, want Allow", got)
	}

	a, b := &recorder{}, &recorder{}
	(driver.Sinks{a, b}).Emit(context.Background(), interp.Event{})
	if a.count() != 1 || b.count() != 1 {
		t.Errorf("sinks saw %d and %d events, want one each", a.count(), b.count())
	}
}
