// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// These tests never let a signal actually reach the test binary, and that is
// not caution — it is the behavior. DieBySignal is nil here, as it is for any
// Runner that is not a shell, so a script that kills the shell stops the
// script and leaves the process alone. A test that had to guard against being
// killed would be describing a library that could kill its embedder.

// killRun runs src and returns both streams and the status.
func killRun(t *testing.T, src string, sem Semantics, dg Diagnostics) (out, errs string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	r := &Runner{Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &dg, Name: "testsh"}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return o.String(), e.String(), st
}

func killSem() Semantics {
	s := CoreSemantics()
	s.KillStatus = KillStatusAnyFailure
	s.KillListAcceptsName = Yes
	s.ExitTrapRunsOnSignalDeath = Yes
	return s
}

// TestSignalToSelfRunsTheHandlerBeforeTheNextCommand is the whole point of the
// builtin.
//
// kill(2) delivers before it returns, so by the time the builtin is done the
// arrival is a fact — and the handler must run before the next command rather
// than whenever a goroutine in os/signal gets round to mentioning it. Run
// enough times that a scheduler-dependent answer would show itself: this
// failed 300 times out of 300 while the recording was mutated out.
func TestSignalToSelfRunsTheHandlerBeforeTheNextCommand(t *testing.T) {
	for i := 0; i < 200; i++ {
		out, _, st := killRun(t, "trap 'echo caught' USR1\nkill -USR1 $$\necho after\n",
			killSem(), Diagnostics{})
		if out != "caught\nafter\n" {
			t.Fatalf("run %d: got %q, want the handler before the next command", i, out)
		}
		if st != 0 {
			t.Fatalf("run %d: status %d, want 0", i, st)
		}
	}
}

// TestSignalToSelfRunsTheHandlerOnce pins the other half of the recording: the
// runtime's own copy arrives later and must not run the handler a second time.
func TestSignalToSelfRunsTheHandlerOnce(t *testing.T) {
	for i := 0; i < 50; i++ {
		// The sleep gives os/signal every chance to deliver its copy, which is
		// what would double the output if the echo were not discarded.
		out, _, _ := killRun(t, "trap 'echo caught' USR2\nkill -USR2 $$\nsleep 0.05\necho after\n",
			killSem(), Diagnostics{})
		if out != "caught\nafter\n" {
			t.Fatalf("run %d: got %q, want one handler run", i, out)
		}
	}
}

func TestKillTargetsAndStatuses(t *testing.T) {
	tests := []struct {
		name string
		src  string
		out  string
		st   int
	}{
		{"a live process is signaled", `kill -0 $$; echo "st=$?"`, "st=0\n", 0},
		{"signal zero is a probe", `kill -s 0 $$; echo "st=$?"`, "st=0\n", 0},
		{"a dead process is 1", `kill 999999 2>/dev/null; echo "st=$?"`, "st=1\n", 0},
		{"a name is not a pid", `kill abc 2>/dev/null; echo "st=$?"`, "st=1\n", 0},
		{"no operands is usage", `kill 2>/dev/null; echo "st=$?"`, "st=2\n", 0},
		{"an unknown signal", `kill -Q 1 2>/dev/null; echo "st=$?"`, "st=1\n", 0},
		{"a missing -s argument", `kill -s 2>/dev/null; echo "st=$?"`, "st=1\n", 0},
		{"-l translates a number", `kill -l 9`, "KILL\n", 0},
		{"-l translates a name", `kill -l INT`, "2\n", 0},
		{"the SIG prefix is accepted", `kill -s SIGCONT $$; echo "st=$?"`, "st=0\n", 0},
		{"a lowercase name is accepted", `kill -cont $$; echo "st=$?"`, "st=0\n", 0},
		{"a number names a signal", `kill -19 $$; echo "st=$?"`, "st=0\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _, st := killRun(t, tt.src, killSem(), Diagnostics{})
			if out != tt.out {
				t.Errorf("output: got %q, want %q", out, tt.out)
			}
			if st != tt.st {
				t.Errorf("status: got %d, want %d", st, tt.st)
			}
		})
	}
}

// TestKillStatusPolicy pins the three answers to one question. Only reachable
// with more than one target: a single failure is 1 in every shell in the
// panel, which is why the axis is not consulted for one.
func TestKillStatusPolicy(t *testing.T) {
	tests := []struct {
		name          string
		policy        KillStatusPolicy
		someWant      int
		allWant       int
		oneAloneWants int
	}{
		{"any failure", KillStatusAnyFailure, 1, 1, 1},
		{"any success", KillStatusAnySuccess, 0, 1, 1},
		{"failure count", KillStatusFailureCount, 1, 2, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sem := killSem()
			sem.KillStatus = tt.policy
			for _, c := range []struct {
				src  string
				want int
			}{
				{`kill -0 $$ 999999 2>/dev/null; echo "st=$?"`, tt.someWant},
				{`kill 999998 999999 2>/dev/null; echo "st=$?"`, tt.allWant},
				{`kill 999999 2>/dev/null; echo "st=$?"`, tt.oneAloneWants},
			} {
				out, _, _ := killRun(t, c.src, sem, Diagnostics{})
				if got := strings.TrimSpace(out); got != "st="+itoa(c.want) {
					t.Errorf("%s: got %q, want st=%d", c.src, got, c.want)
				}
			}
		})
	}
}

