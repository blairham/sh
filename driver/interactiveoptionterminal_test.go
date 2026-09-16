// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// The other direction of #3195, and the only place it can be asked: an
// invocation that says `+o <name>` on a **terminal** reads its program instead
// of prompting.
//
// Every other case is a pipe, where the shell would not have prompted anyway,
// so a front end that read the option in one direction only would pass all of
// them. Here the descriptor says "a person" and the invocation says otherwise,
// and the invocation wins — measured 2026-09-16 on zsh 5.9.2 through a
// pseudo-terminal, where `zsh -f +o interactive` draws no prompt at all and
// `zsh -f` draws one.
//
// The mark for "it ran" is an expression, so that waiting for `mark-42` cannot
// be satisfied by the terminal echoing back what was typed — which matters
// especially here, because a shell that is not interactive never takes the
// terminal out of cooked mode and the line discipline echoes every keystroke.
func TestAnInvocationOptionTurnsThePromptOffAtATerminal(t *testing.T) {
	for _, tc := range []struct {
		name    string
		argv    []string
		prompts bool
	}{
		{"nothing written", []string{"testsh"}, true},
		{"the name under a plus", []string{"testsh", "+o", "interactive"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir()) // never the person's own

			control, tty := terminal(t)
			sh := shell()
			sh.Register = func(r *interp.Runner) { r.AddSetOptions("interactive") }
			sh.Semantics.InteractiveOptionName = "interactive"
			sh.Stdin, sh.Stdout, sh.Stderr = tty, tty, tty

			drawn := watch(t, control, defaultPrompt)
			done := make(chan int, 1)
			go func() { done <- driver.MainArgs(sh, tc.argv) }()

			if tc.prompts {
				// A prompt has to arrive before anything is typed: bytes put
				// into a terminal the shell has not yet taken are read under
				// the line discipline instead.
				drawn.awaitReadyForInput(t)
			}
			write(t, control, "echo mark-$((6 * 7))\r")
			drawn.await(t, "mark-42")
			// `exit` rather than a ^D, because the two shells here are
			// reading in different modes and only one of them has an editor
			// to hand the byte to.
			write(t, control, "exit\r")

			select {
			case code := <-done:
				if code != 0 {
					t.Errorf("status = %d, want 0", code)
				}
			case <-time.After(sessionBudget):
				_ = control.Close()
				t.Fatalf("the session did not end; drawn so far: %q", drawn.text())
			}
			// The prompt itself is the whole of what this route decides. It
			// is asserted after the session has ended so that "not yet" and
			// "never" are not the same reading.
			if got := strings.Contains(drawn.text(), defaultPrompt); got != tc.prompts {
				t.Errorf("prompted = %v, want %v — drew %q", got, tc.prompts, drawn.text())
			}
		})
	}
}
