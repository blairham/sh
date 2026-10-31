// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver_test

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// waitsHere is a program the shell waits for in the foreground that says the
// shell is ready only once it is running, so the signal can only land while
// the shell waits for it — and so the status the shell held before it is
// still the shell's, where a `printf` saying so would have replaced it.
const waitsHere = `/bin/sh -c 'printf r >"$1"; exec sleep 30' sh "$READY"`

// exitTrapHolds are the places the shell can be standing when a fatal signal
// from outside lands, each with the `$?` its EXIT trap is shown: what the
// shell held when the signal landed, which for a command the signal
// interrupted is what that command began with. A subshell's own EXIT trap
// does not run: in every reference the subshell is a process the signal never
// reached, and bash 5.3 prints `bye 7` alone for that row.
//
// Measured 2026-10-29 against bash 5.3.20, bash 3.2.57 and ksh93u+, each in a
// process group of its own with `trap 'echo "bye $?"' EXIT` and `/bin/kill
// -TERM` from another process: `(exit 7)` and then an external `sleep`, a
// `read`, `wait` for a job, the program inside a function, inside `( … )`,
// inside `$( … )` and in a `&&` list all print `bye 7` at once and die of the
// signal. A read and a wait the signal gave up do not show their own status:
// the read here would report 1 and the wait 143.
var exitTrapHolds = []struct{ name, hold, shown string }{
	{"an external command", "(exit 7); " + waitsHere, "7"},
	{"a builtin read", `printf r >"$READY"; read line`, "0"},
	{"a busy loop", `printf r >"$READY"; while :; do :; done`, "0"},
	{"a wait for a job", `sleep 30 & ! printf r >"$READY"; wait`, "1"},
	{"a function", "f() { (exit 7); " + waitsHere + "; echo in; }; f", "7"},
	{"a subshell", "(exit 7); ( " + waitsHere + "; echo in )", "7"},
	{"a subshell with an EXIT trap of its own", "(exit 7); ( trap 'echo in' EXIT; " + waitsHere + " )", "7"},
	{"a command substitution", "(exit 7); v=$(" + waitsHere + "; echo in)", "7"},
	{"a pipeline", waitsHere + " | cat", "0"},
	{"an and-list", "(exit 7); " + waitsHere + " && echo in", "7"},
}

// An untrapped fatal signal another process sends runs the EXIT trap before it
// ends the shell, in a dialect that runs one on a death — wherever the shell
// is standing, at once rather than once the command it waits for ends, and
// the shell is still killed by that signal afterwards (#7054).
//
// The front end's watch used to end the process where the signal landed, or
// leave TERM, INT and HUP to the runtime to do the same, so the trap that
// removes a script's temporary files never ran: bash 5.3, bash 3.2 and ksh93
// print `bye` in every one of these cells, and this shell printed nothing.
//
// The rows that nothing but the hand-over could pass are the read and the
// wait, which show the status before the command where the command's own
// would be 1 and 143, and the waits: a shell that only looked between
// commands would still be in `sleep 30` at the deadline.
func TestAFatalSignalFromOutsideRunsTheExitTrapFirst(t *testing.T) {
	dieRunningAsAShell()
	sigs := []namedSignal{
		{"TERM", syscall.SIGTERM},
		{"INT", syscall.SIGINT},
		{"HUP", syscall.SIGHUP},
		{"QUIT", syscall.SIGQUIT},
		{"USR1", syscall.SIGUSR1},
		{"ALRM", syscall.SIGALRM},
	}
	for _, hold := range exitTrapHolds {
		for _, s := range sigs {
			t.Run(hold.name+"/"+s.name, func(t *testing.T) {
				ws, out := signaledInAGroupOfItsOwn(t, "^TestAFatalSignalFromOutsideRunsTheExitTrapFirst$",
					outsideSignal{
						setup: `trap 'echo "bye $?"' EXIT; `, hold: hold.hold, sig: s.sig, ownReady: true,
						env: []string{dyingExitTrapRuns + "=yes", dyingHangupExits + "=no"},
					})
				diedOfAfter(t, ws, out, s.sig, "bye "+hold.shown+"\n")
			})
		}
	}
}

