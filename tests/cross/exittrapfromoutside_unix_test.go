// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package cross_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
)

// outsideDeathShell names the dialect a re-executed copy of this test binary
// runs as, and outsideDeathScript the script it runs: set, it means "you are
// the shell".
const (
	outsideDeathShell  = "SH_TEST_OUTSIDE_DEATH_SHELL"
	outsideDeathScript = "SH_TEST_OUTSIDE_DEATH_SCRIPT"
)

// What each dialect does with its EXIT trap when another process sends it an
// untrapped fatal signal, through the front end that owns the process (#7054).
//
// Measured 2026-10-29, each shell in a process group of its own, `trap 'echo
// "bye $?"' EXIT; (exit 7)` and then an external `sleep`, a `read` on a pipe or
// a busy loop, and `/bin/kill` from another process:
//
//	                 TERM                  HUP
//	bash 5.3, 3.2    bye 7, killed by it   bye 7, killed by it
//	ksh93u+          bye 7, killed by it   bye 7, killed by it
//	zsh 5.9.2        killed, nothing       bye 1, exit 1
//	dash             killed, nothing       killed, nothing
//	BusyBox ash      killed, nothing       killed, nothing
//
// BusyBox ash is the pinned alpine image internal/oracle reaches, the shell
// started under `setsid` with the container under `--init`, so that it is not
// PID 1 and its default dispositions stand. It is ExitTrapRunsOnSignalDeath
// and HangupIsAnOrderlyExit reached from outside: every column answers as it
// does for a signal the script sends itself.
func TestADialectsExitTrapAnswersAFatalSignalFromOutside(t *testing.T) {
	if name := os.Getenv(outsideDeathShell); name != "" {
		sh := echoShells()[name]
		os.Exit(driver.MainArgs(sh, []string{sh.Name, "-c", os.Getenv(outsideDeathScript)}))
	}
	type ending struct {
		// sig is the signal the shell must die of, or 0 for an exit at
		// status.
		sig    syscall.Signal
		status int
		out    string
	}
	killed := func(sig syscall.Signal, out string) ending { return ending{sig: sig, out: out} }
	for _, c := range []struct {
		name string
		sig  syscall.Signal
		want map[string]ending
	}{
		{"TERM", syscall.SIGTERM, map[string]ending{
			"bash": killed(syscall.SIGTERM, "bye 7\n"),
			"ksh":  killed(syscall.SIGTERM, "bye 7\n"),
			"zsh":  killed(syscall.SIGTERM, ""),
			"dash": killed(syscall.SIGTERM, ""),
			"ash":  killed(syscall.SIGTERM, ""),
		}},
		{"HUP", syscall.SIGHUP, map[string]ending{
			"bash": killed(syscall.SIGHUP, "bye 7\n"),
			"ksh":  killed(syscall.SIGHUP, "bye 7\n"),
			"zsh":  {status: 1, out: "bye 1\n"},
			"dash": killed(syscall.SIGHUP, ""),
			"ash":  killed(syscall.SIGHUP, ""),
		}},
	} {
		for name, want := range c.want {
			t.Run(name+"/"+c.name, func(t *testing.T) {
				ws, out := diesFromOutside(t, name, c.sig)
				switch {
				case want.sig != 0 && (!ws.Signaled() || ws.Signal() != want.sig):
					t.Errorf("ended %v, want killed by %v", ws, want.sig)
				case want.sig == 0 && (ws.Signaled() || ws.ExitStatus() != want.status):
					t.Errorf("ended %v, want an exit at %d", ws, want.status)
				}
				if out != want.out {
					t.Errorf("wrote %q, want %q", out, want.out)
				}
			})
		}
	}
}