// TestKillStatusPolicyRefusesWhenUnanswered checks the axis is refused rather
// than guessed — and that a single target still needs no answer.
func TestKillStatusPolicyRefusesWhenUnanswered(t *testing.T) {
	sem := killSem()
	sem.KillStatus = KillStatusUnspecified

	_, errs, _ := killRun(t, `kill -0 $$ 999999`, sem, Diagnostics{})
	if !strings.Contains(errs, "no dialect was chosen") {
		t.Errorf("two targets: got %q, want a refusal naming the axis", errs)
	}

	_, errs, _ = killRun(t, `kill 999999`, sem, Diagnostics{})
	if strings.Contains(errs, "no dialect was chosen") {
		t.Errorf("one target: got %q, want no refusal — the panel agrees here", errs)
	}
}

// TestKillListAcceptsNameRefusesWhenUnanswered does the same for `kill -l`,
// which is a question only for a name: a number needs no dialect.
func TestKillListAcceptsNameRefusesWhenUnanswered(t *testing.T) {
	sem := killSem()
	sem.KillListAcceptsName = Unspecified

	out, errs, _ := killRun(t, `kill -l 9`, sem, Diagnostics{})
	if out != "KILL\n" || strings.Contains(errs, "no dialect was chosen") {
		t.Errorf("a number: got out=%q errs=%q, want KILL and no refusal", out, errs)
	}

	_, errs, _ = killRun(t, `kill -l INT`, sem, Diagnostics{})
	if !strings.Contains(errs, "no dialect was chosen") {
		t.Errorf("a name: got %q, want a refusal naming the axis", errs)
	}
}

// TestFatalSignalStopsTheScriptWithoutKillingTheProcess is the library half of
// the death. The test binary is still running, which is the assertion.
func TestFatalSignalStopsTheScriptWithoutKillingTheProcess(t *testing.T) {
	out, _, st := killRun(t, "echo before\nkill -INT $$\necho after\n", killSem(), Diagnostics{})
	if out != "before\n" {
		t.Errorf("got %q, want nothing after the signal", out)
	}
	if want := 128 + int(syscall.SIGINT); st != want {
		t.Errorf("status: got %d, want %d", st, want)
	}
}

// TestIgnoredSignalNeitherRunsNorKills is the third disposition, and the one
// that would be easy to fold into the wrong branch: an ignored signal is
// trapped with an empty body, so it must not run a handler and must not stop
// the script either.
func TestIgnoredSignalNeitherRunsNorKills(t *testing.T) {
	out, _, st := killRun(t, "trap '' INT\nkill -INT $$\necho after\n", killSem(), Diagnostics{})
	if out != "after\n" {
		t.Errorf("got %q, want the script to carry on", out)
	}
	if st != 0 {
		t.Errorf("status: got %d, want 0", st)
	}
}

// TestExitTrapAfterAFatalSignalIsAnAxis pins both sides of the two-two split.
func TestExitTrapAfterAFatalSignalIsAnAxis(t *testing.T) {
	src := "trap 'echo bye' EXIT\nkill -INT $$\necho after\n"
	for _, tt := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"runs the trap", Yes, "bye\n"},
		{"does not", No, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sem := killSem()
			sem.ExitTrapRunsOnSignalDeath = tt.answer
			out, _, st := killRun(t, src, sem, Diagnostics{})
			if out != tt.want {
				t.Errorf("got %q, want %q", out, tt.want)
			}
			if want := 128 + int(syscall.SIGINT); st != want {
				t.Errorf("status: got %d, want %d", st, want)
			}
		})
	}
}

// TestDieBySignalIsAskedLast checks the order: the script stops, the EXIT trap
// runs, and only then is the process asked to end.
func TestDieBySignalIsAskedLast(t *testing.T) {
	f, err := syntax.Parse("trap 'echo bye' EXIT\nkill -TERM $$\necho after\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var o bytes.Buffer
	var got syscall.Signal
	asked := 0
	sem := killSem()
	r := &Runner{
		Stdout: &o, Semantics: &sem, Name: "testsh",
		DieBySignal: func(sig syscall.Signal) error {
			got, asked = sig, asked+1
			// A real one does not return; this one does, so the test can
			// assert on what happened before it was called.
			return nil
		},
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if asked != 1 {
		t.Errorf("asked to die %d times, want once", asked)
	}
	if got != syscall.SIGTERM {
		t.Errorf("died by %v, want SIGTERM", got)
	}
	if o.String() != "bye\n" {
		t.Errorf("output %q, want the EXIT trap to have run first", o.String())
	}
}

// TestNoDeathWithoutAHandlerToDoIt is the default a library gets.
func TestNoDeathWithoutAHandlerToDoIt(t *testing.T) {
	sem := killSem()
	_, _, st := killRun(t, "kill -TERM $$\necho after\n", sem, Diagnostics{})
	if want := 128 + int(syscall.SIGTERM); st != want {
		t.Errorf("status: got %d, want %d", st, want)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
