// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
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
			got, st := run(t, tc.src, withSem(bash.Semantics()))
			if got != tc.wantOut || st != tc.wantSt {
				t.Errorf("got %q/%d, want %q/%d", got, st, tc.wantOut, tc.wantSt)
			}
		})
	}
	// A subshell's exit is its own.
	if got, st := run(t, `(exit 5); echo after=$?`, withSem(bash.Semantics())); got != "after=5\n" || st != 0 {
		t.Errorf("subshell: got %q/%d", got, st)
	}
}

// TestExitArgumentIsAnOrdering is the second axis that is not a side: dash
// refuses both a negative and a non-number, bash refuses only the non-number,
// and ksh93 and zsh take either.
func TestExitArgumentIsAnOrdering(t *testing.T) {
	for _, tc := range []struct {
		name          string
		sem           Semantics
		negSt, textSt int
	}{
		{"dash", dash.Semantics(), 2, 2},
		{"bash", bash.Semantics(), 255, 2},
		{"ksh93", ksh.Semantics(), 255, 0},
		{"zsh", zsh.Semantics(), 255, 0},
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
			got, st := run(t, tc.src, withSem(bash.Semantics()))
			if got != tc.wantOut || st != tc.wantSt {
				t.Errorf("got %q/%d, want %q/%d", got, st, tc.wantOut, tc.wantSt)
			}
		})
	}
}

// TestTrapInAFunctionIsAnAxis is zsh's alone: there a trap set inside a
// function fires when the function returns. A trap set at the top level
// behaves the same everywhere, which is what makes this about the function
// rather than about traps.
func TestTrapInAFunctionIsAnAxis(t *testing.T) {
	const src = `f() { trap 'echo TRAP' EXIT; echo enter; }; f; echo between`
	if got, _ := run(t, src, withSem(zsh.Semantics())); got != "enter\nTRAP\nbetween\n" {
		t.Errorf("zsh: got %q", got)
	}
	for _, tc := range []struct {
		name string
		sem  Semantics
	}{{"dash", dash.Semantics()}, {"bash", bash.Semantics()}, {"ksh93", ksh.Semantics()}} {
		if got, _ := run(t, src, withSem(tc.sem)); got != "enter\nbetween\nTRAP\n" {
			t.Errorf("%s: got %q", tc.name, got)
		}
	}
	// A top-level trap is not affected by the axis.
	const top = `trap 'echo T' EXIT; f() { echo in; }; f; echo end`
	for _, sem := range []Semantics{zsh.Semantics(), dash.Semantics()} {
		if got, _ := run(t, top, withSem(sem)); got != "in\nend\nT\n" {
			t.Errorf("top-level trap: got %q", got)
		}
	}
}

func TestTrapRefusesSignalsItCannotCatch(t *testing.T) {
	// KILL and STOP cannot be caught by anyone, so accepting them would be
	// promising something the kernel will not allow.
	for _, sig := range []string{"KILL", "STOP", "NOSUCHSIGNAL"} {
		out, st := run(t, `trap 'x' `+sig, withSem(bash.Semantics()))
		if st == 0 || !strings.Contains(out, "not a signal this shell can catch") {
			t.Errorf("%s: got %q/%d", sig, out, st)
		}
	}
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
	got, _ := run(t, `echo "[$$]"`, withSem(bash.Semantics()))
	if got != "["+itoaForTest(os.Getpid())+"]\n" {
		t.Errorf("got %q, want the process id", got)
	}
	inner, _ := run(t, `echo "$$"`, withSem(bash.Semantics()))
	sub, _ := run(t, `(echo "$$")`, withSem(bash.Semantics()))
	if inner != sub {
		t.Errorf("a subshell should report the same id: %q vs %q", inner, sub)
	}
}

func itoaForTest(n int) string { return strconv.Itoa(n) }
