// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"errors"
	"sync"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A process substitution's body must not hand the terminal to its child.
//
// # What the terminal has to do with a substitution
//
// The shell gives the terminal to a command it is *waiting for*, so that ^C
// and ^Z reach the command rather than the shell, and takes it back when the
// command ends. A substitution's body is not a command the shell waits for: it
// runs beside the command that named it, on a goroutine, while the shell
// carries on. So the hook that hands the terminal over has no business being
// reachable from one, and while it was, the body handed the terminal to its
// child and held it there for as long as that child lived.
//
// # What it cost
//
// A session. Measured 2026-09-10 through a pseudo-terminal, with the shell in
// a session of its own and this in the startup file:
//
//	exec {wfd}< <(sleep 60)
//
// the first read after the prompt failed with
// `read /dev/stdin: input/output error` and the shell exited before a single
// line could be typed. That is what a shell gets for reading its terminal from
// a background process group: the kernel sends SIGTTIN, a shell ignores
// SIGTTIN, and an ignored SIGTTIN turns the read into EIO. Real zsh 5.9.2
// through the same driver answers every line typed and never fires the
// watcher, which is what this now does too.
//
// It is #1759, which saw it from a `zle -F` watcher and found the watcher
// blameless: the substitution alone reproduces it, with nothing else in the
// startup file. A body whose command finishes at once — `<(echo hi)` — hides
// it, because the terminal is back before the prompt is read.
//
// # Why the assertion is the hook and not a terminal
//
// A test process has no session of its own to lose, and putting one on a
// pseudo-terminal would be measuring the pseudo-terminal. The hook is where
// the decision is: Runner.Foreground is the only way this package can reach a
// terminal at all, so a body that never calls it is a body that cannot take
// one. `read` and `printf` are builtins, so the *only* thing in this script
// that could reach the hook is the substitution's own `/bin/echo` — and the
// output is asserted as well, because a hook never called by a body that
// never ran would be evidence about nothing.
func TestASubstitutionsBodyNeverTakesTheTerminal(t *testing.T) {
	var mu sync.Mutex
	var handed []int
	out, st := runBoundedScript(t, "read line < <(/bin/echo hi)\nprintf %s \"$line\"", nil, func(r *Runner) {
		// The wait a shell with job control does: it is what puts an external
		// command down the path that offers the terminal, so without it this
		// test could not see the bug even with the fix reverted.
		r.WaitForCommand = waitForTestCommand
		r.Foreground = func(pgid int) error {
			mu.Lock()
			defer mu.Unlock()
			handed = append(handed, pgid)
			return nil
		}
	})
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
	if out != "hi" {
		t.Fatalf("out = %q, want %q — the substitution's body did not run, so "+
			"nothing here is evidence about what it did with the terminal", out, "hi")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(handed) != 0 {
		t.Errorf("the terminal was handed to %v — a substitution's body runs beside "+
			"the shell, so a shell that gives it the terminal is reading its own "+
			"terminal from a background process group", handed)
	}
}

// waitForTestCommand is the shell's own wait, as driver supplies it: the only
// kind that can report a command that stopped, and the field whose presence
// decides whether a command is offered the terminal at all.
func waitForTestCommand(pid int) (Wait, error) {
	for {
		var ws syscall.WaitStatus
		reaped, err := syscall.Wait4(pid, &ws, syscall.WUNTRACED, nil)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return Wait{}, err
		}
		if reaped == 0 {
			return Wait{}, nil
		}
		return Wait{
			Status:  ws.ExitStatus(),
			Signal:  ws.StopSignal(),
			Stopped: ws.Stopped(),
			Killed:  ws.Signaled(),
		}, nil
	}
}
