// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver_test

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
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
// shell that parked forever — so this is loose enough that a loaded runner
// cannot fail it and tight enough that a hang is reported as a hang rather
// than as whatever the enclosing timeout eventually says.
//
// It is also deliberately under the raise's own grace period, so a shell that
// gave up and exited with a number is caught by this as well as by the wait
// status.
const deathBudget = 2 * time.Second

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
	os.Exit(driver.MainArgs(shell(), []string{"testsh", "-c", src}))
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
