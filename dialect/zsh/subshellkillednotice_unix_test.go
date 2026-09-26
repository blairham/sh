// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh_test

import (
	"strconv"
	"syscall"
	"testing"
)

// This shell says nothing about a foreground subshell a signal ended, which is
// what it says about every other command a signal ends (#4649).
//
// The row that keeps #4649's fix from becoming everyone's sentence: the
// notice is reached through the same question an external command's death
// asks — `ReportsACommandKilledBySignal`, which this dialect answers No — so
// a subshell reporting here would mean the rule had been written twice.
//
// Measured 2026-09-26 against `/opt/homebrew/bin/zsh -f` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` on it says it is not a Go
// executable — with `zmodload zsh/system; ( kill -NAME $sysparams[pid] );
// print "st=$?"`: SIGTERM is `st=143` and SIGUSR1 is `st=158`, both with
// nothing on stderr at all.
func TestAForegroundSubshellKilledBySignalIsNotReported(t *testing.T) {
	for _, tc := range []struct {
		signal string
		number syscall.Signal
	}{
		{"TERM", syscall.SIGTERM},
		{"USR1", syscall.SIGUSR1},
	} {
		src := "zmodload zsh/system\n( kill -" + tc.signal + " $sysparams[pid] )\nprint \"st=$?\"\n"
		// The number rather than the number written out: SIGUSR1 is 30 on a
		// BSD and 10 on Linux, and CI runs both.
		want := "st=" + strconv.Itoa(128+int(tc.number)) + "\n"
		if got := runZshAnchored(t, src); got != want {
			t.Errorf("%s: got %q, want %q and nothing else", tc.signal, got, want)
		}
	}
}
