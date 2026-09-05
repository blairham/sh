// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"syscall"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The prompt starts on a line of its own after a ^C, and the two ways it can
// know are not interchangeable.
//
// The shell hears the signal only while it still holds the terminal — a
// builtin, a loop of its own. An external command runs in a group the ^C goes
// to instead, and there the only evidence is what the command died of. Asking
// the first question alone is what put the prompt on top of the `^C` for every
// real command once job control started handing the terminal over, which is a
// bug no assertion about the signal could have caught.
func TestThePromptKnowsTheLineWasInterrupted(t *testing.T) {
	for _, tc := range []struct {
		name  string
		heard bool
		died  syscall.Signal
		want  bool
	}{
		{"the shell heard it, which is a builtin or a loop of its own", true, 0, true},
		{"the command died of it, which is everything else", false, syscall.SIGINT, true},
		{"the command died of something else", false, syscall.SIGTERM, false},
		{"nothing happened", false, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := &interrupts{}
			in.hit.Store(tc.heard)
			s := Shell{Runner: runnerWhoseCommandDiedOf(t, tc.died)}
			if got := s.interrupted(in); got != tc.want {
				t.Errorf("interrupted = %v, want %v", got, tc.want)
			}
		})
	}
}

// A ^Z is not a ^C. The interpreter decides what a stop does to the line, from
// the command that stopped, so a handler that offered it here as well would
// end the line both shells carry on with.
func TestOnlyAnInterruptIsOfferedToTheInterpreter(t *testing.T) {
	if !offersToTheInterpreter(syscall.SIGINT) {
		t.Error("an interrupt is what the interpreter has to be told about")
	}
	if offersToTheInterpreter(syscall.SIGTSTP) {
		t.Error("a stop is not, or the line would end where both shells carry on")
	}
}

// And the two flags are read independently: whoever asks first must not take
// the arrival from the other.
func TestTheTwoReadersOfAnArrivalDoNotTakeItFromEachOther(t *testing.T) {
	in := &interrupts{}
	in.hit.Store(true)
	in.pending.Store(true)
	if !in.took() {
		t.Error("the prompt was not told")
	}
	if !in.take() {
		t.Error("the interpreter was not told, so the prompt had taken it")
	}
	// And each is forgotten once read.
	if in.took() || in.take() {
		t.Error("the same arrival was read twice")
	}
}

// runnerWhoseCommandDiedOf is a shell whose last command a signal ended, built
// by running one the shell's own wait reports that way.
func runnerWhoseCommandDiedOf(t *testing.T, sig syscall.Signal) *interp.Runner {
	t.Helper()
	r := newTestRunner(nil)
	if sig != 0 {
		r.WaitForCommand = func(int) (interp.Wait, error) {
			return interp.Wait{Signal: sig, Killed: true}, nil
		}
	}
	f, err := syntax.Parse("/usr/bin/true", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RunPart(t.Context(), f); err != nil {
		t.Fatal(err)
	}
	return r
}
