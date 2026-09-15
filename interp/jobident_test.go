// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `$!` is a number for every background job, whatever the job is made of.
//
// Measured 2026-09-13 across the panel on `( exit 5 ) &`, `true &`, `{ :; } &`
// and `/bin/sleep 0 &`: bash 5.3.15, that bash as `sh`, bash 3.2.57, zsh
// 5.9.2, ksh93u+ 2012-08-01, dash and BusyBox ash 1.37.0 all report a positive
// id for all four, and all four ids differ. They fork before the body runs, so
// the question never comes up for them.
//
// This shell reported 0 for the three that never reach an external program,
// and 0 is the one number that must not be given: POSIX gives it to `kill` as
// every process in the sender's process group (#2650).
//
// The external command is the control. Without it a shell that reported the
// same wrong thing for everything would have to be told apart from one that
// reports nothing at all, and `set=` alone cannot do it.
func TestEveryBackgroundJobHasAnIdOfItsOwn(t *testing.T) {
	const src = `( exit 5 ) & a=$!
true & b=$!
{ :; } & c=$!
/bin/sleep 0 & d=$!
echo "set=$(( a > 0 && b > 0 && c > 0 && d > 0 ))"
echo "distinct=$(( a != b && a != c && a != d && b != c && b != d && c != d ))"
wait
`
	out, errOut := runJobScript(t, src, nil)
	if !strings.Contains(out, "set=1") {
		t.Errorf("out = %q, want every `$!` above zero; stderr %q", out, errOut)
	}
	if !strings.Contains(out, "distinct=1") {
		t.Errorf("out = %q, want four different ids; stderr %q", out, errOut)
	}
}

// And the id is the job's, so a `wait` for it answers about that job and not
// about whichever other processless job the walk reached last.
//
// This is what the sameness cost: both reads were 0, `wait 0` matched every
// job with no process, and the loop reported the last one's status for both.
func TestWaitingForAProcesslessJobAnswersAboutThatJob(t *testing.T) {
	const src = `( exit 5 ) & a=$!
( exit 6 ) & b=$!
wait "$b"; echo "second=$?"
wait "$a"; echo "first=$?"
`
	out, errOut := runJobScript(t, src, func(s *Semantics) {
		s.WaitRemembersAReapedJob = Yes
	})
	if !strings.Contains(out, "second=6") || !strings.Contains(out, "first=5") {
		t.Errorf("out = %q, want second=6 and first=5; stderr %q", out, errOut)
	}
}

// `kill` reads the number back as the job it names rather than handing it to
// the kernel.
//
// The invented ids are out above every process id a kernel can issue, which is
// what makes the lookup a lookup and not a guess — but the lookup is the point:
// `p=$!; kill "$p"` is the line a script writes to stop one job, and it has to
// mean the job. A job of builtins has no process to signal, so what comes back
// is the answer `kill %1` already gives for the same job, and what must *not*
// come back is anything about a process group.
func TestKillReadsAnInventedIdAsItsJob(t *testing.T) {
	// The job is kept alive by a read the test releases, and the fifo is
	// opened `<>` rather than `<` on purpose.
	//
	// A fifo opened for reading blocks until a writer arrives and one opened
	// for writing blocks until a reader does, so a script whose two halves
	// open opposite ends is a rendezvous **in which either side can block
	// forever**. Nothing orders the background job's open against the
	// foreground's, and where a real shell has a forked child here we have a
	// goroutine — so if the two ever miss each other, the foreground shell
	// waits in open(2) for a reader that is not coming, `wait` is never
	// reached, and the whole package is lost to the ten-minute timeout with
	// this test's name on it. That is what #2692 cost PR #2689: a hang rather
	// than a failure, on a diff that touches no job, signal or kill code.
	//
	// `<>` opens read-write, which blocks for neither, so only one of the two
	// opens can block now and the side that can is waiting on a goroutine
	// that is runnable rather than on one that is already blocked. The
	// release is a line of data instead of the writer closing, because the
	// reader's own read-write end counts as a writer and no end-of-file would
	// ever come.
	const src = `mkfifo p
( read x <> p; exit 5 ) & j=$!
kill "$j" 2>byid.txt; echo "byid=$?"
kill %1 2>byspec.txt; echo "byspec=$?"
grep -q "no such job" byid.txt; echo "job=$?"
grep -q "no such job" byspec.txt; echo "spec=$?"
echo go > p
wait
`
	out, errOut := runGatedJobScript(t, src, nil, GateFunc(func(_ context.Context, a Action) Decision {
		if a.Kind == ActionSignal {
			return Deny
		}
		return Allow
	}))
	// The two complaints differ only in how the script spelled the operand,
	// and the kind is the load-bearing half: a number handed to the kernel
	// instead comes back as a *process* that is not there, which is a
	// different sentence and a different question.
	if !strings.Contains(out, "job=0") || !strings.Contains(out, "spec=0") {
		t.Errorf("out = %q, want `kill $!` read as the job `kill %%1` names; stderr %q", out, errOut)
	}
	if !strings.Contains(out, "byid=1") {
		t.Errorf("out = %q, want the id read as a job with nothing to signal; stderr %q", out, errOut)
	}
}
