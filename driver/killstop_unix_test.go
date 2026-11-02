// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver_test

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/internal/testenv"
)

// A shell that aims a stop at itself is stopped by that very signal, and only
// the front end can do it.
//
// interp hands the stop to Runner.StopThisProcess and makes no system call of
// its own (#7040): a library Runner must not suspend the program embedding
// it. So what is left to prove is the other half — that a binary that *is* a
// shell still stops, and stops by the signal the script named. That half is
// not a formality. The keyboard's three stops are discarded by the kernel for
// an orphaned process group and SIGSTOP never is, so a front end that stopped
// itself with SIGSTOP for `kill -TSTP $$` would hang a script that bash 5.3,
// bash 3.2, ksh93, zsh and dash all run to the end (measured 2026-10-29).
//
// Each shell is a re-execution of this binary in a process group of its own,
// so the stop reaches nothing else, and it is continued from here.
func TestAStopAimedAtTheShellStopsTheProcessWithIt(t *testing.T) {
	dieRunningAsAShell()
	for _, tc := range []struct {
		src  string
		stop syscall.Signal
	}{
		{`kill -TSTP $$; echo "alive=$?"`, syscall.SIGTSTP},
		{`kill -TTIN $$; echo "alive=$?"`, syscall.SIGTTIN},
		{`kill -s TTOU $$; echo "alive=$?"`, syscall.SIGTTOU},
		{`kill -STOP $$; echo "alive=$?"`, syscall.SIGSTOP},
		// The control: an ignore is nothing, so this one never stops, and a
		// harness that reported every shell as stopped would fail here.
		{`trap '' TSTP; kill -TSTP $$; echo "alive=$?"`, 0},
	} {
		t.Run(tc.src, func(t *testing.T) {
			stoppedBy, out := stopsOfAShell(t, tc.src)
			if stoppedBy != tc.stop {
				t.Errorf("stopped by %v, want %v", stoppedBy, tc.stop)
			}
			if want := "alive=0\n"; out != want {
				t.Errorf("wrote %q, want %q", firstLine(out), want)
			}
		})
	}
}

// stopsOfAShell runs src as a shell, continues it if it stops, and reports the
// signal that stopped it (0 if nothing did) and what it wrote.
func stopsOfAShell(t *testing.T, src string) (syscall.Signal, string) {
	t.Helper()
	outFile, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer outFile.Close()
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()
	self := []string{os.Args[0], "-test.run=^TestAStopAimedAtTheShellStopsTheProcessWithIt$"}
	proc, err := os.StartProcess(os.Args[0], self, &os.ProcAttr{
		Env:   testenv.WithoutRaceExitPause(append(os.Environ(), dyingScript+"="+src)),
		Files: []*os.File{devnull, outFile, outFile},
		Sys:   &syscall.SysProcAttr{Setpgid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	type waited struct {
		ws  syscall.WaitStatus
		err error
	}
	wait := func(options int) waited {
		ch := make(chan waited, 1)
		go func() {
			var ws syscall.WaitStatus
			_, err := syscall.Wait4(proc.Pid, &ws, options, nil)
			ch <- waited{ws, err}
		}()
		select {
		case w := <-ch:
			return w
		case <-time.After(deathDeadline):
			_ = syscall.Kill(proc.Pid, syscall.SIGKILL)
			t.Fatalf("%s: neither stopped nor finished within %v", src, deathDeadline)
			return waited{}
		}
	}
	var stoppedBy syscall.Signal
	w := wait(syscall.WUNTRACED)
	if w.err != nil {
		t.Fatalf("wait: %v", w.err)
	}
	// Decoded by hand rather than through WaitStatus.Stopped, which on darwin
	// answers false for a process stopped by SIGSTOP — the one row here that
	// most needs the answer.
	if w.ws&0x7f == 0x7f {
		stoppedBy = syscall.Signal((w.ws >> 8) & 0xff)
		// Stopped in the kill, as a shell with one thread is: nothing after
		// it has run yet. A stop that landed late, on some other thread of
		// this Go program, let the echo through first (#7290).
		if early, err := os.ReadFile(outFile.Name()); err != nil {
			t.Fatal(err)
		} else if len(early) > 0 {
			t.Errorf("%s: wrote %q before it stopped; the stop is taken in the kill", src, early)
		}
		if err := syscall.Kill(proc.Pid, syscall.SIGCONT); err != nil {
			t.Fatalf("continue: %v", err)
		}
		if w = wait(0); w.err != nil {
			t.Fatalf("wait: %v", w.err)
		}
	}
	if !w.ws.Exited() || w.ws.ExitStatus() != 0 {
		t.Errorf("%s: ended %v, want an exit at 0", src, w.ws)
	}
	data, err := os.ReadFile(outFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	return stoppedBy, strings.ReplaceAll(string(data), "\r", "")
}