// What the EXIT trap does once it is running is the trap's: it is not given
// up for the death it is running ahead of, and an `exit` in it does not save
// the shell. A second fatal signal while it runs ends the shell at once, by
// that second signal; INT, which the dialect holds while a program runs, is
// held there too.
//
// Measured 2026-10-29 with `trap 'echo bye; /bin/sleep 1; echo bye2' EXIT;
// /bin/sleep 3` and TERM from outside, then a second signal 0.3s later: bash
// 5.3, bash 3.2 and ksh93 print `bye` and `bye2` a second later and die of
// TERM with no second signal; a second TERM, USR1 or HUP ends them at once
// with `bye` alone — bash by the second signal. `exit 5` in the trap leaves
// bash dying of TERM. These are that column; ksh93's answers to the last two
// are TestTheOneDialectWhereTheExitTrapDecidesHowTheShellLeaves.
func TestTheExitTrapRunsToItsEndAheadOfADeathFromOutside(t *testing.T) {
	dieRunningAsAShell()
	trapWaits := `trap 'echo bye; /bin/sh -c "printf r >\"\$AGAIN\"; exec sleep 30"; echo bye2' EXIT; `
	for _, tc := range []struct {
		name, setup, hold string
		again             syscall.Signal
		env               []string
		dies              syscall.Signal
		out               string
	}{
		{
			name:  "its own program is waited for",
			setup: `trap 'echo bye; /bin/sh -c "sleep 0.2; echo late"; echo bye2' EXIT; `,
			dies:  syscall.SIGTERM, out: "bye\nlate\nbye2\n",
		},
		{
			name:  "an exit in it",
			setup: `trap 'echo bye; exit 5' EXIT; `,
			dies:  syscall.SIGTERM, out: "bye\n",
		},
		{name: "a second TERM", setup: trapWaits, again: syscall.SIGTERM, dies: syscall.SIGTERM, out: "bye\n"},
		{name: "a second USR1", setup: trapWaits, again: syscall.SIGUSR1, dies: syscall.SIGUSR1, out: "bye\n"},
		{
			// The substitution's program goes on writing after its wait
			// was given up, while the trap keeps the shell alive to hear
			// it: what it writes is nobody's, and under the race detector
			// a write into the substitution's buffer after the
			// substitution read it is a report rather than silence.
			name:  "a substitution whose program writes after it was given up",
			setup: `trap 'echo "bye $?"; sleep 0.5' EXIT; `,
			hold: `(exit 7); v=$(/bin/sh -c 'printf r >"$1"; sleep 0.2; echo late; exec sleep 30' sh "$READY")` +
				`; echo "v=$v"`,
			dies: syscall.SIGTERM, out: "bye 7\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hold := tc.hold
			if hold == "" {
				hold = waitsHere
			}
			ws, out := signaledInAGroupOfItsOwn(t, "^TestTheExitTrapRunsToItsEndAheadOfADeathFromOutside$",
				outsideSignal{
					setup: tc.setup, hold: hold, sig: syscall.SIGTERM, again: tc.again, ownReady: true,
					env: append([]string{dyingExitTrapRuns + "=yes"}, tc.env...),
				})
			diedOfAfter(t, ws, out, tc.dies, tc.out)
		})
	}
}

