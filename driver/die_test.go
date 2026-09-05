// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver_test

import (
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

// deathBudget is how long a shell that is going to be killed by a signal is
// given to be killed by it.
//
// It is not a performance assertion. The measured time is tens of
// milliseconds on both platforms, and the defect this guards against was a
// shell that parked forever — so it only has to be tight enough that a hang
// is reported as a hang rather than as whatever the enclosing timeout
// eventually says.
//
// It was two seconds, and a loaded ubuntu runner under -race missed it by
// five milliseconds (2.0048s), which failed unrelated pull requests until
// somebody read the number. Forking and starting a second copy of the test
// binary is most of the measurement and is exactly what a busy machine
// stretches, so the budget is set by what a stall costs rather than by what
// the work costs.
//
// It is still deliberately under the raise's own grace period (deathGrace),
// so a shell that gave up and exited with a number is caught by this as well
// as by the wait status.
const deathBudget = 4 * time.Second

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
// Not tidiness. SIGABRT is a core-dumping signal on Linux where it is not on
// macOS, and a race-instrumented test binary is large: measured on a CI runner,
// the kernel spent 1.5 seconds writing the image before reaping the process, so
// the timing this file is here to measure was the core writer's rather than the
// shell's. The failure looked platform-specific and was a matter of what the
// default action *does* on each one.
func writeNoCore() {
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_CORE, &lim); err != nil {
		return
	}
	lim.Cur = 0
	_ = syscall.Setrlimit(syscall.RLIMIT_CORE, &lim)
}

// waitStatusOfAShell re-executes this test binary as a shell running src and
// reports how it ended, how long that took, and what it wrote.
func waitStatusOfAShell(t *testing.T, name, src string) (syscall.WaitStatus, time.Duration, string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run="+name)
	cmd.Env = append(os.Environ(), dyingScript+"="+src)
	start := time.Now()
	out, _ := cmd.CombinedOutput()
	took := time.Since(start)
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		t.Fatalf("no wait status for the shell half")
	}
	return ws, took, string(out)
}

// killedBy asserts the shape every one of these cases wants: the shell was
// killed by the signal it raised, it did not merely exit with a number it
// computed, it said nothing, and it did it promptly.
func killedBy(t *testing.T, name, src string, want syscall.Signal) {
	t.Helper()
	ws, took, out := waitStatusOfAShell(t, name, src)
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
	if took > deathBudget {
		t.Errorf("%s: took %v to die, budget %v", src, took, deathBudget)
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
			ws, took, out := shellSignaledFromOutside(t,
				"TestAFatalSignalFromOutsideKillsTheShellQuietly", "", sig)

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
			if took > deathBudget {
				t.Errorf("took %v to die, budget %v", took, deathBudget)
			}
		})
	}
}

// reachesTheFrontEnd reports whether a signal *sent* to this process is handed
// to the front end rather than left to the Go runtime.
//
// Two facts, and the platforms split on both.
//
// The runtime lets a sent signal through to os/signal for the throwing class
// only when the kernel marks it as one a process sent, and macOS does not do
// that for SEGV, BUS or ILL. Measured with a program that registers all eight
// and is then sent each one: on Linux all eight arrive; on darwin five arrive
// and those three crash with a fault code in the dump rather than the user
// one. There is no road to those three from here, and putting the kernel
// default back at startup instead would silence a *real* fault as well, which
// is the one case where a Go stack is the right thing to print.
//
// And on darwin the front end does not listen at all — see
// fatalsignal_darwin.go, where asking os/signal for a signal costs the
// descriptors an `exec` then writes over. So the whole class is the runtime's
// there.
//
// Asserted rather than skipped, in both directions, so that the day either
// changes is a failing test rather than one that has been passing vacuously.
func reachesTheFrontEnd(syscall.Signal) bool { return runtime.GOOS != "darwin" }

// A signal the script has trapped is still the script's.
//
// os/signal hands an arrival to every channel registered for it, so a shell
// that traps QUIT has interp's registration and the front end's both told —
// and only one of them may act. Without the question the front end asks first,
// this shell would be killed in the middle of running its own handler, which
// is a worse answer than the goroutine dump it replaced.
func TestATrappedFatalSignalFromOutsideIsTheScriptsToHandle(t *testing.T) {
	dieRunningAsAShell()
	ws, _, out := shellSignaledFromOutside(t,
		"TestATrappedFatalSignalFromOutsideIsTheScriptsToHandle",
		`trap 'echo caught; exit 7' QUIT; `, syscall.SIGQUIT)

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
func shellSignaledFromOutside(t *testing.T, name, setup string, sig syscall.Signal) (syscall.WaitStatus, time.Duration, string) {
	t.Helper()
	ready := filepath.Join(t.TempDir(), "ready")
	cmd := exec.Command(os.Args[0], "-test.run="+name)
	// A loop of short sleeps rather than one long one, because the same
	// number would otherwise be both a floor and a ceiling: a shell told to
	// sleep once is only still there if the signal beats the sleep, and a
	// shell that has *trapped* the signal runs its handler only once the
	// command it was in has finished. A loop keeps it alive for as long as
	// the whole of it while giving the handler its turn after one step.
	//
	// Counted rather than endless, and that is not caution: `exit` from a
	// trap fired inside `while :; do … done` leaves this shell with a status
	// of 0 where bash leaves 7, which is #796 and is not what this is
	// testing.
	cmd.Env = append(os.Environ(), dyingScript+"="+setup+"printf r >"+ready+
		"; for i in 1 2 3 4 5 6 7 8 9 10; do sleep 1; done")
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

	start := time.Now()
	if err := cmd.Process.Signal(sig); err != nil {
		t.Fatalf("signaling the shell half: %v", err)
	}
	_ = cmd.Wait()
	took := time.Since(start)
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		t.Fatalf("no wait status for the shell half")
	}
	return ws, took, readLog(t, logPath)
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
