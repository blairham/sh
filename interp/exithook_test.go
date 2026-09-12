// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Semantics.ExitHook: the function the shell calls on the way out.
//
// The axis is named here and no shell is. What a shell answers, and the
// measurements behind the answer, live in the dialect — see dialect/zsh's
// hookstyle_test.go, which is the one column of the panel that has one.
//
// Every hook prints a marker nothing else in the snippet can print, for the
// reason directoryhook_test.go gives: a case asserting that a hook did *not*
// run has to be unable to pass on somebody else's output.

// exitHookRunner is a runner whose dialect has an exit hook and a hook list.
//
// The suffix is `_list` and the hook is `leaving`, which are nobody's
// spelling. A test in this package names the axis; the spelling is the
// dialect's.
func exitHookRunner(t *testing.T, out *strings.Builder) *Runner {
	t.Helper()
	sem := PosixSemantics()
	sem.ExitHook = "leaving"
	sem.HookListSuffix = "_list"
	return newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: t.TempDir(),
		Stdout: out, Stderr: out,
	})
}

// runToTheEnd runs a snippet the way a script ends: through Run, which is what
// calls Finish, which is where the hook fires. The status is the shell's.
func runToTheEnd(t *testing.T, r *Runner, src string) int {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	status, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

// The named function, then the list, in the order the list holds it — and each
// of them is told the status the shell is leaving with.
func TestTheExitHookFiresWithTheStatusTheShellIsLeavingWith(t *testing.T) {
	var out strings.Builder
	r := exitHookRunner(t, &out)
	status := runToTheEnd(t, r, `leaving() { echo "EXITMARK-NAMED st=$? n=$#"; }
a() { echo "EXITMARK-A st=$?"; }
b() { echo "EXITMARK-B st=$?"; }
leaving_list=(a b)
exit 4
`)
	want := "EXITMARK-NAMED st=4 n=0\nEXITMARK-A st=4\nEXITMARK-B st=4\n"
	if out.String() != want {
		t.Errorf("the exit hook wrote %q, want %q", out.String(), want)
	}
	if status != 4 {
		t.Errorf("the shell left with %d, want 4", status)
	}
}

// It fires after the EXIT trap, which is the order measured: the trap is the
// script's own last word and the hook is the shell's.
func TestTheExitHookFiresAfterTheExitTrap(t *testing.T) {
	var out strings.Builder
	r := exitHookRunner(t, &out)
	runToTheEnd(t, r, `trap 'echo EXITMARK-TRAP' EXIT
leaving() { echo EXITMARK-HOOK; }
exit 3
`)
	want := "EXITMARK-TRAP\nEXITMARK-HOOK\n"
	if out.String() != want {
		t.Errorf("the two endings wrote %q, want %q", out.String(), want)
	}
}

// A hook that merely fails changes nothing: not the status the shell leaves
// with, and not the status the item after it is told.
func TestAFailingExitHookChangesNothing(t *testing.T) {
	var out strings.Builder
	r := exitHookRunner(t, &out)
	status := runToTheEnd(t, r, `leaving() { echo "EXITMARK-NAMED st=$?"; return 5; }
a() { echo "EXITMARK-A st=$?"; false; }
leaving_list=(a)
exit 4
`)
	want := "EXITMARK-NAMED st=4\nEXITMARK-A st=4\n"
	if out.String() != want {
		t.Errorf("the exit hook wrote %q, want %q", out.String(), want)
	}
	if status != 4 {
		t.Errorf("the shell left with %d, want 4", status)
	}
}

// An `exit` inside one, though, is the one thing that does — and it stops
// nothing.
//
// This is the single rule the exit chain does not share with every other hook
// chain here. Everywhere else an item that exited ends the chain, because what
// it ended is the session; at this site the session is already over, so `exit`
// has nothing left to end and only records a status. The last one wins.
func TestAnExitInsideTheExitHookWinsAndStopsNothing(t *testing.T) {
	var out strings.Builder
	r := exitHookRunner(t, &out)
	status := runToTheEnd(t, r, `leaving() { echo EXITMARK-NAMED; exit 9; }
a() { echo EXITMARK-A; exit 11; }
b() { echo EXITMARK-B; }
leaving_list=(a b)
exit 4
`)
	want := "EXITMARK-NAMED\nEXITMARK-A\nEXITMARK-B\n"
	if out.String() != want {
		t.Errorf("the exit hook wrote %q, want %q", out.String(), want)
	}
	if status != 11 {
		t.Errorf("the shell left with %d, want 11", status)
	}
}

// A shell whose dialect names no exit hook runs nothing and says nothing, and
// that is three of the four.
func TestAShellWithNoExitHookRunsNothing(t *testing.T) {
	var out strings.Builder
	sem := PosixSemantics()
	sem.HookListSuffix = "_list"
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: t.TempDir(),
		Stdout: &out, Stderr: &out,
	})
	status := runToTheEnd(t, r, `leaving() { echo EXITMARK-NAMED; }
exit 4
`)
	if out.String() != "" {
		t.Errorf("a shell with no exit hook wrote %q", out.String())
	}
	if status != 4 {
		t.Errorf("the shell left with %d, want 4", status)
	}
}

// And a run that was never told to exit leaves the shell looking as though it
// had not.
//
// The flag the hook clears around each call is the same one a front end reads
// afterwards to decide whether the session is over — the driver's `if
// r.Exited()` after the startup files is one such reader. Firing the hook must
// not set it: a prelude installed on a runner goes through this same site, and
// a shell that came out of it already exited never ran its first command.
// That is what shipped for the length of one build. #2111.
func TestTheExitHookLeavesTheControlFlagAsItFoundIt(t *testing.T) {
	var out strings.Builder
	r := exitHookRunner(t, &out)
	runToTheEnd(t, r, `leaving() { echo EXITMARK-NAMED; }
echo body
`)
	if out.String() != "body\nEXITMARK-NAMED\n" {
		t.Errorf("the run wrote %q", out.String())
	}
	if r.Exited() {
		t.Error("a run that was never told to exit came out of the exit hook exited")
	}
}