// In one dialect the EXIT trap decides how the shell leaves: an `exit` that
// names a status in it ends the shell at that status rather than by the
// signal it was running ahead of, and a fatal signal from outside while it
// runs ends the trap and nothing more, so the shell leaves at the status or
// by the signal it was already leaving with (#7063, #7064).
//
// Measured 2026-10-30, each shell in a process group of its own and the
// signals sent by another process: ksh93u+ exits 5 for `trap 'echo bye; exit
// 5' EXIT` and a TERM or a USR1; dies of TERM for a TERM and then a USR1 or a
// HUP 0.3s later while the trap waits for a `sleep`, and of HUP for a HUP and
// then a TERM; exits 3 for a USR1 while the trap of an `exit 3` waits, and 4
// for a TERM while the trap of a script that ran off its end at `(exit 4)`
// waits. bash 5.3 and 3.2 die of the signal in every one of those cells, of
// the second where there are two, and that column is the test above.
//
// The rows nothing but the hand-over could pass are the ones whose trap is
// waiting: the shell that leaves as it was leaving has to give the wait up
// for the second signal, where the first one's own wait is long done.
func TestTheOneDialectWhereTheExitTrapDecidesHowTheShellLeaves(t *testing.T) {
	dieRunningAsAShell()
	trapWaits := `trap 'echo bye; /bin/sh -c "printf r >\"\$AGAIN\"; exec sleep 30"; echo bye2' EXIT; `
	trapWaitsReady := `trap 'echo bye; /bin/sh -c "printf r >\"\$READY\"; exec sleep 30"; echo bye2' EXIT; `
	const exits = `trap 'echo bye; exit 5' EXIT; `
	env := []string{
		dyingExitTrapRuns + "=yes", dyingHangupExits + "=no",
		dyingTrapEnds + "=yes", dyingExitOverrides + "=yes",
	}
	for _, tc := range []struct {
		name, setup, hold string
		sig, again        syscall.Signal
		dies              syscall.Signal
		status            int
		out               string
	}{
		{
			name: "an exit in it, for a TERM", setup: exits,
			sig: syscall.SIGTERM, status: 5, out: "bye\n",
		},
		{
			name: "an exit in it, for a USR1", setup: exits,
			sig: syscall.SIGUSR1, status: 5, out: "bye\n",
		},
		{
			name: "a second USR1", setup: trapWaits,
			sig: syscall.SIGTERM, again: syscall.SIGUSR1, dies: syscall.SIGTERM, out: "bye\n",
		},
		{
			name: "a second HUP", setup: trapWaits,
			sig: syscall.SIGTERM, again: syscall.SIGHUP, dies: syscall.SIGTERM, out: "bye\n",
		},
		{
			name: "a TERM after a HUP", setup: trapWaits,
			sig: syscall.SIGHUP, again: syscall.SIGTERM, dies: syscall.SIGHUP, out: "bye\n",
		},
		{
			name: "a USR1 in the trap of an exit", setup: trapWaitsReady, hold: "exit 3",
			sig: syscall.SIGUSR1, status: 3, out: "bye\n",
		},
		{
			name: "a TERM in the trap of a script that ran out", setup: trapWaitsReady, hold: "(exit 4)",
			sig: syscall.SIGTERM, status: 4, out: "bye\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hold := tc.hold
			if hold == "" {
				hold = waitsHere
			}
			ws, out := signaledInAGroupOfItsOwn(t, "^TestTheOneDialectWhereTheExitTrapDecidesHowTheShellLeaves$",
				outsideSignal{
					setup: tc.setup, hold: hold, sig: tc.sig, again: tc.again, ownReady: true, env: env,
				})
			if tc.dies != 0 {
				diedOfAfter(t, ws, out, tc.dies, tc.out)
				return
			}
			if ws.Signaled() {
				t.Fatalf("killed by %v, want an exit at %d (wrote %q)", ws.Signal(), tc.status, firstLine(out))
			}
			if ws.ExitStatus() != tc.status || out != tc.out {
				t.Errorf("status %d, wrote %q; want %d and %q", ws.ExitStatus(), out, tc.status, tc.out)
			}
		})
	}
}

