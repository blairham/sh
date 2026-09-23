// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// childGroup runs a command that names its own process id and process group,
// and hands both back.
//
// A real command and a real `ps`, because the subject is what the kernel was
// asked for rather than what this shell believes it asked for. `$$` inside the
// inner shell is the child's own pid — the child *is* that shell — so the pair
// says both things a group needs: whether the child leads a group, and whose
// group it is in where it does not.
func childGroup(t *testing.T, monitor, terminal, watched bool) (pid, pgid int) {
	t.Helper()
	out, st := runBoundedScript(t, "sh -c 'ps -o pid=,pgid= -p $$'", nil, func(r *Runner) {
		r.Terminal = terminal
		if watched {
			r.WaitForCommand = waitForTestCommand
		}
		if monitor {
			if code := r.SetOptionLetters("m", true); code != 0 {
				t.Fatalf("set -m: status %d", code)
			}
		}
	})
	if st != 0 {
		t.Fatalf("status %d, want 0: %q", st, out)
	}
	fields := strings.Fields(out)
	if len(fields) != 2 {
		t.Fatalf("ps wrote %q, want a pid and a pgid", out)
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatalf("pid %q: %v", fields[0], err)
	}
	pgid, err = strconv.Atoi(fields[1])
	if err != nil {
		t.Fatalf("pgid %q: %v", fields[1], err)
	}
	return pid, pgid
}

// A foreground command gets a process group of its own only where the monitor
// is on **and** there is a terminal, and is in this shell's group otherwise.
//
// The condition used to be "something is able to notice the command", which in
// the shipped binary is always true — so every child of every script led a
// group of its own. That is a standing a child of a script should not have: a
// signal aimed at the shell's group, which is what a terminal's ^C and what
// `kill -- -$$` both send, then reaches the shell and not the command it is
// waiting on (#4250).
//
// Measured 2026-09-23, `ps -o pid,pgid` in the shell and in a child:
//
//	                      no terminal   terminal, set -m   a prompt
//	bash 5.3.20           the shell's   its own            its own
//	ksh93u+               the shell's   the shell's        its own
//	zsh 5.9.2             the shell's   the shell's        its own
//
// The first column is unanimous and is what this fixes; `set -m` does not move
// it in any of them, bash included, which grants the option there and puts `m`
// in `$-` all the same.
func TestAForegroundCommandLeadsAGroupOnlyWithAMonitorAndATerminal(t *testing.T) {
	ours := syscall.Getpgrp()
	for _, c := range []struct {
		name                       string
		monitor, terminal, watched bool
		own                        bool
	}{
		// The route the issue is about: a script, no terminal anywhere.
		{"a script with nothing watching", false, false, false, false},
		// And a script in the binary, where something *is* watching. This is
		// the row that was wrong: presence of a wait is not job control.
		{"a script the front end waits for", false, false, true, false},
		// The monitor without a terminal is what `set -m` leaves in a
		// pipeline. No panel member gives the child a group there.
		{"the monitor with no terminal", true, false, true, false},
		// A terminal without the monitor is a script run from one, and an
		// interactive shell after `set +m`.
		{"a terminal with the monitor off", false, true, true, false},
		// Both, which is what an interactive shell is.
		{"the monitor and a terminal", true, true, true, true},
		// Both, and nothing to notice the command: there is then nobody to
		// hand the terminal to or to see a stop, so the group buys nothing.
		{"both, with nothing watching", true, true, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			pid, pgid := childGroup(t, c.monitor, c.terminal, c.watched)
			if c.own {
				if pid != pgid {
					t.Errorf("child %d is in group %d, want a group of its own", pid, pgid)
				}
				if pgid == ours {
					t.Errorf("child is in this process's group %d", ours)
				}
				return
			}
			if pgid != ours {
				t.Errorf("child %d is in group %d, want this process's own %d",
					pid, pgid, ours)
			}
		})
	}
}

// The terminal is handed to a command only where the command leads a group.
//
// Two things ride on this rather than one. A `tcsetpgrp` for a process that
// leads no group asks the kernel for a group that is not there; and where the
// command is in this shell's group there is nothing to hand over, because the
// shell's group is already the terminal's foreground one.
func TestTheTerminalGoesOnlyToACommandThatLeadsAGroup(t *testing.T) {
	for _, c := range []struct {
		name              string
		monitor, terminal bool
		want              int
	}{
		{"a script", false, false, 0},
		{"the monitor with no terminal", true, false, 0},
		{"the monitor and a terminal", true, true, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			var mu sync.Mutex
			var handed []int
			out, st := runBoundedScript(t, "sh -c ':'", nil, func(r *Runner) {
				r.Terminal = c.terminal
				r.WaitForCommand = waitForTestCommand
				r.Foreground = func(pgid int) error {
					mu.Lock()
					defer mu.Unlock()
					if pgid != 0 {
						// Zero is the shell taking the terminal back, which
						// happens once for every handing over and is not one.
						handed = append(handed, pgid)
					}
					return nil
				}
				if c.monitor {
					if code := r.SetOptionLetters("m", true); code != 0 {
						t.Fatalf("set -m: status %d", code)
					}
				}
			})
			if st != 0 {
				t.Fatalf("status %d, want 0: %q", st, out)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(handed) != c.want {
				t.Errorf("the terminal was handed over %d times (%v), want %d",
					len(handed), handed, c.want)
			}
		})
	}
}
