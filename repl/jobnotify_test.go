// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// The wake is opened for the session that will use it and for no other.
//
// Two properties in one table, and the second is the one worth stating: the
// four dialects that hold a finished job's notice until the next prompt are
// answered with a negative descriptor, so fdset.Wait is handed an empty set
// and returns without a system call. That is watchfd.go's standing promise —
// "with nothing to watch, a keystroke is read exactly the way it was read
// before this file existed" — and it is what this change had to keep.
//
// The arming is deliberately *not* decided once at the start: `unsetopt
// notify` in a startup file and `setopt notify` typed at the prompt afterwards
// both have to work, so the question is asked each time round the wait. The
// last two rows are that, asked of one wake (#4524).
func TestTheJobWakeIsArmedOnlyWhereTheDialectReportsAtOnce(t *testing.T) {
	for _, tc := range []struct {
		name    string
		atOnce  interp.Answer
		control bool
		armed   bool
	}{
		{"a dialect that reports at once", interp.Yes, true, true},
		{"a dialect that waits for the prompt", interp.No, true, false},
		{"an axis nobody answered", interp.Unspecified, true, false},
		{"a session with nobody to tell", interp.Yes, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := notifyingRunner(t, tc.atOnce)
			r.JobControl = tc.control
			s := Shell{Runner: r, Out: &strings.Builder{}, Err: &strings.Builder{}}
			wake, stop := s.jobNotifying()
			defer stop()
			if !tc.control {
				if wake != nil {
					t.Fatalf("a session with no job control was given a wake")
				}
				if r.JobEnded != nil {
					t.Errorf("JobEnded was wired for a session with nobody to tell")
				}
				return
			}
			if wake == nil {
				t.Fatalf("no wake for a session with job control")
			}
			if r.JobEnded == nil {
				t.Errorf("JobEnded was not wired, so nothing can ever poke the wake")
			}
			if armed := wake.fd() >= 0; armed != tc.armed {
				t.Errorf("wake.fd() >= 0 is %v, want %v", armed, tc.armed)
			}
		})
	}

	// And the same wake answers differently once the option moves, which is
	// the whole reason it is lazy rather than decided at the start.
	t.Run("the option moving under a live session", func(t *testing.T) {
		r := notifyingRunner(t, interp.No)
		s := Shell{Runner: r, Out: &strings.Builder{}, Err: &strings.Builder{}}
		wake, stop := s.jobNotifying()
		defer stop()
		if wake.fd() >= 0 {
			t.Fatalf("armed while the option was off")
		}
		r.Semantics.FinishedJobNoticeArrivesAtOnce = interp.Yes
		if wake.fd() < 0 {
			t.Errorf("still unarmed after the option was turned on, so `setopt notify` at a prompt would do nothing")
		}
		// And a poke now reaches the descriptor the editor is waiting on.
		r.JobEnded()
		if !wake.signal.pending {
			t.Errorf("the poke did not reach the wake")
		}
		wake.drain()
		if wake.signal.pending {
			t.Errorf("the wake was not taken back off the pipe")
		}
	})
}

// A wake with nothing owed behind it draws nothing over the line.
//
// The race this is about is ordinary rather than exceptional: the job ends
// just as the prompt is being drawn, the prompt's own report takes the line,
// and the byte in the pipe arrives at an editor with nothing left to say. A
// redraw there would put a second copy of the prompt on the screen for a
// notice nobody wrote.
func TestAJobWakeWithNothingOwedDrawsNoSecondPrompt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		notices func() bool
		want    []string
		absent  []string
	}{
		{
			"a notice to write",
			func() bool { return true },
			// A row of its own, then the prompt's leading rows, then the
			// line back under them. Measured on zsh 5.9.2 — see
			// jobnotify.go, and cmd/zsh's pty test for the same shape at a
			// terminal.
			//
			// A bare line feed rather than `\r\n` because this editor has
			// no terminal and so took no raw mode: the return is owed only
			// where the kernel has stopped adding it. See editor.newline.
			[]string{"\nLEAD\n", "$ ", "typed"},
			nil,
		},
		{
			"nothing owed after all",
			func() bool { return false },
			[]string{"\n"},
			[]string{"LEAD", "typed"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			e := Shell{}.newEditor(t.Context(), nil)
			e.out = &out
			e.line = []rune("typed")
			e.pos = len(e.line)
			e.jobNotices = tc.notices
			e.reportJobs(drawPrompt("LEAD\n$ "))
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("the terminal was given %q, want %q in it", out.String(), want)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(out.String(), absent) {
					t.Errorf("the terminal was given %q, which redraws for a notice nobody wrote", out.String())
				}
			}
		})
	}
}

// notifyingRunner is an interactive shell with the monitor on and one answer
// to the axis under test.
func notifyingRunner(t *testing.T, atOnce interp.Answer) *interp.Runner {
	t.Helper()
	sem := interp.PosixSemantics()
	sem.FinishedJobNoticeArrivesAtOnce = atOnce
	r := &interp.Runner{
		Semantics: &sem, JobControl: true, Terminal: true,
		Stdout: &strings.Builder{}, Stderr: &strings.Builder{},
	}
	r.SetInteractiveMonitor()
	return r
}
