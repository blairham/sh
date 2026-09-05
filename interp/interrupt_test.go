// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"io"
	"strings"
	"sync"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// ticker is a builtin that prints `tick` and, once it has printed enough of
// them, arms the interrupt — so an interrupt lands *inside* the construct
// under test whatever shape it is, rather than after a command count that
// differs from one shape to the next.
type ticker struct {
	mu      sync.Mutex
	printed int
	after   int
	armed   bool
}

func (k *ticker) register(r *Runner) {
	r.Register("tick", func(r *Runner, _ context.Context, _ []string) int {
		k.mu.Lock()
		k.printed++
		reached := k.printed == k.after
		if reached {
			k.armed = true
		}
		k.mu.Unlock()
		// Written after the lock is dropped, and to the Runner's own stream:
		// a background job holds a guarded copy of it, which is what makes
		// two of these safe to run at once.
		_, _ = io.WriteString(r.Out(), "tick\n")
		return 0
	})
	r.TakeInterrupt = func() bool {
		k.mu.Lock()
		defer k.mu.Unlock()
		if !k.armed {
			return false
		}
		k.armed = false
		return true
	}
}

// A loop the shell runs itself is stopped by an interrupt, and so is the rest
// of the line it was on.
//
// This is the shape that had no way out at all: nothing in `while :; do echo
// tick; done` blocks, waits or returns to the front end, so the only place the
// answer can be read is the top of each command — and until it was read there,
// a runaway loop typed at a prompt could be ended only by closing the
// terminal.
//
// Measured through a pseudo-terminal on 2026-09-05 and unanimous between bash
// and zsh: the loop ends, the commands after it on the same line do not run,
// and the status is 128 plus SIGINT.
func TestAnInterruptGivesUpTheLine(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"a loop of the shell's own commands", "while :; do tick; done; echo after"},
		{"a loop over a list", "for i in 1 2 3 4 5 6 7 8; do tick; done; echo after"},
		{"a C-style loop", "for ((i = 0; i < 8; i++)); do tick; done; echo after"},
		{"nested loops", "while :; do while :; do tick; done; done; echo after"},
		{"a function body", "f() { tick; tick; tick; }; f; echo after"},
		{"a plain list", "tick; tick; tick; echo after"},
		{"a group", "{ tick; tick; tick; }; echo after"},
		{"an if", "if tick; then tick; tick; fi; echo after"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Interrupted after the second tick, so a construct that
			// stopped has printed exactly two and one that did not has
			// printed more — and either way `after` says whether the rest
			// of the line survived.
			out, st, _ := interruptedRun(t, tc.src, 2)
			if strings.Contains(out, "after") {
				t.Errorf("printed %q, want the rest of the line given up", out)
			}
			if got := strings.Count(out, "tick"); got != 2 {
				t.Errorf("printed %q — %d ticks, want the construct stopped at the second", out, got)
			}
			if want := 128 + int(syscall.SIGINT); st != want {
				t.Errorf("status = %d, want %d", st, want)
			}
		})
	}
}

// And the shell is still there afterwards: giving up the line is not giving up
// the session, which is the whole difference between this and an exit.
func TestTheShellSurvivesAnInterrupt(t *testing.T) {
	_, _, r := interruptedRun(t, "while :; do tick; done", 2)
	if r.Exited() {
		t.Fatal("the session ended, want only the line given up")
	}
	out, st, _ := jobRun2(t, r, "echo alive")
	if !strings.Contains(out, "alive") || st != 0 {
		t.Errorf("the next line gave %q status %d, want it to run", out, st)
	}
}

// A shell that is not a session is not interrupted by somebody typing: there
// is nobody typing. What a script does with a signal is the signal's own
// business and a different question.
func TestAScriptIsNotInterruptedByTheKeyboard(t *testing.T) {
	out, _, _ := interruptedRunIn(t, "for i in 1 2 3; do tick; done; echo after", 2, false)
	if strings.Count(out, "tick") != 3 || !strings.Contains(out, "after") {
		t.Errorf("printed %q, want the whole line to have run", out)
	}
}

