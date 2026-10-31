// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"syscall"
	"testing"
	"time"

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
	// A dialect with an exit hook, in each of the three: one that ends on a
	// HUP as an exit and runs no trap on a death, one that does neither, and
	// one that runs a trap on a death and ends on a HUP as an exit too —
	// the last is no shell's, and is what says the hook arms for the hangup
	// alone.
	hooked := func(s Semantics) Semantics {
		s.ExitHook, s.HookListSuffix = "zshexit", "_functions"
		return s
	}
	hookOnAHangup, hookNoHangup := hooked(hangupExits), hooked(none)
	hookBoth := hooked(runs)
	hookBoth.HangupIsAnOrderlyExit = Yes
	// And the shell that holds its caller's EXIT trap for a call, and loses
	// it to a stop that ends the shell from inside the call.
	holds := hangupExits
	holds.ExitTrapIsFunctionLocal, holds.StopInsideACallLosesTheHeldExitTrap = Yes, Yes
	const hook = `zshexit() { echo "hook $?"; }; `
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
		// The exit hook, with no EXIT trap (#7066).
		{
			name: "an exit hook, at a hangup that is an exit", src: hook + "offer; echo after", sem: hookOnAHangup,
			sig: syscall.SIGHUP, taken: true, out: "hook 1\n", status: 1,
		},
		{
			name: "an exit hook's list, at a hangup that is an exit", sem: hookOnAHangup,
			src: `h() { echo "hook $?"; }; zshexit_functions=(h); offer; echo after`,
			sig: syscall.SIGHUP, taken: true, out: "hook 1\n", status: 1,
		},
		{
			name: "an exit hook's list naming nothing defined", sem: hookOnAHangup,
			src: "zshexit_functions=(h); offer; echo after",
			sig: syscall.SIGHUP, out: "after\n",
		},
		{
			name: "an exit hook, at a hangup that is not an exit", src: hook + "offer; echo after", sem: hookNoHangup,
			sig: syscall.SIGHUP, out: "after\nhook 0\n",
		},
		{
			name: "an exit hook, at a death", src: hook + "offer; echo after", sem: hookBoth,
			sig: syscall.SIGTERM, out: "after\nhook 0\n",
		},
		{
			name: "an exit hook, at a hangup, in a dialect that runs a trap on a death", src: hook + "offer; echo after",
			sem: hookBoth, sig: syscall.SIGHUP, taken: true, out: "hook 1\n", status: 1,
		},
		// A hangup inside a call that holds its caller's trap (#7065).
		{
			name: "a hangup inside a call holding the trap", sem: holds,
			src: trap + "f() { trap 'echo own' EXIT; offer; echo in; }; f; echo after",
			sig: syscall.SIGHUP, taken: true, out: "own\n", status: 1,
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

// The hangup an exit hook runs for has already landed, so what the hook waits
// for is waited out rather than given up for it: measured 2026-10-30 on zsh
// 5.9.2, `zshexit() { s=$SECONDS; /bin/sleep 2; echo "hook $((SECONDS - s))";
// }` writes `hook 2` after a HUP from another process. Armed for the hook, the
// shell gave up every wait the hook made at once, on a death it had already
// taken.
func TestAnExitHookWaitsOutWhatItWaitsForAfterAHangupFromOutside(t *testing.T) {
	sem := CoreSemantics()
	sem.HangupIsAnOrderlyExit, sem.ExitTrapRunsOnSignalDeath = Yes, No
	sem.ExitHook = "zshexit"
	f, err := syntax.Parse(`zshexit() { read -r line; echo "hook read $line"; }; offer; echo after`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	in, feed := io.Pipe()
	defer func() { _ = in.Close() }()
	var o bytes.Buffer
	r := newTestRunner(t, &Runner{Stdin: in, Stdout: &o, Stderr: &o, Semantics: &sem, Name: "testsh"})
	takes := r.TakesAFatalSignal()
	r.Register("offer", func(*Runner, context.Context, []string) int {
		if _, _, taken := takes(syscall.SIGHUP); !taken {
			t.Error("the hangup was not taken, with an exit hook to run")
		}
		// The line arrives once the hook is reading, well after the
		// hangup was handed over.
		go func() {
			time.Sleep(200 * time.Millisecond)
			_, _ = io.WriteString(feed, "line\n")
			_ = feed.Close()
		}()
		return 0
	})
	status, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if got := o.String(); got != "hook read line\n" || status != 1 {
		t.Errorf("wrote %q and ended %d, want %q and 1", got, status, "hook read line\n")
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
