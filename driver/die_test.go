// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// dyingScript names the script a re-executed copy of this test binary should
// run as a shell. Set, it means "you are the shell"; unset, "you are the
// test".
const dyingScript = "SH_TEST_DYING_SCRIPT"

// deathDeadline is how long a shell that has been asked to die is given
// before this test gives up on it and says so.
//
// **It is a deadline that fires, not a stopwatch read afterwards.** That is
// the whole of what changed, and the reason is that the two catch different
// things. A stopwatch cannot see the defect this exists for — a shell that
// parks forever leaves the wait blocking, and the run ends at the harness
// timeout with a goroutine dump naming nothing, which the stopwatch never
// gets to read. What the stopwatch *could* see was how long a successful
// death took, which nobody needs: it is not a performance assertion, and
// twice it failed unrelated pull requests (#627, #635, #1088) for saying so.
//
// So the number is now generous on purpose. A passing run never reaches it —
// the wait returns as soon as the process is reaped — and a run that does
// reach it is a hang, which is the only thing worth reporting. Raising a
// stopwatch's number buys time; raising a deadline's costs nothing.
//
// **What a slow death is measured at.** Signal sent to process reaped, on
// Linux under -race: 1ms median (N=48, `docker run --cpus=1`), 6ms median and
// 91ms worst under nine runnable threads on that one cpu (N=48). On macOS
// 8ms median idle (N=25). The two failures that reopened this were 4.07s and
// 4.28s on ubuntu-latest, which is 40000 times the work — see writeNoCore for
// what was actually taking that long and why the guard against it had stopped
// working.
//
// **What is no longer asserted here, and where it moved.** The old comment
// noted that the budget sat under the raise's own grace period (deathGrace),
// so a shell that gave up and exited with a number was caught by the clock as
// well as by the wait status. It is still caught, by the wait status, which is
// the assertion that names the thing rather than a proxy for it: ws.Signaled()
// is false for such a shell whatever the clock says.
const deathDeadline = 30 * time.Second

// dieRunningAsAShell turns this process into a shell when it was re-executed
// as one, and does nothing otherwise.
//
// A second process is not caution here, it is the only way to ask the
// question. The behavior under test is *this process being killed*, and a
// test that ran it in-process would take the test binary with it — along
// with, for the signals involved, the disposition change that gets it there,
// which is process-global and survives exec.
func dieRunningAsAShell() {
	src := os.Getenv(dyingScript)
	if src == "" {
		return
	}
	writeNoCore()
	sh := shell()
	// The two axes a `trap` needs answered. Only one case here installs a
	// handler, and the core refuses what the shells disagree about — so
	// without these that shell reports an unchosen dialect instead of
	// running the trap, and the test reads a diagnostic where it wanted a
	// handler. Which way they are answered does not matter to it.
	sh.Semantics.TrapBodyRunsWhatParsed = interp.Yes
	sh.Semantics.SignalHandlerSeesEarlierStatus = interp.Yes
	os.Exit(driver.MainArgs(sh, []string{"testsh", "-c", src}))
}

