// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// runJobScript runs a script over a vector the caller has adjusted, and hands
// back what it wrote.
//
// External commands rather than builtins wherever a job has to carry a status,
// because that is the shape the panel was measured on and because a job made
// of builtins has no process here — which is the other half of what these
// tests are about.
func runJobScript(t *testing.T, src string, set func(*Semantics)) (out, errOut string) {
	t.Helper()
	return runGatedJobScript(t, src, set, nil)
}

// runGatedJobScript is the same with a gate, for the one test that runs a
// `kill`.
//
// The gate is there for what a **mutant** would do rather than for what this
// shell does. `kill "$!"` on a job with no process of its own reaches no
// signal at all here: the number is read back as the job, and a job with
// nothing to signal is reported rather than aimed at. Undo that and the
// number is 0 again — which POSIX gives `kill` as *every process in the
// sender's group*, so the mutant terminates the test binary running it. It
// did, the first time it was tried. A gate that answers no to every signal
// changes nothing about the passing path and turns that into a refusal.
func runGatedJobScript(t *testing.T, src string, set func(*Semantics), gate Gate) (out, errOut string) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sem := testSemantics()
	if set != nil {
		set(&sem)
	}
	// Locked buffers rather than plain ones: on the deadline below the shell
	// is still running and still writing, and reporting what it had managed to
	// say is the whole value of that report.
	var o, e syncBuffer
	r := newTestRunner(t, &Runner{
		Stdout: &o, Stderr: &e, Semantics: &sem, Name: "testsh", Env: testPATH(), Gate: gate,
	})
	// Bounded, because **a test that can hang is a defect on its own**,
	// whatever it asserts. These scripts wait for jobs, and a job that never
	// ends leaves the shell in a `wait` nothing ends either — and since a
	// blocked open(2) or read(2) is not something a context can interrupt,
	// the deadline has to be here rather than on the run. Without it the cost
	// of one lost rendezvous is the whole `interp` package at `go test`'s
	// ten-minute timeout, on an unrelated pull request, which is what #2692
	// was: nine and a half minutes of a required check spent in this helper.
	//
	// Generous, so that it is never the reason a slow runner goes red: these
	// scripts are milliseconds, and a minute is three orders of magnitude of
	// room. The goroutine is left behind on the failing path deliberately —
	// it is blocked in a syscall and cannot be told to stop, and the report
	// is worth more than the leak in a binary that is about to exit.
	done := make(chan error, 1)
	go func() {
		_, rerr := r.Run(context.Background(), f)
		done <- rerr
	}()
	select {
	case rerr := <-done:
		if rerr != nil {
			t.Fatalf("run: %v", rerr)
		}
	case <-time.After(time.Minute):
		t.Fatalf("the script never finished, so something in it is waiting for what is not coming; it had written %q (stderr %q):\n%s",
			o.String(), e.String(), src)
	}
	return o.String(), e.String()
}

// A job waited out by process id gives its number back, so the next job takes
// it and `%1` names that job and not the one before it.
//
// Measured 2026-09-13 across the whole panel — bash 5.3.15, that bash invoked
// as `sh`, bash 3.2.57, zsh 5.9.2, ksh93u+ 2012-08-01, dash and BusyBox ash
// 1.37.0 — and all seven answer 4. This shell answered 7: waiting by id left
// the finished job in the table forever, so the second job was numbered `%2`
// and `%1` went on reporting a status the script had already collected
// (#2651).
//
// The reaping is done **by process id and not by `%1`**, which is the whole
// discriminating part: the `%` spec route already dropped the job, so a probe
// written that way passed while the ordinary spelling every script uses did
// not. And the second job is started *after* the first is waited for, because
// a shell that never reaps anything answers alike whatever its rule is.
func TestAReapedJobGivesItsNumberBack(t *testing.T) {
	const src = `/bin/sh -c 'exit 7' & p=$!
wait "$p"
/bin/sh -c 'exit 4' &
wait %1; echo "one=$?"
`
	out, errOut := runJobScript(t, src, nil)
	if !strings.Contains(out, "one=4") {
		t.Errorf("out = %q, want one=4 — `%%1` is the job the table holds now; stderr %q", out, errOut)
	}
}

// And the number really is free rather than merely shadowed: there is no `%2`
// for the second job to have taken instead.
//
// The pair is what separates "the slot was reused" from "the shell numbered on
// past it and `%1` happens to answer": with only the row above, a shell that
// put the new job at `%2` and *also* dropped the old one from `%1` would pass.
func TestAReapedJobLeavesNoSecondSlot(t *testing.T) {
	const src = `/bin/sh -c 'exit 7' & p=$!
wait "$p"
/bin/sh -c 'exit 4' &
wait %2 >/dev/null 2>&1; echo "two=$?"
wait
`
	out, errOut := runJobScript(t, src, nil)
	if strings.Contains(out, "two=4") {
		t.Errorf("out = %q, want no job at `%%2`; stderr %q", out, errOut)
	}
}

