// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/internal/testenv"
	"github.com/blairham/sh/interp"
)

// namedSignal is a signal and the word a script writes for it.
type namedSignal struct {
	name string
	sig  syscall.Signal
}

// The places a shell can be standing when the signal arrives, and each one
// waits on something different: a builtin reading a pipe nobody writes, a
// child it is waiting for, and nothing at all.
var outsideSignalHolds = []struct{ name, src string }{
	{"a builtin read", "read line"},
	{"an external command", "sleep 30"},
	{"a busy loop", "while :; do :; done"},
}

// An untrapped signal whose default action ends a process ends this shell
// when another process sends it, wherever the shell is standing.
//
// The runtime catches every one of these and, with nothing subscribed, drops
// it — or, for EMT, prints a goroutine dump and exits 2. Measured 2026-10-29
// with `/bin/kill -SIG` from another process, each shell in a process group of
// its own and parked in each of the three holds: bash 5.3.20, bash 3.2.57,
// ksh93u+, zsh 5.9.2 and dash die of USR1, USR2, ALRM, VTALRM, XCPU and EMT in
// every cell and print nothing. This shell carried on in every cell, and wrote
// a page of Go for EMT (#7046).
func TestAnUntrappedSignalFromOutsideEndsTheShell(t *testing.T) {
	dieRunningAsAShell()
	sigs := append([]namedSignal{
		{"USR1", syscall.SIGUSR1},
		{"USR2", syscall.SIGUSR2},
		{"ALRM", syscall.SIGALRM},
		{"VTALRM", syscall.SIGVTALRM},
		{"XCPU", syscall.SIGXCPU},
	}, platformEndsTheShellFromOutside...)
	for _, hold := range outsideSignalHolds {
		for _, s := range sigs {
			t.Run(hold.name+"/"+s.name, func(t *testing.T) {
				ws, out := signaledInAGroupOfItsOwn(t, "^TestAnUntrappedSignalFromOutsideEndsTheShell$",
					outsideSignal{hold: hold.src, sig: s.sig})
				diedOf(t, ws, out, s.sig)
			})
		}
	}
}

// A start does not wait for the watch to take the signals the runtime drops —
// each is a round trip to the runtime's signal-mask thread, and on Linux they
// came to about a millisecond of every start (#7263) — but the shell's first
// command does. So a signal sent once the script has run anything still ends
// the shell, however long the watch took: here it is held back far longer
// than a start takes, and the signal goes the moment the script says it is
// ready. Without the join the script is ready long before the watch is, the
// runtime drops the signal, and the shell sits in its `read`.
func TestASignalFromOutsideFindsTheWatchReadyAtTheFirstCommand(t *testing.T) {
	dieRunningAsAShell()
	for _, s := range []namedSignal{{"USR1", syscall.SIGUSR1}, {"ALRM", syscall.SIGALRM}} {
		t.Run(s.name, func(t *testing.T) {
			ws, out := signaledInAGroupOfItsOwn(t, "^TestASignalFromOutsideFindsTheWatchReadyAtTheFirstCommand$",
				outsideSignal{hold: "read line", sig: s.sig, env: []string{dyingHeldWatch + "=1"}})
			diedOf(t, ws, out, s.sig)
		})
	}
}

// The same under `-i`, which is a different road to the watch: an interactive
// shell hands it a second answer about what it ignores. bash 5.3, bash 3.2 and
// dash die of all of these under `-i -c` as well; see
// TestAnInteractiveShellAnswersAnAlarmFromOutsideItsOwnWay for the two
// interactive shells that do something else with ALRM and XCPU.
func TestAnInteractiveShellDiesOfAnUntrappedUserSignal(t *testing.T) {
	dieRunningAsAShell()
	for _, s := range []namedSignal{{"USR1", syscall.SIGUSR1}, {"VTALRM", syscall.SIGVTALRM}} {
		t.Run(s.name, func(t *testing.T) {
			ws, out := signaledInAGroupOfItsOwn(t, "^TestAnInteractiveShellDiesOfAnUntrappedUserSignal$",
				outsideSignal{hold: "read line", sig: s.sig, env: []string{dyingInteractive + "=1"}})
			if !ws.Signaled() || ws.Signal() != s.sig {
				t.Errorf("ended %v, want killed by %v (wrote %q)", ws, s.sig, firstLine(out))
			}
			if strings.Contains(out, "goroutine") {
				t.Errorf("wrote a goroutine dump: %q", firstLine(out))
			}
		})
	}
}

