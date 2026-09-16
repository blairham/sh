// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// `+i` is the other half of the `-i` letter, and this front end used to read
// only the minus — #3221.
//
// The plus fell through to the runner as an ordinary `set` letter, where it
// was refused: ksh93's binary answered `+i: unknown option`, bash printed its
// whole usage, dash said `+i is not implemented yet`, and zsh gave the letter
// away to a different option and reported the wrong `$-`. Every shell in the
// panel takes the word.
//
// Which way it is taken is the dialect's, because BusyBox ash does not read
// the sign — see Semantics.PlusSignedInteractiveLetterStillPrompts. The
// unspecified arm is the core with no dialect chosen, and it must stay a
// refusal: a front end that decided the sign for itself would answer for a
// column measured to disagree.
func TestThePlusSignedInteractiveLetterIsTheDialectsToRead(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  interp.Answer
		argv    []string
		prompts bool
		refuses bool
	}{
		{"the plus takes it back", interp.No, []string{"testsh", "-i", "+i"}, false, false},
		{"the plus alone", interp.No, []string{"testsh", "+i"}, false, false},
		{"the plus then the letter", interp.No, []string{"testsh", "+i", "-i"}, true, false},
		{"the letter, for comparison", interp.No, []string{"testsh", "-i"}, true, false},
		// The column that does not read the sign. `+i` asks for a prompt
		// there exactly as `-i` does, so the last-wins sequence cannot take
		// one away and both orders prompt.
		{"the plus asks for it", interp.Yes, []string{"testsh", "+i"}, true, false},
		{"the letter then the plus", interp.Yes, []string{"testsh", "-i", "+i"}, true, false},
		// And no dialect at all: the letter reaches the runner, which
		// refuses it the way it always did.
		{"nobody answered", interp.Unspecified, []string{"testsh", "+i"}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh := shell()
			sh.Semantics.PlusSignedInteractiveLetterStillPrompts = tc.answer
			sh.Stdin = pipeWith(t, "echo TYPED\n")
			out, errs, code := runArgs(t, sh, tc.argv...)
			both := out + errs
			if tc.refuses {
				if code == 0 {
					t.Fatalf("status 0, want a refusal — out %q, stderr %q", out, errs)
				}
				return
			}
			if code != 0 {
				t.Fatalf("status %d, out %q, stderr %q", code, out, errs)
			}
			if !strings.Contains(both, "TYPED") {
				t.Fatalf("said %q, want the line to have run under either answer", both)
			}
			if got := strings.Contains(both, "$ "); got != tc.prompts {
				t.Errorf("prompted = %v, want %v — said %q", got, tc.prompts, both)
			}
		})
	}
}
