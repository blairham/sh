// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"strconv"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

func TestExitBuiltin(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		wantOut   string
		wantSt    int
	}{
		{"exit N stops there", `echo a; exit 3; echo b`, "a\n", 3},
		{"bare exit reports the last command", `false; exit`, "", 1},
		{"bare exit after success", `true; exit`, "", 0},
		{"a status wraps at 256", `exit 300`, "", 44},
		{"exit from a function ends the script", `f() { exit 4; }; f; echo no`, "", 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := run(t, tc.src, nil)
			if got != tc.wantOut || st != tc.wantSt {
				t.Errorf("got %q/%d, want %q/%d", got, st, tc.wantOut, tc.wantSt)
			}
		})
	}
	// A subshell's exit is its own.
	if got, st := run(t, `(exit 5); echo after=$?`, nil); got != "after=5\n" || st != 0 {
		t.Errorf("subshell: got %q/%d", got, st)
	}
}

// TestStatusArgumentIsAnOrdering is the second axis that is not a side: the
// strict policy refuses both a negative and a non-number, the numeric one
// refuses only the non-number, and the two lenient readings refuse neither —
// the leading-digit one taking the number a word begins with, and the
// arithmetic one evaluating the whole word.
//
// Asked here of `exit`. The same axis reads `return`'s operand, and
// returnoperand_test.go asks it there and asks that the two agree.
func TestStatusArgumentIsAnOrdering(t *testing.T) {
	exitSem := func(p StatusArgumentPolicy) Semantics {
		s := permissive()
		s.StatusArgument = p
		return s
	}
	for _, tc := range []struct {
		name          string
		sem           Semantics
		negSt, textSt int
	}{
		{"strict", exitSem(StatusArgStrict), 2, 2},
		{"numeric", exitSem(StatusArgNumeric), 255, 2},
		{"leading digits", exitSem(StatusArgLeadingDigits), 255, 0},
		// `abc` has no digits to read, so arithmetic makes it an unset name
		// and an unset name is 0 — the same answer the leading-digit reader
		// reaches by the other route, which is why `3abc` is the row that
		// tells the two apart and lives with `return`'s cases.
		{"arithmetic", exitSem(StatusArgArithmetic), 255, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, st := run(t, `exit -1`, withSem(tc.sem)); st != tc.negSt {
				t.Errorf("exit -1: status = %d, want %d", st, tc.negSt)
			}
			if _, st := run(t, `exit abc`, withSem(tc.sem)); st != tc.textSt {
				t.Errorf("exit abc: status = %d, want %d", st, tc.textSt)
			}
		})
	}
	// A good argument needs no answer, so the core does not refuse it.
	if _, st := run(t, `exit 3`, withSem(CoreSemantics())); st != 3 {
		t.Errorf("core, good argument: status = %d, want 3", st)
	}
	// A questionable one it must refuse rather than pick.
	out, st := run(t, `exit abc; echo reached`, withSem(CoreSemantics()))
	if st != 2 || strings.Contains(out, "reached") {
		t.Errorf("core, bad argument: %q/%d", out, st)
	}
}

func TestExitTrap(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		wantOut   string
		wantSt    int
	}{
		{"runs at the end", `trap 'echo bye' EXIT; echo hi`, "hi\nbye\n", 0},
		{"runs on an explicit exit", `trap 'echo bye' EXIT; exit 3`, "bye\n", 3},
		{"sees the last status", `trap 'echo st=$?' EXIT; false`, "st=1\n", 1},
		{"its own exit wins", `trap 'echo bye; exit 7' EXIT; exit 2`, "bye\n", 7},
		{"a second trap replaces", `trap 'echo one' EXIT; trap 'echo two' EXIT; echo body`, "body\ntwo\n", 0},
		{"trap - removes it", `trap 'echo bye' EXIT; trap - EXIT; echo hi`, "hi\n", 0},
		{"fires once, not per subshell", `trap 'echo T' EXIT; (echo sub); echo after`, "sub\nafter\nT\n", 0},
		{"nor per substitution", `trap 'echo T' EXIT; x=$(echo s); echo got=$x`, "got=s\nT\n", 0},
		{"set -e still fires it", `set -e; trap 'echo bye' EXIT; false; echo no`, "bye\n", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := run(t, tc.src, nil)
			if got != tc.wantOut || st != tc.wantSt {
				t.Errorf("got %q/%d, want %q/%d", got, st, tc.wantOut, tc.wantSt)
			}
		})
	}
}

