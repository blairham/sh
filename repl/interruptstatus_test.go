// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "testing"

// What a line abandoned with ^C leaves in `$?` (#5867), asked of the method
// the loop calls rather than through a terminal, because the third row cannot
// be asked through one yet.
//
// Measured 2026-10-04 through a pseudo-terminal against bash 5.3.20 and zsh
// 5.9.2: 130 whatever the status was before, and under `trap "" INT` the
// status left alone. Both shells also leave the half-typed line in
// place there, where this editor abandons it — so a session test of the
// ignored row would be waiting for a prompt the right answer never draws.
// cmd/bash and cmd/zsh carry the session tests for the rows that draw one.
func TestAControlCAtThePromptSetsOneHundredThirty(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup string
		want  int
	}{
		{"with no trap", "false", 130},
		// bash's answer. zsh runs the body at the prompt, keeps the line
		// and leaves the status — and this session runs no trap at the
		// prompt at all, which is a gap of its own rather than this row's.
		{"under a trap with a body", "trap : INT; false", 130},
		{"not under trap '' INT", "trap '' INT; false", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestRunner(nil)
			// The ignore is the process's, so it is put back whatever
			// happens below.
			t.Cleanup(func() { _ = runScript(t, r, "trap - INT") })
			_ = runScript(t, r, tc.setup)
			Shell{Runner: r}.interruptedAtThePrompt()
			if got := r.ExitStatus(); got != tc.want {
				t.Errorf("after %q and ^C, $? = %d, want %d", tc.setup, got, tc.want)
			}
		})
	}
}
