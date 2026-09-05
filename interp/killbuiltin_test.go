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
	r := newTestRunner(t, &Runner{Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
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
	s.TrapBodyRunsWhatParsed = Yes
	s.SIGPrefixAccepted = Yes
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
		// 15 rather than a harmless-looking number, because harmless is not
		// portable: 19 is CONT on a BSD and STOP on Linux, so the first
		// version of this line suspended the test binary on one platform and
		// hung there until CI killed it eleven minutes later. The numbers
		// that agree everywhere are all fatal, so this one is trapped.
		{
			"a number names a signal",
			"trap 'echo caught' TERM\nkill -15 $$\necho \"st=$?\"\n",
			"caught\nst=0\n", 0,
		},
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
	r := newTestRunner(t, &Runner{
		Stdout: &o, Semantics: &sem, Name: "testsh",
		DieBySignal: func(sig syscall.Signal) error {
			got, asked = sig, asked+1
			// A real one does not return; this one does, so the test can
			// assert on what happened before it was called.
			return nil
		},
	})
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

// TestAnIgnoredFatalSignalIsNotADeath covers the axis for a signal one side of
// the panel takes the default action away from.
//
// Three questions in one, because the axis is only the middle of them: a shell
// that answers "it still ends me" is killed as any fatal signal kills it, one
// that answers "it does not" carries on with a status of 0, and an interactive
// shell carries on whatever it answers — that last part is unanimous, so the
// axis is not consulted there at all.
func TestAnIgnoredFatalSignalIsNotADeath(t *testing.T) {
	const src = "kill -QUIT $$\necho after\n"
	for _, c := range []struct {
		name        string
		answer      Answer
		interactive bool
		wantOut     string
		wantStatus  int
	}{
		{"ends the shell", No, false, "", 128 + int(syscall.SIGQUIT)},
		{"does nothing", Yes, false, "after\n", 0},
		{"never ends an interactive shell", No, true, "after\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := syntax.Parse(src, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			var o, e bytes.Buffer
			sem := killSem()
			sem.QuitIgnoredWhenNotInteractive = c.answer
			r := newTestRunner(t, &Runner{
				Stdout: &o, Stderr: &e, Semantics: &sem, Name: "testsh",
				Interactive: c.interactive,
			})
			st, rerr := r.Run(context.Background(), f)
			if rerr != nil {
				t.Fatal(rerr)
			}
			if o.String() != c.wantOut {
				t.Errorf("output %q, want %q", o.String(), c.wantOut)
			}
			if st != c.wantStatus {
				t.Errorf("status %d, want %d", st, c.wantStatus)
			}
			if e.String() != "" {
				t.Errorf("diagnostic %q, want none", e.String())
			}
		})
	}
}

// TestAnOrderlyEndingIsNotADeath covers the axis for the other fatal signal
// the panel splits on, and the split is about the *kind* of ending rather than
// about a number: one side is killed by the signal and reports 128 plus it,
// the other stops as though it had run `exit 1`.
func TestAnOrderlyEndingIsNotADeath(t *testing.T) {
	const src = "kill -HUP $$\necho after\n"
	for _, c := range []struct {
		name       string
		answer     Answer
		wantStatus int
	}{
		{"killed by the signal", No, 128 + int(syscall.SIGHUP)},
		{"exits instead", Yes, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := killSem()
			sem.HangupIsAnOrderlyExit = c.answer
			out, errs, st := killRun(t, src, sem, Diagnostics{})
			if out != "" {
				t.Errorf("output %q, want the script to have stopped", out)
			}
			if errs != "" {
				t.Errorf("diagnostic %q, want none", errs)
			}
			if st != c.wantStatus {
				t.Errorf("status %d, want %d", st, c.wantStatus)
			}
		})
	}
}

// An ending that is not a death has nothing to re-raise, which is the half of
// the axis a status cannot show: the same script asks to die on one answer and
// does not on the other.
func TestAnOrderlyEndingRaisesNothing(t *testing.T) {
	for _, c := range []struct {
		name      string
		answer    Answer
		wantAsked int
	}{
		{"a death is raised", No, 1},
		{"an exit is not", Yes, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := syntax.Parse("kill -HUP $$\necho after\n", syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			asked := 0
			sem := killSem()
			sem.HangupIsAnOrderlyExit = c.answer
			var o bytes.Buffer
			r := newTestRunner(t, &Runner{
				Stdout: &o, Semantics: &sem, Name: "testsh",
				DieBySignal: func(syscall.Signal) error { asked++; return nil },
			})
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if asked != c.wantAsked {
				t.Errorf("asked to die %d times, want %d", asked, c.wantAsked)
			}
			if o.String() != "" {
				t.Errorf("output %q, want the script to have stopped", o.String())
			}
		})
	}
}