// writeNoCore stops this process dumping core when a signal kills it, which is
// what a shell's own `ulimit -c 0` does and is scoped to the shell half rather
// than to the test binary.
//
// Not tidiness. Every signal these tests raise is a core-dumping one, and a
// race-instrumented test binary is large — 458MB of image, measured. On a CI
// runner the kernel spent 1.5 seconds writing it before reaping the process,
// so the timing this file is here to measure was the core writer's rather than
// the shell's.
//
// **The resource limit alone does not do it, and that is why this came back.**
// A Linux core_pattern beginning with `|` names a program to pipe the image to,
// which is what a distribution with a crash reporter installs. On that path the
// kernel does not enforce RLIMIT_CORE at all — the limit is consulted only for
// a dump it writes itself, plus the one reserved value of 1 that stops the
// helper dumping recursively. So `ulimit -c 0` reads as a guard and is not one,
// on exactly the machines that have a crash reporter.
//
// Measured, `docker run --cpus=1` with a race-instrumented binary and
// core_pattern set to a pipe: 156ms median with the limit alone (N=16), 1ms
// median with refuseToBeDumped as well (N=16), against 1ms with no pipe at all.
// A 156-fold difference on a fast local disk with nothing else running, for
// work that scales with the size of the image and the load on the machine —
// which is the shape of a 4-second death on a busy runner, and the reason two
// pull requests about arrays and about `for` were failed by a test about
// signals.
//
// refuseToBeDumped is the part that holds either way: it tells the kernel this
// process is not to be dumped at all, so there is no image to write and no
// helper to start.
func writeNoCore() {
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_CORE, &lim); err != nil {
		return
	}
	lim.Cur = 0
	_ = syscall.Setrlimit(syscall.RLIMIT_CORE, &lim)
	refuseToBeDumped()
}

// waitStatusOfAShell re-executes this test binary as a shell running src and
// reports how it ended and what it wrote.
//
// The deadline is the context's, so a shell that never dies is killed and
// named here rather than left to hold the harness open — see deathDeadline.
// It covers the fork and the exec as well as the death, which is deliberate:
// on this route there is no moment to start counting from, and a deadline
// generous enough for a hang does not care.
func waitStatusOfAShell(t *testing.T, name, src string) (syscall.WaitStatus, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), deathDeadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run="+name)
	cmd.Env = append(os.Environ(), dyingScript+"="+src)
	// So the deadline is one a child cannot outlive. CombinedOutput reads
	// through a pipe, and a pipe is held open by whatever inherited it — a
	// `sleep` the shell left behind would keep the read going after the
	// process itself had been killed, which would put the hang back in a
	// second form. WaitDelay closes the descriptors a second after the
	// process is gone.
	cmd.WaitDelay = time.Second
	out, _ := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("%s: the shell was still running after %v and had to be killed — "+
			"it never died of the signal", src, deathDeadline)
	}
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		t.Fatalf("no wait status for the shell half")
	}
	return ws, string(out)
}

// killedBy asserts the shape every one of these cases wants: the shell was
// killed by the signal it raised, it did not merely exit with a number it
// computed, it said nothing, and it did it promptly.
func killedBy(t *testing.T, name, src string, want syscall.Signal) {
	t.Helper()
	ws, out := waitStatusOfAShell(t, name, src)
	switch {
	case !ws.Signaled():
		t.Errorf("%s: exited with %d rather than being killed by a signal", src, ws.ExitStatus())
	case ws.Signal() != want:
		t.Errorf("%s: killed by %v, want %v", src, ws.Signal(), want)
	}
	if out != "" {
		// Truncated on purpose: the failure this reports is a goroutine dump,
		// and a test that reprints the whole of one buries every other line of
		// the run. The first of it says which signal and is enough to tell
		// this apart from a shell writing a diagnostic of its own.
		t.Errorf("%s: wrote %q, want nothing at all", src, firstLine(out))
	}
}

// firstLine is out up to its first newline, so a failure quotes a line rather
// than a crash report.
func firstLine(out string) string {
	if i := strings.IndexByte(out, '\n'); i >= 0 {
		return out[:i]
	}
	return out
}

// The signal the runtime forwards. This one worked before anything below did,
// and it is here so that a change that breaks the others is told apart from
// one that breaks the seam itself.
func TestAForwardedSignalKillsTheShell(t *testing.T) {
	dieRunningAsAShell()
	killedBy(t, "TestAForwardedSignalKillsTheShell", "kill -TERM $$; echo after", syscall.SIGTERM)
}

