// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// pipelineDeathSem is unforkedDeathSem with nothing waited out, so the shell
// takes a death from outside at its next command, and with the answer this
// file is about.
func pipelineDeathSem(before Answer) Semantics {
	s := unforkedDeathSem(No)
	s.DeathDuringAPipelineShowsTheStatusBeforeIt = before
	return s
}

// The `$?` the EXIT trap reads after a fatal signal from outside that landed
// while the shell waited for a pipeline is the status the pipeline began with
// in one dialect, and 0 in the other (#7160).
//
// Measured 2026-11-01, `trap 'echo "bye $?"' EXIT; (exit 7)` and the line, TERM
// from another process while the `sleep` runs, each shell in a session of its
// own: bash 5.3.20 reads the first column and ksh93u+ the second. This shell
// read 0 for the first and fourth rows and 7 for the second, third and fifth,
// whatever the dialect: a pipeline is not a command, so the status shown was
// the one a command before it began with. A program or parentheses alone
// already read what they began with, either way.
func TestADeathFromOutsideDuringAPipelineInOneDialect(t *testing.T) {
	const trap = `trap 'echo "bye $?"' EXIT; (exit 7); `
	for _, tc := range []struct {
		name, src string
		yes, no   string
	}{
		{"a program last", "P | /bin/cat; echo after", "bye 7\n", "bye 0\n"},
		{"after another status", "(exit 5); P | /bin/cat; echo after", "bye 5\n", "bye 0\n"},
		// The pipeline's own beginning, not the statement's.
		{"behind an and list", "true && P | /bin/cat; echo after", "bye 0\n", "bye 0\n"},
		{"a subshell last", ": | ( P; echo in ); echo after", "bye 7\n", "bye 0\n"},
		{"the last command", "(exit 5); P | /bin/cat", "bye 5\n", "bye 0\n"},
		{"a program alone", "P; echo after", "bye 7\n", "bye 7\n"},
		{"parentheses alone", "( P ); echo after", "bye 7\n", "bye 7\n"},
	} {
		for _, answer := range []Answer{Yes, No} {
			t.Run(tc.name+"/"+answer.String(), func(t *testing.T) {
				want := killedByTERM(tc.no)
				if answer == Yes {
					want = killedByTERM(tc.yes)
				}
				got, taken := handedOverWhileWaiting(t, trap+tc.src, pipelineDeathSem(answer), syscall.SIGTERM, 1)
				if len(taken) != 1 || !taken[0] {
					t.Errorf("taken %v, want the one offer taken", taken)
				}
				if got != want {
					t.Errorf("ended %+v, want %+v", got, want)
				}
			})
		}
	}
}
