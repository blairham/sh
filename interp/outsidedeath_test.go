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

// offered is what one `offer` builtin saw when it offered the shell a death.
type offered struct {
	taken, again bool
}

// offerADeath runs src in a Runner whose front end has asked to hand it a
// death, with an `offer` builtin that does the handing: it offers sig the way
// the front end's watch would, offers it a second time, and returns 3. With
// revoke set it takes the death straight back, as a watch that will not wait
// any longer does. listened counts the calls to ListenForFatalSignals.
func offerADeath(
	t *testing.T,
	src string,
	sem Semantics,
	sig syscall.Signal,
	interactive, revoke bool,
) (out string, status int, got offered, listened int, claimed bool) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &o, Stderr: &e, Semantics: &sem, Name: "testsh"})
	r.Interactive = interactive
	r.ListenForFatalSignals = func() { listened++ }
	takes := r.TakesAFatalSignal()
	var gone <-chan struct{}
	r.Register("offer", func(*Runner, context.Context, []string) int {
		c, back, taken := takes(sig)
		got.taken = taken
		_, _, got.again = takes(sig)
		if taken {
			gone = c
			if revoke && !back() {
				t.Error("revoke found the death already taken, before any command could take it")
			}
		}
		return 3
	})
	status, err = r.Run(context.Background(), f)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	if gone != nil {
		select {
		case <-gone:
			claimed = true
		default:
		}
	}
	return o.String() + e.String(), status, got, listened, claimed
}

// A fatal signal from outside is handed to the shell only where it has
// something to run first: an EXIT trap, in a dialect that runs one on a death
// or, for HUP, in the one that ends on it as an exit. Everywhere else the
// front end ends the process where the signal lands, as it did before
// (#7054).
//
// The shell takes what it is handed at its next command, so nothing after the
// `offer` runs; it runs the EXIT trap, shown the status it held when the
// signal landed, and stops at 128 plus the number. It does not end the
// process: in a library DieBySignal is nil, and this test's own process is
// still here to read the result.
func TestAFatalSignalFromOutsideIsTakenOnlyWhereThereIsSomethingToRun(t *testing.T) {
	runs, none := CoreSemantics(), CoreSemantics()
	runs.TrapBodyRunsWhatParsed, none.TrapBodyRunsWhatParsed = Yes, Yes
	runs.ExitTrapRunsOnSignalDeath, none.ExitTrapRunsOnSignalDeath = Yes, No
	runs.HangupIsAnOrderlyExit, none.HangupIsAnOrderlyExit = No, No
	hangupExits := none
	hangupExits.HangupIsAnOrderlyExit = Yes
	unanswered := CoreSemantics()
	unanswered.TrapBodyRunsWhatParsed = Yes
	const trap = `trap 'echo "bye $?"' EXIT; `
	for _, tc := range []struct {
		name        string
		src         string
		sem         Semantics
		sig         syscall.Signal
		interactive bool
		revoke      bool
		taken       bool
		out         string
		status      int
		listened    int
	}{
		{
			name: "an EXIT trap, in a dialect that runs it", src: trap + "offer; echo after", sem: runs,
			sig: syscall.SIGTERM, taken: true, out: "bye 3\n", status: 143, listened: 1,
		},
		{
			name: "no EXIT trap", src: "offer; echo after", sem: runs,
			sig: syscall.SIGTERM, out: "after\n",
		},
		{
			name: "an EXIT trap taken away", src: trap + "trap - EXIT; offer; echo after", sem: runs,
			sig: syscall.SIGTERM, out: "after\n", listened: 1,
		},
		{
			name: "a dialect that runs none on a death", src: trap + "offer; echo after", sem: none,
			sig: syscall.SIGTERM, out: "after\nbye 0\n",
		},
		{
			name: "a dialect that has not answered", src: trap + "offer; echo after", sem: unanswered,
			sig: syscall.SIGTERM, out: "after\nbye 0\n",
		},
		{
			name: "at a prompt", src: trap + "offer; echo after", sem: runs, interactive: true,
			sig: syscall.SIGTERM, out: "after\nbye 0\n",
		},
		{
			name: "a hangup that is an exit", src: trap + "offer; echo after", sem: hangupExits,
			sig: syscall.SIGHUP, taken: true, out: "bye 1\n", status: 1,
		},
		{
			name: "a hangup that is not an exit", src: trap + "offer; echo after", sem: none,
			sig: syscall.SIGHUP, out: "after\nbye 0\n",
		},
		{
			name: "taken back before the shell took it", src: trap + "offer; echo after", sem: runs, revoke: true,
			sig: syscall.SIGTERM, taken: true, out: "after\nbye 0\n", listened: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status, got, listened, claimed := offerADeath(t, tc.src, tc.sem, tc.sig, tc.interactive, tc.revoke)
			if got.taken != tc.taken {
				t.Errorf("taken = %v, want %v", got.taken, tc.taken)
			}
			if got.again {
				t.Error("a second offer was taken too, where a death already handed over must leave it to the front end")
			}
			if claimed != (tc.taken && !tc.revoke) {
				t.Errorf("claimed = %v, want %v", claimed, tc.taken && !tc.revoke)
			}
			if out != tc.out || status != tc.status {
				t.Errorf("wrote %q and ended %d, want %q and %d", out, status, tc.out, tc.status)
			}
			if listened != tc.listened {
				t.Errorf("ListenForFatalSignals called %d times, want %d", listened, tc.listened)
			}
		})
	}
}

// Without a front end that asked to hand it a death, nothing about a Runner
// changes: it is never armed, so nothing it waits for is given up, and the
// hook that would have the process listen is never called. An embedder's
// signals are the embedder's.
//
// The USR1 trap is what makes the row ask anything: it builds the signal
// state the arming is kept in, which an EXIT trap alone never does.
func TestARunnerNobodyHandsADeathToIsNeverArmed(t *testing.T) {
	sem := CoreSemantics()
	sem.TrapBodyRunsWhatParsed, sem.ExitTrapRunsOnSignalDeath = Yes, Yes
	f, err := syntax.Parse(`trap : USR1; trap 'echo "bye $?"' EXIT; echo after`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var o bytes.Buffer
	listened := 0
	r := newTestRunner(t, &Runner{Stdout: &o, Stderr: &o, Semantics: &sem, Name: "testsh"})
	r.ListenForFatalSignals = func() { listened++ }
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if listened != 0 {
		t.Errorf("ListenForFatalSignals called %d times by a Runner no front end listens for", listened)
	}
	if got := o.String(); !strings.HasSuffix(got, "bye 0\n") {
		t.Errorf("wrote %q", got)
	}
}