// An untrapped SEGV from outside ends the shell where it lands in the dialect
// that says so, which runs its EXIT trap for every other fatal signal: before
// the trap, and in place of the end of a running one (#7069).
//
// Measured 2026-10-30, each shell in a process group of its own, `/bin/kill
// -SEGV` from another process: ksh93u+ writes nothing and dies of SEGV at
// `trap 'echo "bye $?"' EXIT; (exit 7); sleep 3`, and writes `in` and dies of
// SEGV while `trap 'echo in; sleep 3; echo still' EXIT; exit 3` waits —
// exiting 3 for TERM there — where bash 5.3 and 3.2 write `bye 7` and die of
// it. BUS runs ksh93's trap as TERM does.
//
// Only where the front end hears a SEGV at all: on darwin a sent one never
// reaches it, which TestAFatalSignalFromOutsideKillsTheShellQuietly asserts.
func TestAnUntrappedSegvFromOutsideEndsTheShellWhereItLands(t *testing.T) {
	dieRunningAsAShell()
	if !reachesTheFrontEnd(syscall.SIGSEGV) {
		t.Skip("a sent SEGV does not reach the front end here; see reachesTheFrontEnd")
	}
	trapWaitsReady := `trap 'echo in; /bin/sh -c "printf r >\"\$READY\"; exec sleep 30"; echo still' EXIT; `
	kills := []string{
		dyingExitTrapRuns + "=yes", dyingHangupExits + "=no",
		dyingTrapEnds + "=yes", dyingExitOverrides + "=yes", dyingSegvKills + "=yes",
	}
	runsTheTrap := []string{dyingExitTrapRuns + "=yes", dyingHangupExits + "=no"}
	for _, tc := range []struct {
		name, setup, hold string
		sig               syscall.Signal
		env               []string
		dies              syscall.Signal
		status            int
		out               string
	}{
		{
			name: "at a program", setup: `trap 'echo "bye $?"' EXIT; `, hold: "(exit 7); " + waitsHere,
			sig: syscall.SIGSEGV, env: kills, dies: syscall.SIGSEGV,
		},
		{
			name: "in the EXIT trap", setup: trapWaitsReady, hold: "exit 3",
			sig: syscall.SIGSEGV, env: kills, dies: syscall.SIGSEGV, out: "in\n",
		},
		{
			name: "another fatal signal in the same dialect", setup: `trap 'echo "bye $?"' EXIT; `,
			hold: "(exit 7); " + waitsHere, sig: syscall.SIGBUS, env: kills, dies: syscall.SIGBUS, out: "bye 7\n",
		},
		{
			name: "a TERM in the EXIT trap in the same dialect", setup: trapWaitsReady, hold: "exit 3",
			sig: syscall.SIGTERM, env: kills, status: 3, out: "in\n",
		},
		{
			name: "at a program, where the trap runs first", setup: `trap 'echo "bye $?"' EXIT; `,
			hold: "(exit 7); " + waitsHere, sig: syscall.SIGSEGV, env: runsTheTrap,
			dies: syscall.SIGSEGV, out: "bye 7\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws, out := signaledInAGroupOfItsOwn(t, "^TestAnUntrappedSegvFromOutsideEndsTheShellWhereItLands$",
				outsideSignal{setup: tc.setup, hold: tc.hold, sig: tc.sig, ownReady: true, env: tc.env})
			if tc.dies != 0 {
				diedOfAfter(t, ws, out, tc.dies, tc.out)
				return
			}
			if ws.Signaled() {
				t.Fatalf("killed by %v, want an exit at %d (wrote %q)", ws.Signal(), tc.status, firstLine(out))
			}
			if ws.ExitStatus() != tc.status || out != tc.out {
				t.Errorf("status %d, wrote %q; want %d and %q", ws.ExitStatus(), out, tc.status, tc.out)
			}
		})
	}
}

// Where nothing is to run first, nothing changes: the shell dies of the signal
// where it lands and writes nothing. That is a dialect that answers no —
// measured 2026-10-29, zsh and dash print nothing in every cell above — a
// dialect that has not answered, which is not asked from the watch's
// goroutine and reads as no, and a shell with no EXIT trap.
func TestAFatalSignalFromOutsideRunsNoExitTrapWhereNoneWouldRun(t *testing.T) {
	dieRunningAsAShell()
	const trapBye = `trap 'echo bye' EXIT; `
	for _, tc := range []struct {
		name, setup string
		env         []string
	}{
		{name: "a dialect that runs none on a death", setup: trapBye, env: []string{dyingExitTrapRuns + "=no"}},
		{name: "a dialect that has not answered", setup: trapBye},
		{name: "no EXIT trap", env: []string{dyingExitTrapRuns + "=yes"}},
		{name: "an EXIT trap taken away again", setup: trapBye + "trap - EXIT; ", env: []string{dyingExitTrapRuns + "=yes"}},
	} {
		for _, s := range []namedSignal{{"TERM", syscall.SIGTERM}, {"USR1", syscall.SIGUSR1}} {
			t.Run(tc.name+"/"+s.name, func(t *testing.T) {
				ws, out := signaledInAGroupOfItsOwn(t, "^TestAFatalSignalFromOutsideRunsNoExitTrapWhereNoneWouldRun$",
					outsideSignal{setup: tc.setup, hold: "(exit 7); " + waitsHere, sig: s.sig, ownReady: true, env: tc.env})
				diedOf(t, ws, out, s.sig)
			})
		}
	}
}