// The EXIT trap tells the two endings apart where the status cannot, because
// a shell that answers "no" to ExitTrapRunsOnSignalDeath still runs it after
// an ending that was never a death. Both axes are set against each other here
// on purpose: that combination is the only evidence that this one is about the
// kind of ending rather than about a number.
func TestAnOrderlyEndingRunsTheExitTrapAnyway(t *testing.T) {
	const src = "trap 'echo bye' EXIT\nkill -HUP $$\necho after\n"
	for _, c := range []struct {
		name       string
		orderly    Answer
		wantOut    string
		wantStatus int
	}{
		{"a death does not reach it", No, "", 128 + int(syscall.SIGHUP)},
		{"an exit does", Yes, "bye\n", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := killSem()
			sem.ExitTrapRunsOnSignalDeath = No
			sem.HangupIsAnOrderlyExit = c.orderly
			out, _, st := killRun(t, src, sem, Diagnostics{})
			if out != c.wantOut {
				t.Errorf("output %q, want %q", out, c.wantOut)
			}
			if st != c.wantStatus {
				t.Errorf("status %d, want %d", st, c.wantStatus)
			}
		})
	}
}

// An `exit` in the EXIT trap takes the status, which it could not do if the
// ending were a death carrying a number of its own.
func TestAnOrderlyEndingLetsTheExitTrapChooseTheStatus(t *testing.T) {
	sem := killSem()
	sem.HangupIsAnOrderlyExit = Yes
	_, _, st := killRun(t, "trap 'exit 5' EXIT\nkill -HUP $$\n", sem, Diagnostics{})
	if st != 5 {
		t.Errorf("status %d, want the trap's 5", st)
	}
}

// The status is a constant rather than anything the script had reached, which
// is what says it is this ending's own number and not a status carried over.
func TestAnOrderlyEndingIgnoresTheEarlierStatus(t *testing.T) {
	sem := killSem()
	sem.HangupIsAnOrderlyExit = Yes
	_, _, st := killRun(t, "false\nkill -HUP $$\n", sem, Diagnostics{})
	if st != 1 {
		t.Errorf("status %d, want 1", st)
	}
	_, _, st = killRun(t, "(exit 7)\nkill -HUP $$\n", sem, Diagnostics{})
	if st != 1 {
		t.Errorf("status %d, want 1", st)
	}
}

// A subshell raising it is the same ending, arriving by the other route: the
// subshell finishes, the parent does not carry on, and the status is the
// orderly one rather than a death's.
func TestAnOrderlyEndingFromASubshellEndsTheParent(t *testing.T) {
	sem := killSem()
	sem.HangupIsAnOrderlyExit = Yes
	out, _, st := killRun(t, "(kill -HUP $$; echo inner)\necho outer\n", sem, Diagnostics{})
	if out != "inner\n" {
		t.Errorf("output %q, want the subshell to finish and the parent to stop", out)
	}
	if st != 1 {
		t.Errorf("status %d, want 1", st)
	}
}

// A trapped hangup never reaches the axis, so the answer cannot turn a handled
// signal into an ending.
func TestATrappedHangupDoesNotReachTheOrderlyAxis(t *testing.T) {
	sem := killSem()
	sem.HangupIsAnOrderlyExit = Unspecified
	// Answered because running a handler at all asks it, and an unanswered
	// axis writes a diagnostic this test reads as the failure it is looking
	// for. Which way it is answered does not matter here.
	sem.SignalHandlerSeesEarlierStatus = No
	out, errs, st := killRun(t, "trap 'echo caught' HUP\nkill -HUP $$\necho after\n", sem, Diagnostics{})
	if out != "caught\nafter\n" {
		t.Errorf("output %q, want the handler and then the script", out)
	}
	if errs != "" {
		t.Errorf("diagnostic %q, want none — the axis was not reached", errs)
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}

// And a signal nobody ignores never reaches the axis, so leaving it unanswered
// is not a way to break every other death.
func TestAnUnansweredIgnoreAxisDoesNotReachOtherSignals(t *testing.T) {
	sem := killSem()
	sem.QuitIgnoredWhenNotInteractive = Unspecified
	out, errs, st := killRun(t, "kill -TERM $$\necho after\n", sem, Diagnostics{})
	if out != "" || errs != "" {
		t.Errorf("output %q and %q, want neither", out, errs)
	}
	if want := 128 + int(syscall.SIGTERM); st != want {
		t.Errorf("status %d, want %d", st, want)
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