// The status of a job that has been reported is still reachable by the id it
// was reported under, where the dialect keeps it.
//
// Measured 2026-09-13 on `/bin/sh -c 'exit 7' & p=$!; wait %1; wait "$p"`:
// six of the seven columns answer 7 on the second wait and ksh93u+ answers the
// status a process that was never this shell's child gets. See
// Semantics.WaitRemembersAReapedJob.
func TestWaitRemembersAJobItHasAlreadyReported(t *testing.T) {
	const src = `/bin/sh -c 'exit 7' & p=$!
wait %1; echo "first=$?"
wait "$p" 2>/dev/null; echo "again=$?"
`
	for _, tc := range []struct {
		name     string
		remember Answer
		want     string
	}{
		{"remembered", Yes, "again=7"},
		{"forgotten", No, "again=127"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut := runJobScript(t, src, func(s *Semantics) {
				s.WaitRemembersAReapedJob = tc.remember
			})
			if !strings.Contains(out, "first=7") {
				t.Fatalf("out = %q, want the wait by name to report the job; stderr %q", out, errOut)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("out = %q, want %s; stderr %q", out, tc.want, errOut)
			}
		})
	}
}

// The memory is not the table: a job that has been reported is out of the
// listing and out of every `%` spec, whichever way the axis is answered.
//
// Asked under the answer that *keeps* the status, because that is the one a
// mistake would hide behind — a shell that had simply left the job in the
// table would pass the test above and fail this one.
func TestAReportedJobIsOutOfTheTable(t *testing.T) {
	const src = `/bin/sh -c 'exit 7' & p=$!
wait "$p"
jobs > listing.txt
grep -c . listing.txt > rows.txt
read rows < rows.txt; echo "rows=$rows"
wait %1 >/dev/null 2>&1; echo "spec=$?"
`
	out, errOut := runJobScript(t, src, func(s *Semantics) {
		s.WaitRemembersAReapedJob = Yes
	})
	if !strings.Contains(out, "rows=0") {
		t.Errorf("out = %q, want an empty listing; stderr %q", out, errOut)
	}
	if strings.Contains(out, "spec=7") {
		t.Errorf("out = %q, want `%%1` to name nothing; stderr %q", out, errOut)
	}
}

// A background job that has ended and has not been waited for still answers
// `kill`, because on every reference it is a child that has exited and that
// the shell has not reaped.
//
// `cmd & p=$!; kill -0 "$p"` is how a script asks whether a job it started is
// still its own to collect, and this shell answered no for every job made of
// builtins or of a compound command: a real shell forks there and has a body
// to leave behind, and here there is nothing to aim at, so the number came
// back as a job with no process and `kill` reported it missing (#2938).
//
// Both spellings, because they are one decision: `$!` is read back as the job
// it names, so `kill "$!"` and `kill %1` have to reach the same answer. The
// second line is what stops it being "the table forgot the job": after `wait`
// the job is out of the table and the invented id goes to the kernel, where
// it is out above every process id one can issue and finds nothing.
//
// Held against the panel — bash 5.3.15, zsh 5.9.2, ksh93u+ 2012-08-01 and
// dash — all four answer the same two ways round on
// `( exit 0 ) & p=$!; kill -0 "$p"; …; wait; kill -0 "$p"`. What they do
// *not* do is hold the answer until `wait`: each reaps the child from its own
// SIGCHLD handler, so a busy loop of builtins between the `&` and the `kill`
// turns all four to a failure. That is a window rather than a state, and a
// window is not something a shell can implement — so this shell holds the job
// until the script collects it, which is the rule the idiom is written
// against and is right at the instant a script actually asks.
//
// The gate is there for what a mutant would do, for the reason
// runGatedJobScript gives: nothing is signaled on the passing path, so a gate
// that refuses everything changes nothing about it.
func TestAnEndedJobStillAnswersKillUntilItIsWaitedFor(t *testing.T) {
	const src = `( exit 0 ) & p=$!
kill -0 "$p" 2>/dev/null; echo "byid=$?"
kill -0 %1 2>/dev/null; echo "byspec=$?"
wait
kill -0 "$p" 2>/dev/null; echo "after=$(( $? != 0 ))"
`
	out, errOut := runGatedJobScript(t, src, nil, GateFunc(func(_ context.Context, a Action) Decision {
		if a.Kind == ActionSignal {
			return Deny
		}
		return Allow
	}))
	if !strings.Contains(out, "byid=0") {
		t.Errorf("out = %q, want `kill -0 $!` to find the job this shell has not been asked to collect; stderr %q", out, errOut)
	}
	if !strings.Contains(out, "byspec=0") {
		t.Errorf("out = %q, want `%%1` to answer where `$!` does — one job, one answer; stderr %q", out, errOut)
	}
	if !strings.Contains(out, "after=1") {
		t.Errorf("out = %q, want the answer to stop once `wait` has collected the job; stderr %q", out, errOut)
	}
}