// The signals the runtime discards. Every one of these left the shell parked
// in the raise's own sleep loop, alive and holding the harness open for its
// full timeout, because nothing had put the default action back.
func TestADiscardedSignalKillsTheShell(t *testing.T) {
	dieRunningAsAShell()
	for _, c := range []struct {
		name string
		sig  syscall.Signal
	}{
		{"USR1", syscall.SIGUSR1},
		{"USR2", syscall.SIGUSR2},
		{"ALRM", syscall.SIGALRM},
		{"PIPE", syscall.SIGPIPE},
	} {
		killedBy(t, "TestADiscardedSignalKillsTheShell", "kill -"+c.name+" $$; echo after", c.sig)
	}
}

// The signal that has no disposition to put back. Asking the kernel to reset
// SIGKILL is EINVAL rather than a no-op, and reporting that error printed
// `kill: invalid argument` on the way to a death that was otherwise right.
func TestTheUncatchableSignalNeedsNoDispositionPutBack(t *testing.T) {
	dieRunningAsAShell()
	killedBy(t, "TestTheUncatchableSignalNeedsNoDispositionPutBack",
		"kill -KILL $$; echo after", syscall.SIGKILL)
}

// The signals the runtime turns into a crash. These did end the process, and
// ended it wrongly in two ways at once: an exit status of 2 rather than a
// death, with a full goroutine dump on standard error where a shell writes
// nothing.
func TestACrashingSignalKillsTheShellQuietly(t *testing.T) {
	dieRunningAsAShell()
	killedBy(t, "TestACrashingSignalKillsTheShellQuietly", "kill -ABRT $$; echo after", syscall.SIGABRT)
}

// The same signals, arriving from another process.
//
// This is the road the raise does not take. dieBySignal puts the kernel's
// default action back before raising, which is why `kill -QUIT $$` above dies
// quietly — and a signal sent by somebody else never reaches that code, so the
// runtime's own handler answers it. For the throwing class that answer is a
// goroutine dump and an exit status of 2, where bash, dash and zsh all die of
// the signal and print nothing. Measured against the panel with the shell in
// the foreground, so that nothing had inherited an ignore: all three agree on
// all eight, and this shell disagreed with all three on all eight.
//
// It is the last road for the leak internal/panicguard exists to close, and it
// is the one the corpus cannot see: every signal-death case in it self-kills.
func TestAFatalSignalFromOutsideKillsTheShellQuietly(t *testing.T) {
	dieRunningAsAShell()
	for _, sig := range []syscall.Signal{
		syscall.SIGQUIT, syscall.SIGABRT, syscall.SIGFPE, syscall.SIGTRAP,
		syscall.SIGSYS, syscall.SIGILL, syscall.SIGBUS, syscall.SIGSEGV,
	} {
		t.Run(sig.String(), func(t *testing.T) {
			ws, out := shellSignaledFromOutside(t,
				"TestAFatalSignalFromOutsideKillsTheShellQuietly", "", countedSleeps, sig)

			if !reachesTheFrontEnd(sig) {
				// Asserted rather than skipped, so that the day the
				// platform changes is a failing test rather than a test
				// that has been passing vacuously for a year. What is
				// asserted is what this kernel does: see
				// reachesTheFrontEnd.
				if ws.Signaled() {
					t.Fatalf("killed by %v — this platform now reaches the front end "+
						"with a sent %v, so reachesTheFrontEnd is out of date and the "+
						"exception it guards can go", ws.Signal(), sig)
				}
				return
			}
			switch {
			case !ws.Signaled():
				t.Errorf("exited with %d rather than being killed by %v", ws.ExitStatus(), sig)
			case ws.Signal() != sig:
				t.Errorf("killed by %v, want %v", ws.Signal(), sig)
			}
			if out != "" {
				t.Errorf("wrote %q, want nothing at all", firstLine(out))
			}
		})
	}
}

