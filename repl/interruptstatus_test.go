// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "testing"

// What a ^C at the prompt leaves in `$?` and whether it keeps the line
// (#5867, #5888), asked of the method every read calls rather than through a
// terminal.
//
// Measured 2026-10-04 through a pseudo-terminal: with no trap the line is
// given up and `$?` is 130 whatever it was before, in every shell; under
// `trap "" INT` the line is kept and the status left alone. A trap with a body
// is the dialect's answer, and this runner's semantics are the core's, whose
// answer is ksh93's and dash's: the trap runs, the line is given up and the
// status stands. cmd/bash and cmd/zsh carry the session tests for theirs.
func TestAControlCAtThePromptIsAnsweredOnce(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup string
		want  int
		keep  bool
		ran   string
	}{
		{"with no trap", "false", 130, false, ""},
		{"under a trap with a body", "trap 'ran=yes' INT; false", 1, false, "yes"},
		{"under trap '' INT", "trap '' INT; false", 1, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestRunner(nil)
			// The ignore is the process's, so it is put back whatever
			// happens below.
			t.Cleanup(func() { _ = runScript(t, r, "trap - INT") })
			_ = runScript(t, r, tc.setup)
			keep := Shell{Runner: r}.answerInterrupt(t.Context(), nil, true)
			if got := r.ExitStatus(); got != tc.want {
				t.Errorf("after %q and ^C, $? = %d, want %d", tc.setup, got, tc.want)
			}
			if keep != tc.keep {
				t.Errorf("after %q and ^C, kept the line = %v, want %v", tc.setup, keep, tc.keep)
			}
			if got, _ := r.GetVar("ran"); got != tc.ran {
				t.Errorf("after %q and ^C, the trap left ran=%q, want %q", tc.setup, got, tc.ran)
			}
		})
	}
}