// Two interactive shells answer an untrapped ALRM from outside with something
// other than its default action, and one of them answers XCPU that way too.
//
// Measured 2026-10-30 with `/bin/kill -SIG` from another process at a shell
// under `-i -c` in a `read`, an external `sleep` and a busy loop, and at a
// prompt in a session on a pseudo-terminal: zsh 5.9.2 writes `zsh: timeout`
// at the prompt and exits 14, and ksh93u+ carries on — for ALRM, and for XCPU
// and XFSZ as well — where bash 5.3.20, bash 3.2.57, dash and BusyBox ash die
// of each (#7058). See interp.Semantics.InteractiveAlarm.
func TestAnInteractiveShellAnswersAnAlarmFromOutsideItsOwnWay(t *testing.T) {
	dieRunningAsAShell()
	answered := func(a interp.InteractiveAlarmAnswer, interactive bool) []string {
		env := []string{dyingInteractiveAlarm + "=" + strconv.Itoa(int(a))}
		if interactive {
			env = append(env, dyingInteractive+"=1")
		}
		return env
	}
	for _, hold := range outsideSignalHolds {
		t.Run(hold.name, func(t *testing.T) {
			ws, out := signaledInAGroupOfItsOwn(t, "^TestAnInteractiveShellAnswersAnAlarmFromOutsideItsOwnWay$",
				outsideSignal{hold: hold.src, sig: syscall.SIGALRM, env: answered(interp.InteractiveAlarmTimesOut, true)})
			if ws.Signaled() || ws.ExitStatus() != int(syscall.SIGALRM) || out != "testsh: timeout\n" {
				t.Errorf("a timeout: ended %v, wrote %q; want an exit at %d and %q",
					ws, out, int(syscall.SIGALRM), "testsh: timeout\n")
			}
		})
	}
	for _, c := range []struct {
		name        string
		answer      interp.InteractiveAlarmAnswer
		interactive bool
		setup       string
		sig         syscall.Signal
		survives    bool
	}{
		{"ignored", interp.InteractiveAlarmIsIgnored, true, "", syscall.SIGALRM, true},
		{"ignored, and a trap does not run", interp.InteractiveAlarmIsIgnored, true, "trap 'echo T' ALRM; ", syscall.SIGALRM, true},
		{"ignored, XCPU beside it", interp.InteractiveAlarmIsIgnored, true, "", syscall.SIGXCPU, true},
		{"the default action", interp.InteractiveAlarmKills, true, "", syscall.SIGALRM, false},
		{"a timeout given back to the default", interp.InteractiveAlarmTimesOut, true, "trap - ALRM; ", syscall.SIGALRM, false},
		{"a timeout is not interactive's", interp.InteractiveAlarmTimesOut, false, "", syscall.SIGALRM, false},
		{"an ignore is not interactive's", interp.InteractiveAlarmIsIgnored, false, "", syscall.SIGALRM, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			hold := "read line; echo after"
			o := outsideSignal{setup: c.setup, hold: hold, sig: c.sig, env: answered(c.answer, c.interactive)}
			if c.survives {
				// Then the hold has to end on its own for the shell to be
				// seen carrying on.
				o.hold, o.stdinClosed = "sleep 0.3; echo after", true
			}
			ws, out := signaledInAGroupOfItsOwn(t, "^TestAnInteractiveShellAnswersAnAlarmFromOutsideItsOwnWay$", o)
			if !c.survives {
				diedOf(t, ws, out, c.sig)
				return
			}
			if ws.Signaled() || ws.ExitStatus() != 0 || out != "after\n" {
				t.Errorf("ended %v, wrote %q; want %q at 0", ws, out, "after\n")
			}
		})
	}
}

