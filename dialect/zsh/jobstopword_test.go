// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"syscall"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// This shell words a stopped job by the signal that stopped it, and ^Z is only
// one of four answers.
//
// Measured 2026-09-26 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` on it says it is not a Go
// executable — on a pseudo-terminal with `TERM=dumb` and the shell started
// `-fiV +Z`, one `sleep 30 &` and one `kill -NAME %1` per row, with a `jobs`
// after each and a `kill -CONT %1` between them:
//
//	kill -TSTP %1   [1]  + suspended  sleep 30
//	kill -STOP %1   [1]  + suspended (signal)  sleep 30
//	kill -TTIN %1   [1]  + suspended (tty input)  sleep 30
//	kill -TTOU %1   [1]  + suspended (tty output)  sleep 30
//
// The `jobs` row and the notice carry the same word every time, which is why
// this is the state column and not a second sentence.
//
// **SIGTSTP is the row that discriminates**, and it is the one this shell
// already wrote: ^Z is the plain word, so a table that answered every stop
// with `suspended (signal)` would look right on three rows out of four and
// wrong on the only one anybody reaches by hand (#4527).
func TestAStoppedJobIsWordedByTheSignalThatStoppedIt(t *testing.T) {
	dg := zsh.Diagnostics()
	if got, want := dg.JobStopped, "suspended"; got != want {
		t.Errorf("JobStopped = %q, want %q — what ^Z reaches", got, want)
	}
	if _, worded := dg.JobStoppedBySignal[syscall.SIGTSTP]; worded {
		t.Errorf("SIGTSTP has a word of its own, want it to fall through to JobStopped")
	}
	for _, c := range []struct {
		sig  syscall.Signal
		want string
	}{
		{syscall.SIGSTOP, "suspended (signal)"},
		{syscall.SIGTTIN, "suspended (tty input)"},
		{syscall.SIGTTOU, "suspended (tty output)"},
	} {
		if got := dg.JobStoppedBySignal[c.sig]; got != c.want {
			t.Errorf("signal %d stops a job as %q, want %q", c.sig, got, c.want)
		}
	}
}

// And the longest of them lands in the state column the same way a long signal
// word does.
//
// `suspended (tty output)` is twenty-two characters, well past the column's
// nine, so it is followed by two spaces and not padded to anything — the rule
// TestTheStateColumnIsNineWideWithTwoSpacesAfterIt measured, asked again of
// the words that are new here because a flat eleven would truncate or pad
// every one of them.
func TestAStoppedJobsSignalWordSitsInTheStateColumn(t *testing.T) {
	dg := zsh.Diagnostics()
	for _, c := range []struct{ state, want string }{
		{"suspended", "[1]  + suspended  sleep 30"},
		{"suspended (signal)", "[1]  + suspended (signal)  sleep 30"},
		{"suspended (tty input)", "[1]  + suspended (tty input)  sleep 30"},
		{"suspended (tty output)", "[1]  + suspended (tty output)  sleep 30"},
	} {
		if got := interp.Wording(dg.JobLine, "", 1, "+", c.state, "sleep 30"); got != c.want {
			t.Errorf("row = %q, want %q", got, c.want)
		}
	}
}