// diesFromOutside runs this test binary as the dialect's shell in a process
// group of its own, waiting on a program that says it is ready once it runs,
// sends it sig from here, and reports how it ended and what it wrote.
func diesFromOutside(t *testing.T, dialect string, sig syscall.Signal) (syscall.WaitStatus, string) {
	t.Helper()
	const deadline = 30 * time.Second
	dir := t.TempDir()
	ready, logPath := filepath.Join(dir, "ready"), filepath.Join(dir, "out")
	script := `trap 'echo "bye $?"' EXIT; (exit 7); /bin/sh -c 'printf r >"$1"; exec sleep 30' sh "$READY"; echo after`
	cmd := exec.Command(os.Args[0], "-test.run=^TestADialectsExitTrapAnswersAFatalSignalFromOutside$")
	cmd.Env = append(os.Environ(), outsideDeathShell+"="+dialect, outsideDeathScript+"="+script, "READY="+ready)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// The whole group, so the `sleep` the shell leaves behind goes too.
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
	reaped := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(reaped)
	}()
	readLog := func() string {
		b, _ := os.ReadFile(logPath)
		return string(b)
	}
	until := time.Now().Add(deadline)
	for {
		if b, err := os.ReadFile(ready); err == nil && len(b) > 0 {
			break
		}
		select {
		case <-reaped:
			t.Fatalf("the shell ended before it was ready: %q", readLog())
		default:
		}
		if time.Now().After(until) {
			t.Fatalf("the shell never started its program: %q", readLog())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := cmd.Process.Signal(sig); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reaped:
	case <-time.After(deadline):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-reaped
		t.Fatalf("the shell was still running %v after %v: %q", deadline, sig, readLog())
	}
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		t.Fatal("no wait status")
	}
	return ws, readLog()
}

// What each dialect does with an `exit` in the EXIT trap of a death, and with
// a fatal signal that lands while the EXIT trap runs (#7063, #7064).
//
// Measured 2026-10-30, each shell in a process group of its own:
//
//	                 trap 'echo bye; exit 5' EXIT;   trap 'echo bye; kill -USR1 $$;
//	                 kill -TERM $$                   echo after' EXIT; exit 3
//	bash 5.3, 3.2    bye, killed by TERM             bye, killed by USR1
//	ksh93u+          bye, exit 5                     bye, exit 3
//	zsh 5.9.2        killed by TERM, nothing         bye, killed by USR1
//	dash             killed by TERM, nothing         bye, killed by USR1
//	BusyBox ash      killed by TERM, nothing         bye, killed by USR1
//
// BusyBox ash is the pinned alpine image internal/oracle reaches, started
// under `--init` so that it is not PID 1. Both are the dialect answering
// Semantics.ExitInTheExitTrapOverridesASignalDeath and
// Semantics.FatalSignalInTheExitTrapEndsOnlyTheTrap, read here through each
// preset and the front end that owns the process.
func TestADialectsExitTrapDecidesWhetherASignalEndsTheShell(t *testing.T) {
	type ending struct {
		sig    syscall.Signal
		status int
		out    string
	}
	killed := func(sig syscall.Signal, out string) ending { return ending{sig: sig, out: out} }
	const (
		exitInTheTrap   = `trap 'echo bye; exit 5' EXIT; kill -TERM $$; echo after`
		signalInTheTrap = `trap 'echo bye; kill -USR1 $$; echo after' EXIT; exit 3`
	)
	for _, c := range []struct {
		name, script string
		want         map[string]ending
	}{
		{"an exit in the trap of a death", exitInTheTrap, map[string]ending{
			"bash": killed(syscall.SIGTERM, "bye\n"),
			"ksh":  {status: 5, out: "bye\n"},
			"zsh":  killed(syscall.SIGTERM, ""),
			"dash": killed(syscall.SIGTERM, ""),
			"ash":  killed(syscall.SIGTERM, ""),
		}},
		{"a fatal signal in the trap of an exit", signalInTheTrap, map[string]ending{
			"bash": killed(syscall.SIGUSR1, "bye\n"),
			"ksh":  {status: 3, out: "bye\n"},
			"zsh":  killed(syscall.SIGUSR1, "bye\n"),
			"dash": killed(syscall.SIGUSR1, "bye\n"),
			"ash":  killed(syscall.SIGUSR1, "bye\n"),
		}},
	} {
		for name, want := range c.want {
			t.Run(name+"/"+c.name, func(t *testing.T) {
				ws, out := endsOnItsOwn(t, name, c.script)
				switch {
				case want.sig != 0 && (!ws.Signaled() || ws.Signal() != want.sig):
					t.Errorf("ended %v, want killed by %v", ws, want.sig)
				case want.sig == 0 && (ws.Signaled() || ws.ExitStatus() != want.status):
					t.Errorf("ended %v, want an exit at %d", ws, want.status)
				}
				if out != want.out {
					t.Errorf("wrote %q, want %q", out, want.out)
				}
			})
		}
	}
}

