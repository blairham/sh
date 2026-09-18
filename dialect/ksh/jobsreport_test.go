// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// TestThisPresetTakesTheJobsChangedLetter is #3390.
//
// `jobs -n` is in this shell's own usage line — `jobs [-lnp]` — and was an
// unknown option here at status 2, with the usage block after it, where the
// reference answers 0. Measured 2026-09-17 from a script file under `env -i
// PATH=/usr/bin:/bin` with stdin from /dev/null, on ksh93u+ 2012-08-01:
//
//	( exit 7 ) &; jobs -n >/dev/null 2>&1; echo "jobs -n: $?"     0
//	set -m; ( sleep .3 ) &; ( exit 7 ) &; sleep .15; jobs -n
//	    [2] +  Done(7)  <command unknown>                         0
//	the same `jobs -n` a second time                    nothing,  0
//	the same script without `set -m`                    nothing,  0
//
// The letter is not bash's letter of the same name — that one counts a job
// that has only just started as a change — so it is
// Semantics.JobsListsWhatChangedSinceTheLastReport rather than a shared
// reading, and bash's stays unimplemented.
func TestThisPresetTakesTheJobsChangedLetter(t *testing.T) {
	s := ksh.Semantics()
	if !strings.Contains(s.JobsOptions, "n") {
		t.Errorf("JobsOptions is %q, want the n in it", s.JobsOptions)
	}
	if got := s.JobsListsWhatChangedSinceTheLastReport; got != interp.Yes {
		t.Errorf("JobsListsWhatChangedSinceTheLastReport is %v, want Yes", got)
	}
	out, _ := answersRun(t, `( exit 7 ) &
jobs -n >/dev/null 2>&1
echo "jobs -n: $?"
wait`)
	if !strings.Contains(out, "jobs -n: 0") {
		t.Errorf("jobs -n in a script said %q, want status 0", out)
	}
	if strings.Contains(out, "unknown option") {
		t.Errorf("jobs -n said %q, want the letter taken", out)
	}
}

// TestThisPresetNamesTheSignalAWaitReaped is #3392.
//
// `wait` has a sentence of its own here for a child a signal ended, and this
// shell said nothing. Measured 2026-09-17 from a script file under `env -i`,
// ksh93u+ 2012-08-01:
//
//	sh -c 'kill -TERM $$' &; wait $!
//	    ./case.sh[2]: wait: <pid>: Terminated       then 271
//	the same with `wait %1`                          the same, naming the pid
//	the same with USR1                               wait: <pid>: User signal 1, 286
//	sh -c 'exit 3' &; wait $!                        nothing, 3
//	a bare `wait`                                    nothing, 0
//
// The status was already right — 256 plus the signal is this shell's
// encoding, through Semantics.SignalDeathStatusIsTwoFiftySix — so what was
// missing is only the sentence. It is `wait`'s own and not the general one:
// the same shell's report for a *foreground* command killed by a signal names
// neither the builtin nor a process id, and that one goes through
// Semantics.ReportsACommandKilledBySignal.
//
// The words are this shell's table rather than the machine's, which is why
// USR1 is `User signal 1` where the host says `User defined signal 1`.
func TestThisPresetNamesTheSignalAWaitReaped(t *testing.T) {
	out, _ := answersRun(t, `sh -c 'kill -TERM $$' & 2>/dev/null
wait $! 2>&1
echo "st=$?"`)
	if !strings.Contains(out, "wait:") || !strings.Contains(out, "Terminated") {
		t.Errorf("wait said %q, want its own sentence naming the signal", out)
	}
	if !strings.Contains(out, "st=271") {
		t.Errorf("wait said %q, want 256 plus the signal", out)
	}
	quiet, _ := answersRun(t, `sh -c 'exit 3' &
wait $! 2>&1
echo "st=$?"`)
	if strings.Contains(quiet, "wait:") {
		t.Errorf("an ordinary exit said %q, want nothing before the status", quiet)
	}
}