// TestExitTrapIsFunctionLocalIsAnAxis pins the axis by name: on one side an
// EXIT trap set inside a function fires when the function returns, on the
// other it waits for the script's end. A trap set at the top level behaves
// the same on both sides, which is what makes this about the function rather
// than about traps.
func TestExitTrapIsFunctionLocalIsAnAxis(t *testing.T) {
	local := permissive()
	local.ExitTrapIsFunctionLocal = Yes
	global := permissive()
	global.ExitTrapIsFunctionLocal = No
	const src = `f() { trap 'echo TRAP' EXIT; echo enter; }; f; echo between`
	if got, _ := run(t, src, withSem(local)); got != "enter\nTRAP\nbetween\n" {
		t.Errorf("Yes: got %q", got)
	}
	if got, _ := run(t, src, withSem(global)); got != "enter\nbetween\nTRAP\n" {
		t.Errorf("No: got %q", got)
	}
	// A top-level trap is not affected by the axis.
	const top = `trap 'echo T' EXIT; f() { echo in; }; f; echo end`
	for _, sem := range []Semantics{local, global} {
		if got, _ := run(t, top, withSem(sem)); got != "in\nend\nT\n" {
			t.Errorf("top-level trap: got %q", got)
		}
	}
}

// And the status the call returns with survives the trap, which is the half a
// caller branches on. The trap's own result is discarded in both directions:
// a cleanup that fails does not spoil a call that succeeded, and one that
// succeeds does not hide a call that failed.
//
// Written against the axis rather than against a shell, and asked on the
// `Yes` side only — the `No` side never runs a trap at the return, so there
// is nothing there for a status to survive.
func TestAFunctionLocalExitTrapKeepsTheCallSStatus(t *testing.T) {
	local := permissive()
	local.ExitTrapIsFunctionLocal = Yes
	for _, tc := range []struct{ name, src, want string }{
		{
			"a failing call is still a failing call",
			`g() { trap ':' EXIT; return 2; }; g; echo "g -> $?"`,
			"g -> 2\n",
		},
		{
			"and a succeeding one is still succeeding",
			`m() { trap 'false' EXIT; return 0; }; m; echo "m -> $?"`,
			"m -> 0\n",
		},
		{
			"the body reads the call's status and the caller reads it again",
			`n() { trap 'echo "sees [$?]"' EXIT; return 7; }; n; echo "n -> $?"`,
			"sees [7]\nn -> 7\n",
		},
		{
			"a body naming a status outright keeps that one",
			`p() { trap 'exit 4' EXIT; return 3; }; p; echo unreached`,
			"",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := run(t, tc.src, withSem(local)); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTrapTakesTheSignalsNobodyCanCatch pins both halves of #2919, which are
// two facts rather than one: the word is accepted, and the action never runs.
//
// Measured 2026-09-15 on bash 5.3, bash 3.2, ksh93, zsh and dash — all five
// take `trap : KILL` at 0, take `trap : STOP` at 0, take the reset at 0, and
// print nothing at all when the signal arrives. Refusing it was this shell's
// own idea and it broke the common defensive spelling `trap cleanup HUP INT
// TERM KILL` twice over, since the reset on the way out was refused as well.
func TestTrapTakesTheSignalsNobodyCanCatch(t *testing.T) {
	for _, src := range []string{
		`trap 'x' KILL`,
		`trap 'x' STOP`,
		`trap '' KILL`,
		`trap - KILL STOP`,
		// 9 is KILL on every system this builds for, and the number has to
		// meet the same answer the name does or one spelling is a hole in
		// the other.
		`trap 'x' 9`,
	} {
		if out, st := run(t, src, nil); st != 0 || out != "" {
			t.Errorf("%s: got %q/%d, want it taken quietly", src, out, st)
		}
	}
	// And the other half. The action is a listing and never a handler: the
	// shell dies of the signal exactly as though nothing had been trapped,
	// which is 128 plus 9 and no output. This is the half a naive fix gets
	// wrong, because a signal a script aims at this shell never reaches the
	// kernel here — see selfSignaled — so the handler is the one thing that
	// *could* have run, in the one shell where it must not.
	for _, src := range []string{
		`trap 'echo caught' KILL; kill -KILL $$; echo after`,
		`trap '' KILL; kill -KILL $$; echo after`,
	} {
		if out, st := run(t, src, nil); st != 137 || out != "" {
			t.Errorf("%s: got %q/%d, want %q/137", src, out, st, "")
		}
	}
	// A word naming no signal at all is still a complaint, and one the
	// dialect words. All four report 1 for it.
	out, st := run(t, `trap 'x' NOSUCHSIGNAL`, nil)
	if st != 1 || !strings.Contains(out, "NOSUCHSIGNAL") {
		t.Errorf("a word that names nothing: got %q/%d, want 1", out, st)
	}
}

// TestSIGPrefixIsAnAxis pins the name a signal can be given.
//
// dash reads no SIG-prefixed name anywhere, so the same script traps a signal
// in three shells and reports a bad trap in the fourth.
// TestTrapTakesEverySignalTheHostKnows pins the set.
//
// It was nine names for a while, which left `trap 'x' CONT` refused as
// uncatchable in a shell where all four panel members catch it. KILL and STOP
// were the last two held back, and they are taken now as well — see
// TestTrapTakesTheSignalsNobodyCanCatch, which is where the difference that
// remains between them and these is pinned.
func TestTrapTakesEverySignalTheHostKnows(t *testing.T) {
	for _, sig := range []string{"CONT", "CHLD", "WINCH", "TSTP", "URG", "IO", "SYS", "TRAP", "XCPU", "USR1"} {
		if out, st := run(t, `trap 'x' `+sig, nil); st != 0 || out != "" {
			t.Errorf("%s: got %q/%d, want it taken quietly", sig, out, st)
		}
	}
}

func TestSIGPrefixIsAnAxis(t *testing.T) {
	// The status is echoed rather than taken from the script: a bad trap is
	// not fatal in any of the four, so the script carries on and its own exit
	// status is the echo's.
	const src = `trap 'echo caught' SIGUSR1; echo "st=$?"`
	for _, tc := range []struct {
		name string
		sem  Semantics
		want string
	}{
		{"accepted", sigPrefixSem(Yes), "st=0\n"},
		{"refused", sigPrefixSem(No), "sh: trap: SIGUSR1: bad trap\nst=1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, src, withSem(tc.sem))
			if out != tc.want || st != 0 {
				t.Errorf("got %q/%d, want %q/0", out, st, tc.want)
			}
		})
	}

	// Unspecified is refused rather than guessed, and only where the prefix
	// decides something: a bare name never asks, and neither does a word that
	// names no signal with the prefix taken off.
	sem := sigPrefixSem(Unspecified)
	if _, _ = run(t, `trap 'x' SIGUSR1`, withSem(sem)); true {
		out, _ := run(t, `trap 'x' SIGUSR1`, withSem(sem))
		if !strings.Contains(out, "no dialect was chosen") {
			t.Errorf("SIGUSR1: got %q, want a refusal naming the axis", out)
		}
	}
	if out, st := run(t, `trap 'x' USR1; echo ok`, withSem(sem)); out != "ok\n" || st != 0 {
		t.Errorf("a bare name: got %q/%d, want no question asked", out, st)
	}
	if out, _ := run(t, `trap 'x' SIGNOPE`, withSem(sem)); strings.Contains(out, "no dialect was chosen") {
		t.Errorf("SIGNOPE: got %q, want a bad trap rather than a question", out)
	}
}

// sigPrefixSem answers the SIGPrefixAccepted axis by name on the permissive
// base, so the test names the axis rather than a shell.
func sigPrefixSem(a Answer) Semantics {
	s := permissive()
	s.SIGPrefixAccepted = a
	return s
}

// Signal *delivery* is deliberately not tested in this process.
//
// run() executes in-process, so `kill -INT $$` would send a real signal to the
// test binary and `trap '' INT` would call signal.Ignore for the whole of it —
// process-global state that outlives the test that set it. The behavior is
// covered by the corpus instead, which runs the built shell as its own
// process against all four panel shells: see the trap/ cases in
// internal/oracle/case.go.

func TestProcessIDParameter(t *testing.T) {
	// `$$` is the shell's own process id, and the same inside a subshell:
	// POSIX says the *invoking* shell's, which is what makes it usable as a
	// lock name.
	got, _ := run(t, `echo "[$$]"`, nil)
	if got != "["+itoaForTest(os.Getpid())+"]\n" {
		t.Errorf("got %q, want the process id", got)
	}
	inner, _ := run(t, `echo "$$"`, nil)
	sub, _ := run(t, `(echo "$$")`, nil)
	if inner != sub {
		t.Errorf("a subshell should report the same id: %q vs %q", inner, sub)
	}
}

func itoaForTest(n int) string { return strconv.Itoa(n) }

// A bare `return` in a trap action's own body — Semantics.TrapReturnStatus.
//
// Four readings, and this asserts each is reachable and that they are four.
// Which value each preset picks is in dialect/trapreturn_test.go against the
// panel's own bytes.
//
// The action's last command reports 123 and the status the handler is entered
// at is 4, so every reading answers a different number: 4, 123, 0, and the
// refusal an unanswered axis gives.
func TestABareReturnInATrapActionHasFourReadings(t *testing.T) {
	const src = "f() { return $1; }\n" +
		"h() { ( /bin/sleep 0.1; kill -USR1 $$ ) & ( /bin/sleep 0.5; exit 4 ); return 7; }\n" +
		"trap 'f 123; return' USR1\n" +
		"h\n" +
		"printf 'exit %s' \"$?\"\n"
	for _, c := range []struct {
		name    string
		reading TrapReturnReading
		want    string
	}{
		{"the status before it", TrapReturnTakesTheStatusBeforeIt, "exit 4"},
		{"the handler's last", TrapReturnTakesTheHandlersLastStatus, "exit 123"},
		{"zero", TrapReturnIsZero, "exit 0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := permissive()
			// The probe backgrounds a job inside a function, which asks an
			// axis of its own; pinned so the refusal for that one is not
			// what this test reads.
			sem.SubshellJobTable = SubshellJobsCleared
			sem.TrapReturnStatus = c.reading
			out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if out != c.want {
				t.Errorf("wrote %q, want %q", out, c.want)
			}
		})
	}
}

// And a `return` inside a function the action *called* is that function's,
// read the ordinary way — which is what keeps this an axis about the action
// rather than about a depth. The same vector that answers 4 for the action's
// own `return` leaves the interrupted function's own 7 standing here, because
// the inner `return` ends only the inner function and the action then runs to
// its end normally.
func TestAReturnInsideAFunctionTheActionCalledIsThatFunctions(t *testing.T) {
	const src = "inner() { return; }\n" +
		"h() { kill -USR1 $$; printf 'resumed '; return 7; }\n" +
		"trap 'inner' USR1\n" +
		"h\n" +
		"printf 'exit %s' \"$?\"\n"
	sem := permissive()
	sem.TrapReturnStatus = TrapReturnTakesTheStatusBeforeIt
	out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
	if want := "resumed exit 7"; out != want {
		t.Errorf("wrote %q, want %q", out, want)
	}
}
