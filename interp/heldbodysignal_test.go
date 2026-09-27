// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"syscall"
	"testing"
)

// A forked body holds what it aimed at itself on a list of its own, and the
// list goes with the body.
//
// The half of Semantics.SelfAimedWindowChangeWaitsForInputOrAChild that the
// front end's loop cannot reach: a body reads no input — its text was parsed
// before it began — so the only moment it can arrive at is a child it waits
// for. Measured 2026-09-26 on zsh 5.9.2 under `-f` over a script file, with
// `zmodload zsh/system` for the parameter that names the body:
//
//	( trap 'print S' WINCH; kill -WINCH <own pid>; print a )       a
//	( … ; kill -WINCH <own pid>; /bin/echo x; print a )            x S a
//
// So the first runs the handler not at all. It is asserted on the lists
// rather than through a run because the number a body is signaled by comes
// from a placeholder process, and what this row is about is which list the
// arrival lands on.
func heldBodyRunner(t *testing.T, a Answer) *Runner {
	t.Helper()
	sem := PosixSemantics()
	sem.SelfAimedWindowChangeWaitsForInputOrAChild = a
	return newTestRunner(t, &Runner{
		Semantics:  &sem,
		inSubshell: true,
		traps:      map[string]string{"WINCH": "echo S"},
		bodyAnchor: &procAnchor{tried: true, pgid: 4242},
	})
}

func TestABodysHeldSignalIsNotOnTheListItsCommandsDrain(t *testing.T) {
	r := heldBodyRunner(t, Yes)
	if err := r.signalThisBody("WINCH", syscall.SIGWINCH); err != nil {
		t.Fatal(err)
	}
	if len(r.selfPending) != 0 {
		t.Errorf("selfPending = %v, want nothing to run between the body's commands", r.selfPending)
	}
	if len(r.selfHeldForInput) != 1 {
		t.Fatalf("selfHeldForInput = %v, want the one arrival held", r.selfHeldForInput)
	}
	// And the moment it waits for: a child this body waited out hands it over.
	r.releaseSignalsHeldForInput()
	if len(r.selfHeldForInput) != 0 || len(r.selfPending) != 1 {
		t.Errorf("after a child: held %v pending %v, want it handed over",
			r.selfHeldForInput, r.selfPending)
	}
}

// The control: under the other answer it goes straight onto the list the
// body's commands drain, which is what every other signal does in every
// column.
func TestABodysSignalIsOtherwiseRunBetweenItsCommands(t *testing.T) {
	r := heldBodyRunner(t, No)
	if err := r.signalThisBody("WINCH", syscall.SIGWINCH); err != nil {
		t.Fatal(err)
	}
	if len(r.selfHeldForInput) != 0 || len(r.selfPending) != 1 {
		t.Errorf("held %v pending %v, want it pending", r.selfHeldForInput, r.selfPending)
	}
}