// One shell in the panel ends on an untrapped HUP the way `exit 1` ends it,
// and from outside that runs the EXIT trap too, with `$?` at 1 and an `exit`
// in it taking the status. Measured 2026-10-29 on zsh 5.9.2, a HUP from
// another process at a `sleep`, a `read`, a busy loop, a `( … )` and a
// pipeline: `bye 1` and exit 1, and `exit 5` in the trap exits 5. This shell
// exited 1 and printed nothing. See interp.Semantics.HangupIsAnOrderlyExit.
//
// The exit hook runs after the trap, once — at a `( … )` too, whose own copy
// of the hook does not: `zshexit` writes `bye 1` and then `hook 0` there, and
// the subshell's `hook 1` never appears.
func TestAnOrderlyHangupFromOutsideRunsTheExitTrap(t *testing.T) {
	dieRunningAsAShell()
	for _, tc := range []struct {
		name, setup, hold string
		status            int
		out               string
	}{
		{"at a program", `trap 'echo "bye $?"' EXIT; `, "(exit 7); " + waitsHere, 1, "bye 1\n"},
		{"at a read", `trap 'echo "bye $?"' EXIT; `, `printf r >"$READY"; read line`, 1, "bye 1\n"},
		{"with an exit in the trap", `trap 'echo bye; exit 5' EXIT; `, waitsHere, 5, "bye\n"},
		{
			"with an exit hook, at a subshell", `onexit() { echo hook; }; trap 'echo "bye $?"' EXIT; `,
			"( " + waitsHere + " )", 1, "bye 1\nhook\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws, out := signaledInAGroupOfItsOwn(t, "^TestAnOrderlyHangupFromOutsideRunsTheExitTrap$",
				outsideSignal{
					setup: tc.setup, hold: tc.hold, sig: syscall.SIGHUP, ownReady: true,
					env: []string{dyingHangupExits + "=1", dyingExitHook + "=onexit"},
				})
			if ws.Signaled() {
				t.Fatalf("killed by %v, want an exit at %d (wrote %q)", ws.Signal(), tc.status, firstLine(out))
			}
			if ws.ExitStatus() != tc.status || out != tc.out {
				t.Errorf("status %d, wrote %q; want %d and %q", ws.ExitStatus(), out, tc.status, tc.out)
			}
		})
	}
}

// A shell standing somewhere nothing gives up for the death — here one write
// into a pipe nobody reads — cannot take it over, and still dies of the
// signal: the watch takes it back after its grace and ends the process as it
// did before the shell could take one. The EXIT trap does not run there; a
// shell the signal did not end at all would be worse.
//
// And a background job carries on meanwhile, as a reference's child carries
// on after its parent is killed: it is not one of the copies the shell is
// waiting for, so neither its waits nor its commands stop for the death — its
// `read` waits for the `late` its program writes half a second in. The grace
// is what holds the death open long enough for the job to reach a command
// while it waits to be taken.
func TestADeathTheShellCannotTakeStillEndsIt(t *testing.T) {
	dieRunningAsAShell()
	for _, tc := range []struct{ name, job, out string }{
		{"alone", "", ""},
		{"with a background job", `{ /bin/sh -c "sleep 0.5; echo late" | { read -r x; echo "job $x"; }; } & `, "job late\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			full, writing := aPipeNobodyReads(t)
			ws, out := signaledInAGroupOfItsOwn(t, "^TestADeathTheShellCannotTakeStillEndsIt$",
				outsideSignal{
					setup: `trap 'echo bye' EXIT; ` + tc.job, hold: `printf '%01048576d' 0 >"$FULL"`,
					sig: syscall.SIGTERM, ownReady: true, readyWhen: writing,
					env: []string{dyingExitTrapRuns + "=yes", "FULL=" + full},
				})
			diedOf(t, ws, "", syscall.SIGTERM)
			if out != tc.out {
				t.Errorf("wrote %q, want %q", out, tc.out)
			}
		})
	}
}

// aPipeNobodyReads is a FIFO this test holds the reading end of and never
// drains, and a check that says a writer is inside a write into it: one byte
// read is one byte written, and a megabyte is more than a pipe holds anywhere
// this runs, so the writer is in the write and not about to start it — a
// write in progress has no command boundary to take a death at.
func aPipeNobodyReads(t *testing.T) (path string, writing func() bool) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "full")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Close(fd) })
	return path, func() bool {
		var b [1]byte
		n, _ := syscall.Read(fd, b[:])
		return n > 0
	}
}

// diedOfAfter asserts that the shell was killed by sig, not merely exited with
// a number it computed, and wrote exactly want first — no goroutine dump.
func diedOfAfter(t *testing.T, ws syscall.WaitStatus, out string, sig syscall.Signal, want string) {
	t.Helper()
	switch {
	case !ws.Signaled():
		t.Errorf("exited with %d rather than being killed by %v", ws.ExitStatus(), sig)
	case ws.Signal() != sig:
		t.Errorf("killed by %v, want %v", ws.Signal(), sig)
	}
	if out != want {
		if strings.Contains(out, "goroutine") {
			out = firstLine(out)
		}
		t.Errorf("wrote %q, want %q", out, want)
	}
}