// `trap - SIG` hands a signal from outside back to its default action, which
// for each of these is to end the shell, however the signal was arranged
// before.
//
// A reset is signal.Reset in a shell that owns its process, and that takes
// the front end's own watch off the signal with everything else. Measured
// 2026-10-29, each arrangement and then the signal from another process at a
// builtin `read`: bash 5.3, bash 3.2, ksh93, zsh and dash die of TERM, INT,
// USR1, ALRM and EMT after every one of the three, and of QUIT and HUP too
// wherever the dialect does not answer otherwise. This shell survived every
// signal after `trap "" SIG; trap - SIG` (#7047), and wrote a goroutine dump
// for QUIT and EMT after `trap 'x' SIG; trap - SIG` (#7048).
func TestAResetHandsASignalFromOutsideBackToItsDefault(t *testing.T) {
	dieRunningAsAShell()
	sigs := []namedSignal{
		{"TERM", syscall.SIGTERM},
		{"INT", syscall.SIGINT},
		{"HUP", syscall.SIGHUP},
		{"QUIT", syscall.SIGQUIT},
		{"USR1", syscall.SIGUSR1},
		{"ALRM", syscall.SIGALRM},
	}
	if len(platformEndsTheShellFromOutside) > 0 {
		sigs = append(sigs, platformEndsTheShellFromOutside[0])
	}
	for _, arranged := range []struct{ name, src string }{
		{"ignored, then reset", "trap '' %s; trap - %s; "},
		{"handled, then reset", "trap ':' %s; trap - %s; "},
		{"reset alone", "trap - %s; "},
	} {
		for _, s := range sigs {
			t.Run(arranged.name+"/"+s.name, func(t *testing.T) {
				ws, out := signaledInAGroupOfItsOwn(t, "^TestAResetHandsASignalFromOutsideBackToItsDefault$",
					outsideSignal{setup: strings.ReplaceAll(arranged.src, "%s", s.name), hold: "read line", sig: s.sig})
				diedOf(t, ws, out, s.sig)
			})
		}
	}
}

// What the script arranged is still the script's: the watch that ends the
// shell asks first, and the controls here are what say the deaths above are
// the signal's default and not a watch that ends the shell for anything.
//
// An ignore survives in every column of the panel, a handler runs, and a
// handler set again after a reset runs too — the watch put back by the reset
// must not outlive the next trap. The last two rows are the kernel's copy of
// a signal the shell raised with a write of its own, after it was ignored and
// reset: a pipeline element breaking its pipe, and a subshell writing past
// `ulimit -f`, end the element and the subshell and not the shell. Measured
// 2026-10-29, both print `after` in bash 5.3, bash 3.2, ksh93, zsh and dash,
// so the watch must not have been put on PIPE or XFSZ by the reset. darwin's
// runtime never hands a PIPE to anybody, which is why the second row is here,
// and `capped` is what says the limit was in force and the write ran into it.
func TestASignalFromOutsideTheScriptArrangedIsTheScripts(t *testing.T) {
	dieRunningAsAShell()
	for _, tc := range []struct {
		name, setup, hold string
		sig               syscall.Signal
		want              string
	}{
		{"ignored USR1", "trap '' USR1; ", "sleep 1; echo survived", syscall.SIGUSR1, "survived\n"},
		{"ignored TERM", "trap '' TERM; ", "sleep 1; echo survived", syscall.SIGTERM, "survived\n"},
		{"handled ALRM", "trap 'echo caught' ALRM; ", "sleep 1; echo after", syscall.SIGALRM, "caught\nafter\n"},
		{
			"handled again after a reset", "trap '' USR2; trap - USR2; trap 'echo caught' USR2; ",
			"sleep 1; echo after", syscall.SIGUSR2, "caught\nafter\n",
		},
		{
			"a broken pipe of its own after a reset", "trap '' PIPE; trap - PIPE; ",
			"while :; do echo y; done | head -n 1 >/dev/null; echo after", 0, "after\n",
		},
		{
			"a file too large of its own after a reset", "trap '' XFSZ; trap - XFSZ; ",
			`ulimit -S -f 1; (printf '%4000s' x >"$SCRATCH/big") 2>/dev/null; echo after; ` +
				`case $(wc -c <"$SCRATCH/big") in *1024) echo capped; esac`, 0, "after\ncapped\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws, out := signaledInAGroupOfItsOwn(t, "^TestASignalFromOutsideTheScriptArrangedIsTheScripts$",
				outsideSignal{setup: tc.setup, hold: tc.hold, sig: tc.sig, stdinClosed: true})
			if ws.Signaled() {
				t.Fatalf("killed by %v, want the shell to carry on (wrote %q)", ws.Signal(), firstLine(out))
			}
			if ws.ExitStatus() != 0 || out != tc.want {
				t.Errorf("status %d, wrote %q; want 0 and %q", ws.ExitStatus(), out, tc.want)
			}
		})
	}
}