// reachesTheFrontEnd reports whether a signal *sent* to this process is handed
// to the front end rather than left to the Go runtime.
//
// One fact now, where there used to be two.
//
// The one that is gone: the front end did not listen on darwin at all, because
// asking os/signal for a signal makes the runtime open a pipe to carry it and
// a script's `exec 3>f` then wrote over it — #799, which is the same fault as
// #695 and #731 arriving through signal handling instead of through the
// poller. driver/lowfds_unix.go keeps the runtime above every descriptor a
// script can name, so the pipe is out of reach and the whole class is the
// front end's on every platform this builds on.
//
// The one that remains is the kernel's. The runtime lets a sent signal through
// to os/signal for the throwing class only when the kernel marks it as one a
// *process* sent, and macOS does not do that for SEGV, BUS or ILL. Measured
// with a program that registers all eight and is then sent each one: on Linux
// all eight arrive; on darwin five arrive and those three crash with a fault
// code in the dump rather than the user one. There is no road to those three
// from here, and putting the kernel default back at startup instead would
// silence a *real* fault as well, which is the one case where a Go stack is
// the right thing to print.
//
// Asserted rather than skipped, in both directions, so that the day either
// changes is a failing test rather than one that has been passing vacuously.
// That is how the darwin half above came off: removing the gap turned five of
// these subtests red with a message naming this function.
func reachesTheFrontEnd(sig syscall.Signal) bool {
	if runtime.GOOS != "darwin" {
		return true
	}
	switch sig {
	case syscall.SIGSEGV, syscall.SIGBUS, syscall.SIGILL:
		return false
	}
	return true
}

// A signal the script has trapped is still the script's.
//
// os/signal hands an arrival to every channel registered for it, so a shell
// that traps QUIT has interp's registration and the front end's both told —
// and only one of them may act. Without the question the front end asks first,
// this shell would be killed in the middle of running its own handler, which
// is a worse answer than the goroutine dump it replaced.
func TestATrappedFatalSignalFromOutsideIsTheScriptsToHandle(t *testing.T) {
	dieRunningAsAShell()
	ws, out := shellSignaledFromOutside(t,
		"TestATrappedFatalSignalFromOutsideIsTheScriptsToHandle",
		`trap 'echo caught; exit 7' QUIT; `, countedSleeps, syscall.SIGQUIT)

	if ws.Signaled() {
		t.Fatalf("killed by %v, want the script's own handler to have run", ws.Signal())
	}
	if ws.ExitStatus() != 7 {
		t.Errorf("status %d, want the 7 the handler exits with", ws.ExitStatus())
	}
	if !strings.Contains(out, "caught") {
		t.Errorf("wrote %q, want the handler's line", out)
	}
}

// An `exit` from a handler ends the shell wherever the signal found it, and an
// endless `while` is where it did not.
//
// The handler ran and its line was there in every case, so nothing about the
// delivery was wrong; what was lost was the `exit`, and the only thing a
// caller could see was the status. `while :; do sleep 0.05; done` reported 0
// where bash 5.3.15, bash 3.2.57, that 5.3.15 build named `sh`, dash, ksh93u+
// and zsh 5.9.2 all report 7, while a counted `for` over the same sleeps reported 7 —
// one handler, one signal, two answers depending on the loop it arrived in.
//
// It is asserted here rather than in the corpus only for the external signal.
// The in-process shape — a handler that exits, fired by the loop's own
// `kill -USR1 $$` — is corpus-visible and is pinned there; this is the half
// that needs a second process to send the signal.
func TestAnExitFromATrapEndsAnEndlessLoop(t *testing.T) {
	dieRunningAsAShell()
	ws, out := shellSignaledFromOutside(t,
		"TestAnExitFromATrapEndsAnEndlessLoop",
		`trap 'echo caught; exit 7' USR1; `, endlessLoop, syscall.SIGUSR1)

	if ws.Signaled() {
		t.Fatalf("killed by %v, want the script's own handler to have run", ws.Signal())
	}
	if ws.ExitStatus() != 7 {
		t.Errorf("status %d, want the 7 the handler exits with", ws.ExitStatus())
	}
	if out != "caught\n" {
		t.Errorf("wrote %q, want %q", out, "caught\n")
	}
}