// With no front end to ask, nothing is interrupted — which is what a Runner
// embedded in another program is, and what it was before any of this.
func TestWithoutAFrontEndNothingIsInterrupted(t *testing.T) {
	out, st, _ := interruptedRunIn(t, "for i in 1 2 3; do tick; done; echo after", 0, true)
	if strings.Count(out, "tick") != 3 || !strings.Contains(out, "after") {
		t.Errorf("printed %q, want the whole line to have run", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// A command the interrupt reached instead of the shell ends the line the same
// way. The terminal belonged to the command's process group, so no signal
// arrived here at all and what it died of is the only evidence there is.
func TestACommandKilledByAnInterruptGivesUpTheLine(t *testing.T) {
	f := &fakeJobs{waits: []Wait{{Signal: syscall.SIGINT, Killed: true}}}
	out, st, _ := jobSession(t, f, echoCmd+"; echo after", true, func(*Semantics, *Diagnostics) {})
	if strings.Contains(out, "after") {
		t.Errorf("printed %q, want the rest of the line given up", out)
	}
	if want := 128 + int(syscall.SIGINT); st != want {
		t.Errorf("status = %d, want %d", st, want)
	}
}

// A command killed by something else does not: only an interrupt is somebody
// asking the shell to stop.
func TestACommandKilledBySomethingElseDoesNot(t *testing.T) {
	f := &fakeJobs{waits: []Wait{{Signal: syscall.SIGTERM, Killed: true}}}
	out, _, _ := jobSession(t, f, echoCmd+"; echo after", true, func(*Semantics, *Diagnostics) {})
	if !strings.Contains(out, "after") {
		t.Errorf("printed %q, want the line to have gone on", out)
	}
}

// A job put back in front is a foreground command again, so an interrupt ends
// its line the same way — and says what ended it, which is what the prompt
// reads to start on a line of its own. `fg` had neither: it forgot the job and
// reported a status, and the next prompt landed on the `^C`.
func TestAnInterruptedFgGivesUpTheLine(t *testing.T) {
	f := &fakeJobs{waits: []Wait{stopped, {Signal: syscall.SIGINT, Killed: true}}}
	out, st, r := jobSession(t, f, echoCmd+"\nfg; echo after", true, func(*Semantics, *Diagnostics) {})
	if strings.Contains(out, "after") {
		t.Errorf("printed %q, want the rest of the line given up", out)
	}
	if want := 128 + int(syscall.SIGINT); st != want {
		t.Errorf("status = %d, want %d", st, want)
	}
	if !r.LastCommandWasInterrupted() {
		t.Error("the shell does not know the line was interrupted, so the prompt lands on the ^C")
	}
}

// A stop ends the loops it is inside and nothing more.
//
// Measured in bash: ^Z inside a loop ends the loop and the commands after it
// on the same line still run, while ^Z in a plain list interrupts nothing at
// all. Both halves are here, because a rule that ended the line would get the
// first right and the second wrong.
func TestAStopEndsTheLoopsItIsInside(t *testing.T) {
	for _, tc := range []struct {
		name  string
		src   string
		ticks int
	}{
		{"inside a loop", "for i in 1 2 3; do echo tick; " + echoCmd + "; done; echo after", 1},
		{"inside nested loops", "for i in 1 2; do for j in 1 2; do echo tick; " + echoCmd + "; done; done; echo after", 1},
		{"inside a while loop", "i=0; while [ $i -lt 3 ]; do i=$((i+1)); echo tick; " + echoCmd + "; done; echo after", 1},
		{"inside a C-style loop", "for ((i = 0; i < 3; i++)); do echo tick; " + echoCmd + "; done; echo after", 1},
		{"not in a loop at all", "echo tick; " + echoCmd + "; echo tick; echo after", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeJobs{waits: []Wait{stopped}}
			out, _, _ := jobSession(t, f, tc.src, true, func(*Semantics, *Diagnostics) {})
			if got := strings.Count(out, "tick"); got != tc.ticks {
				t.Errorf("printed %q — %d ticks, want %d", out, got, tc.ticks)
			}
			// The line goes on in every shape: a stop is not an interrupt.
			if !strings.Contains(out, "after") {
				t.Errorf("printed %q, want the rest of the line to have run", out)
			}
		})
	}
}

// A background job runs on a clone of this Runner, and must not answer the
// question the foreground is about to ask: an interrupt taken there is one the
// person typing never sees, and the line they meant to stop carries on.
//
// `wait` is what makes this a fact rather than a race. The job's second
// command asks while the shell is blocked, so a clone that answered has
// answered by the time the foreground looks.
func TestABackgroundJobDoesNotTakeTheInterrupt(t *testing.T) {
	out, _, _ := interruptedRun(t, "{ tick; :; } & wait; tick; echo after", 1)
	if strings.Contains(out, "after") {
		t.Errorf("printed %q — the background job took the interrupt", out)
	}
	if got := strings.Count(out, "tick"); got != 1 {
		t.Errorf("printed %q — %d ticks, want the one the job printed", out, got)
	}
}

// An interrupt taken at the *first* command of a line drops the rest of it
// too.
//
// Giving up a line means naming the line to give up, and the number is
// recorded as each command is reached — so an interrupt read before that
// happened named line zero, matched no statement, and let everything after it
// run as though nothing had happened. Found by the background-job test above,
// which is the only shape where the arrival lands that early.
func TestAnInterruptAtTheFirstCommandDropsTheRestOfTheLine(t *testing.T) {
	f := &fakeJobs{waits: []Wait{{Status: 0}}}
	out, st, _ := jobSessionWith(t, f, "echo one; echo after", true, func(r *Runner) {
		asked := false
		r.TakeInterrupt = func() bool {
			if asked {
				return false
			}
			asked = true
			return true
		}
	})
	if out != "" {
		t.Errorf("printed %q, want nothing at all", out)
	}
	if want := 128 + int(syscall.SIGINT); st != want {
		t.Errorf("status = %d, want %d", st, want)
	}
}

// interruptedRun runs a line in a session that is interrupted after the nth
// tick.
func interruptedRun(t *testing.T, src string, n int) (string, int, *Runner) {
	t.Helper()
	return interruptedRunIn(t, src, n, true)
}

// interruptedRunIn is the same with the session's own answer to whether there
// is anybody at the keyboard. n of zero is a front end that was never wired
// up, which is what an embedded Runner is.
func interruptedRunIn(t *testing.T, src string, n int, interactive bool) (string, int, *Runner) {
	t.Helper()
	f := &fakeJobs{waits: []Wait{{Status: 0}}}
	return jobSessionWith(t, f, src, interactive, func(r *Runner) {
		k := &ticker{after: n}
		k.register(r)
		if n == 0 {
			r.TakeInterrupt = nil
		}
	})
}