// endsOnItsOwn runs this test binary as the dialect's shell running script, in
// a process group of its own, and reports how it ended and what it wrote.
func endsOnItsOwn(t *testing.T, dialect, script string) (syscall.WaitStatus, string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "out")
	cmd := exec.Command(os.Args[0], "-test.run=^TestADialectsExitTrapAnswersAFatalSignalFromOutside$")
	cmd.Env = append(os.Environ(), outsideDeathShell+"="+dialect, outsideDeathScript+"="+script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
	reaped := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(reaped)
	}()
	select {
	case <-reaped:
	case <-time.After(30 * time.Second):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-reaped
		t.Fatal("the shell was still running after 30s")
	}
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		t.Fatal("no wait status")
	}
	b, _ := os.ReadFile(logPath)
	return ws, string(b)
}

// What each dialect's EXIT trap reads after the shell sent itself a fatal
// signal with `kill` (#7062).
//
// Measured 2026-10-30, each shell in a process group of its own, `trap 'echo
// "bye $?"' EXIT; (exit 7)` and then the line:
//
//	                       kill     ! kill   f(){ kill; }   ( kill )
//	bash 5.3, 3.2, sh      bye 0    bye 1    bye 0          bye 7
//	ksh93u+                bye 7    bye 7    bye 271        bye 271
//	zsh 5.9.2, dash        killed by TERM, nothing written, in all four
//
// BusyBox ash runs no EXIT trap on a death either — its preset's measurement.
// Every one of them dies of TERM. Semantics.SelfAimedDeathIsTakenAtTheNextCommand
// and Semantics.DeathInAFunctionEndsTheCallAsKilled, read here through each
// preset and the front end that owns the process.
func TestADialectsExitTrapReadsWhatItsOwnKillLeft(t *testing.T) {
	const trap = `trap 'echo "bye $?"' EXIT; (exit 7); `
	quiet := map[string]string{"zsh": "", "dash": "", "ash": ""}
	for _, c := range []struct {
		name, script string
		bash, ksh    string
	}{
		{"kill", "kill -TERM $$; echo after", "bye 0\n", "bye 7\n"},
		{"a negated kill", "! kill -TERM $$; echo after", "bye 1\n", "bye 7\n"},
		{"a function", "f() { (exit 7); kill -TERM $$; echo inf; }; f; echo after", "bye 0\n", "bye 271\n"},
		{"a subshell", "(kill -TERM $$); echo after", "bye 7\n", "bye 271\n"},
	} {
		want := map[string]string{"bash": c.bash, "ksh": c.ksh}
		for name, out := range quiet {
			want[name] = out
		}
		for name, out := range want {
			t.Run(name+"/"+c.name, func(t *testing.T) {
				ws, got := endsOnItsOwn(t, name, trap+c.script)
				if !ws.Signaled() || ws.Signal() != syscall.SIGTERM {
					t.Errorf("ended %v, want killed by %v", ws, syscall.SIGTERM)
				}
				if got != out {
					t.Errorf("wrote %q, want %q", got, out)
				}
			})
		}
	}
}
