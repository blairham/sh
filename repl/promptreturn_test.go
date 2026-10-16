// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// A `return` with nothing to return from, typed at a prompt, ends the line or
// the session — and never leaves the session reading lines it then refuses to
// run, which is what it did: the `return` was left standing once the line was
// over, so every command after it was refused, `exit` included, and the
// terminal could only be killed (#6717).
//
// Measured 2026-10-16 through a pseudo-terminal with a two-row PS1, the same
// four lines one at a time: zsh 5.9.2 writes nothing for the second line,
// `line3 5` for the third and ends at 9; ksh93u+ and dash end at the `return`,
// at 5, with the EXIT trap run. See Semantics.ReturnAtThePromptEndsTheSession.
//
// The `return` is inside an `if` so that it reaches the line as a transfer
// unwinding a compound, not as the statement itself, and the `echo n2` after
// it on the same line is the rest of the line that goes with it.
func TestAReturnAtThePromptEndsTheLineOrTheSession(t *testing.T) {
	const input = "trap 'echo T $?' EXIT\n" +
		"echo line1\n" +
		"if true; then return 5; fi; echo n2\n" +
		"echo line3 $?\n" +
		"exit 9\n"
	for _, tc := range []struct {
		name       string
		ends       interp.Answer
		want       string
		wantStatus int
		// The prompts drawn, which is what tells a session that ended from
		// one still reading lines it will not run: both leave the same
		// output, and only the wedged one prompts for the lines after.
		prompts int
	}{
		{"the line alone, as zsh", interp.No, "line1\nline3 5\nT 9\n", 9, 5},
		{"the session, as ksh93 and dash", interp.Yes, "line1\nT 5\n", 5, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs := &strings.Builder{}, &strings.Builder{}
			r := newTestRunner(nil)
			r.Stdout, r.Stderr = out, errs
			r.Semantics.ReturnOutsideAFunctionIsRefused = interp.No
			r.Semantics.ReturnAtThePromptEndsTheSession = tc.ends
			s := Shell{Runner: r, In: readerFile(t, input), Out: errs, Err: errs}
			_, err := s.Run(t.Context())
			if err != nil {
				t.Fatalf("the session refused the input: %v", err)
			}
			status := r.Finish(t.Context())
			if got := out.String(); got != tc.want {
				t.Errorf("out = %q, want %q (stderr %q)", got, tc.want, errs.String())
			}
			if status != tc.wantStatus {
				t.Errorf("status = %d, want %d", status, tc.wantStatus)
			}
			if got := strings.Count(errs.String(), "$ "); got != tc.prompts {
				t.Errorf("prompts drawn = %d, want %d (stderr %q)", got, tc.prompts, errs.String())
			}
		})
	}
}