// The two things a shell can be told to hold still with, and each is asking a
// different question.
//
// countedSleeps is a loop of short sleeps rather than one long one, because
// the same number would otherwise be both a floor and a ceiling: a shell told
// to sleep once is only still there if the signal beats the sleep, and a shell
// that has *trapped* the signal runs its handler only once the command it was
// in has finished. A loop keeps it alive for as long as the whole of it while
// giving the handler its turn after one step.
//
// endlessLoop is the same idea with the condition a real script writes, and it
// is the shape #796 was about: an `exit` raised by a handler during
// `while :; do … done` left this shell with 0 where every shell in the panel
// leaves the handler's status. A counted `for` over the same sleeps was right
// throughout, which is what said the fault was in how a loop read its control
// state rather than in how a trap set one. Its `sleep` is short so that the
// handler's turn comes quickly, and the loop only ever ends by the exit under
// test — the harness kills what is left either way.
const (
	countedSleeps = "for i in 1 2 3 4 5 6 7 8 9 10; do sleep 1; done"
	endlessLoop   = "while :; do sleep 0.05; done"
)

// shellSignaledFromOutside re-executes this test binary as a shell, waits for
// it to be running, and sends it a signal from here.
//
// A file rather than a sleep is what says the shell is ready, and it is the
// difference between a test and a guess: the handlers this is about are
// installed while the shell is being built, and a signal that arrives first
// tests the runtime rather than the shell. setup is whatever the script should
// do before it says so.
//
// The wait is measured from the signal rather than from the start, because
// what is being timed is the death and not the second copy of a test binary.
func shellSignaledFromOutside(t *testing.T, name, setup, hold string, sig syscall.Signal) (syscall.WaitStatus, string) {
	t.Helper()
	ready := filepath.Join(t.TempDir(), "ready")
	cmd := exec.Command(os.Args[0], "-test.run="+name)
	cmd.Env = append(os.Environ(), dyingScript+"="+setup+"printf r >"+ready+"; "+hold)
	// A file rather than a buffer, and this is the difference between timing
	// a death and timing a sleep. os/exec copies a non-file stream on a
	// goroutine and Wait does not return until that copy ends — and the copy
	// ends when the last holder of the pipe lets go, which is the `sleep` the
	// shell left behind rather than the shell. Handing it a real file makes
	// the descriptor the child's own and leaves Wait waiting for the process.
	logPath := filepath.Join(filepath.Dir(ready), "out")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the shell half: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	deadline := time.Now().Add(30 * time.Second)
	for {
		if b, err := os.ReadFile(ready); err == nil && len(b) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the shell half never started: %q", readLog(t, logPath))
		}
		time.Sleep(5 * time.Millisecond)
	}

	if err := cmd.Process.Signal(sig); err != nil {
		t.Fatalf("signaling the shell half: %v", err)
	}
	// Waited on with a deadline that fires rather than timed and judged
	// afterwards. The wait itself is the assertion — it returns when the
	// kernel reaps the process, which is the event — and the deadline is here
	// only so that a shell which never dies is reported as one instead of
	// holding the harness open until its own timeout. See deathDeadline.
	reaped := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(reaped)
	}()
	select {
	case <-reaped:
	case <-time.After(deathDeadline):
		_ = cmd.Process.Kill()
		// Waited for even here, so the goroutine above cannot be touching
		// cmd.ProcessState while anything else reads it.
		<-reaped
		t.Fatalf("the shell was still running %v after being sent %v — "+
			"it never died of the signal: %q", deathDeadline, sig, readLog(t, logPath))
	}
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		t.Fatalf("no wait status for the shell half")
	}
	return ws, readLog(t, logPath)
}

// readLog is what the shell half wrote, from the file it wrote it to.
func readLog(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading what the shell half wrote: %v", err)
	}
	return string(b)
}