// One shell in the panel ends on an untrapped HUP the way `exit 1` ends it,
// and a HUP from another process is no exception: measured 2026-10-29, zsh
// 5.9.2 leaves 1 for one sent while it is in a builtin `read`, an external
// `sleep` or a busy loop, and after `trap "" HUP; trap - HUP` as well, where
// bash 5.3, bash 3.2, ksh93 and dash die of it. This shell died of it in every
// dialect (#7046). See interp.Semantics.HangupIsAnOrderlyExit.
//
// The last two rows are a shell started under `nohup`. Every column keeps that
// ignore through `trap - HUP` but zsh, and a watch put back on HUP by the
// reset would take the ignore away.
func TestAnUntrappedHangupFromOutsideEndsTheShellTheDialectsWay(t *testing.T) {
	dieRunningAsAShell()
	exits := dyingHangupExits + "=1"
	for _, tc := range []struct {
		name  string
		setup string
		env   []string
		wrap  []string
		// status is the exit status wanted, or -1 for a death by HUP.
		status int
		out    string
	}{
		{name: "dies of it", status: -1},
		{name: "exits", env: []string{exits}, status: 1},
		{name: "exits after a reset", setup: "trap '' HUP; trap - HUP; ", env: []string{exits}, status: 1},
		{name: "born ignoring it", wrap: []string{"/usr/bin/nohup"}, status: 0, out: "survived\n"},
		{
			name: "born ignoring it, then reset", setup: "trap - HUP; ", wrap: []string{"/usr/bin/nohup"},
			status: 0, out: "survived\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hold := "read line"
			if tc.out != "" {
				hold = "sleep 1; echo survived"
			}
			ws, out := signaledInAGroupOfItsOwn(t,
				"^TestAnUntrappedHangupFromOutsideEndsTheShellTheDialectsWay$",
				outsideSignal{setup: tc.setup, hold: hold, sig: syscall.SIGHUP, env: tc.env, wrap: tc.wrap})
			if tc.status < 0 {
				diedOf(t, ws, out, syscall.SIGHUP)
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

// diedOf asserts that the shell was killed by sig, not merely exited with a
// number it computed, and said nothing — no goroutine dump above all.
func diedOf(t *testing.T, ws syscall.WaitStatus, out string, sig syscall.Signal) {
	t.Helper()
	switch {
	case !ws.Signaled():
		t.Errorf("exited with %d rather than being killed by %v", ws.ExitStatus(), sig)
	case ws.Signal() != sig:
		t.Errorf("killed by %v, want %v", ws.Signal(), sig)
	}
	if out != "" {
		t.Errorf("wrote %q, want nothing at all", firstLine(out))
	}
}

// outsideSignal is one shell for signaledInAGroupOfItsOwn to start and signal.
type outsideSignal struct {
	// setup runs before the shell says it is ready, and hold after.
	setup, hold string
	// sig is sent once the shell is ready; 0 sends nothing.
	sig syscall.Signal
	// env is added to the shell half's environment.
	env []string
	// wrap is a command the test binary is started through, such as nohup.
	wrap []string
	// stdinClosed gives the shell an empty standard input rather than a pipe
	// nobody writes, for a hold that must end on its own.
	stdinClosed bool
	// ownReady leaves saying it is ready to the script, which writes the
	// file `$READY` names itself — from inside a hold, where the moment the
	// shell is ready is the hold's to know.
	ownReady bool
	// again is sent once the file `$AGAIN` names exists, for a script that
	// has something to be doing when a second signal lands; 0 sends none.
	again syscall.Signal
	// readyWhen, where set, says the shell is ready in place of the file,
	// for a hold whose readiness only the test can see.
	readyWhen func() bool
}

// signaledInAGroupOfItsOwn re-executes this test binary as a shell in a
// process group of its own, waits for it to say it is ready, sends it the
// signal from here, and reports how it ended and what it wrote.
//
// shellSignaledFromOutside's discipline — a file that says the shell is
// ready, a deadline that fires rather than a stopwatch, output to a file so
// the wait is for the process and not for a child holding a pipe — with three
// things more. The group is the shell's own, so a signal it aims at its group
// reaches nothing here, and whatever it started is killed with it afterwards.
// Its standard input is a pipe this side holds open and never writes, which
// is what keeps a `read` waiting. And it can be started through a wrapper,
// which is how a shell is born ignoring something. `$SCRATCH` is a directory
// the shell may write in.
func signaledInAGroupOfItsOwn(t *testing.T, name string, o outsideSignal) (syscall.WaitStatus, string) {
	t.Helper()
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	again := filepath.Join(dir, "again")
	argv := append(append([]string{}, o.wrap...), os.Args[0], "-test.run="+name)
	cmd := exec.Command(argv[0], argv[1:]...)
	script := o.setup + "printf r >" + ready + "; " + o.hold
	if o.ownReady {
		script = o.setup + o.hold
	}
	cmd.Env = append(os.Environ(), dyingScript+"="+script, "SCRATCH="+dir, "READY="+ready, "AGAIN="+again)
	cmd.Env = testenv.WithoutRaceExitPause(append(cmd.Env, o.env...))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	logPath := filepath.Join(dir, "out")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()
	cmd.Stdout, cmd.Stderr = log, log
	if !o.stdinClosed {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = w.Close() }()
		defer func() { _ = r.Close() }()
		cmd.Stdin = r
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the shell half: %v", err)
	}
	// The whole group, so a `sleep` the shell left behind goes too.
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })

	reaped := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(reaped)
	}()
	deadline := time.Now().Add(deathDeadline)
	isReady := func() bool {
		b, err := os.ReadFile(ready)
		return err == nil && len(b) > 0
	}
	if o.readyWhen != nil {
		isReady = o.readyWhen
	}
	for !isReady() {
		select {
		case <-reaped:
			t.Fatalf("the shell half ended before it was ready: %q", readLog(t, logPath))
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("the shell half never started: %q", readLog(t, logPath))
		}
		time.Sleep(5 * time.Millisecond)
	}
	if o.sig != 0 {
		if err := cmd.Process.Signal(o.sig); err != nil {
			t.Fatalf("signaling the shell half: %v", err)
		}
	}
	if o.again != 0 {
		for {
			if _, err := os.Stat(again); err == nil {
				break
			}
			select {
			case <-reaped:
				t.Fatalf("the shell ended before it was ready for a second signal: %q", readLog(t, logPath))
			default:
			}
			if time.Now().After(deadline) {
				t.Fatalf("the shell was never ready for a second signal: %q", readLog(t, logPath))
			}
			time.Sleep(5 * time.Millisecond)
		}
		if err := cmd.Process.Signal(o.again); err != nil {
			t.Fatalf("signaling the shell half again: %v", err)
		}
	}
	select {
	case <-reaped:
	case <-time.After(deathDeadline):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-reaped
		t.Fatalf("the shell was still running %v after being sent %v: %q",
			deathDeadline, o.sig, readLog(t, logPath))
	}
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		t.Fatalf("no wait status for the shell half")
	}
	return ws, readLog(t, logPath)
}
