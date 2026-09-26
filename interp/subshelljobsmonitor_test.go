// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The fourth answer to the subshell job table, and the only one that is not
// about the subshell's shape: the parent's jobs are there when the parent's
// **monitor** is on and gone when it is off, whatever kind of subshell asked.
//
// Everything here runs on the substrate with the axis set by hand and names
// no shell, which is this package's rule. What the one dialect that answers
// this way does with it is pinned in dialect/zsh.
//
// The monitor is turned on the way a script turns it on — `set -m`, with
// Runner.Terminal true so it is granted — rather than by reaching for the
// field, so the state under test is the one a real session is in.

// subshellMonitorRun runs src under SubshellJobsKeptUnderTheMonitor and
// answers with what it printed. monitor says whether the shell has one.
func subshellMonitorRun(t *testing.T, monitor bool, src string) string {
	t.Helper()
	if monitor {
		src = "set -m\n" + src
	}
	out, _ := run(t, src, func(r *Runner) {
		s := *r.Semantics
		s.SubshellJobTable = SubshellJobsKeptUnderTheMonitor
		// `disown`'s own two axes, answered so that the case below is
		// about the table rather than about them: the substrate leaves
		// both unspecified, and an unanswered axis is refused before the
		// guard under test is reached. The values are the majority ones —
		// `disown` reads its operand and takes the job out of the table.
		s.DisownAlwaysFails, s.DisownRemovesTheJob = No, Yes
		// And the order a listing of *two* jobs comes out in, which only
		// the case below can reach: without it a subshell holding an
		// inherited row and one of its own refuses the axis instead of
		// listing, and the case would be killed by a complaint rather than
		// by the count it is about.
		s.JobsListNewestFirst = No
		// The state filters the case below uses, and what a listing does
		// with a job that has ended: both are axes of their own and both
		// stand between that case and the count it asserts on.
		s.JobsOptions, s.JobsListFinishedJobs = "lprs", Yes
		r.Semantics = &s
		// Granted rather than refused: `set -m` is the one option whose
		// answer depends on there being a terminal, and a shell without one
		// would leave the monitor off and make every row below read "off".
		r.Terminal = true
		// A relative path in the script resolves against this and not
		// against the package directory. See subshellJobsRun.
		r.Dir = t.TempDir()
	})
	return strings.TrimSpace(out)
}

// The discriminating pair, and the reason this value exists at all.
//
// The same script, the same boundary, the same job — one switch moved. A
// reading of the axis that had kept the table for this dialect unconditionally
// would pass the `on` half and fail the `off` half, and the reading that
// shipped before it does the reverse; nothing that keys on the subshell's
// shape passes both.
func TestASubshellSeesTheParentsJobsOnlyUnderTheMonitor(t *testing.T) {
	const src = `/bin/sleep 0.4 & first=$!
(jobs -p) >s.txt
x=; read x <s.txt
case $x in "$first") echo parents;; "") echo none;; *) echo other;; esac
wait`
	if got := subshellMonitorRun(t, true, src); got != "parents" {
		t.Errorf("with the monitor on: got %q, want the parent's job", got)
	}
	if got := subshellMonitorRun(t, false, src); got != "none" {
		t.Errorf("with the monitor off: got %q, want nothing", got)
	}
}

// And the monitor is the *only* thing it turns on. Every boundary the other
// three answers tell apart agrees here, including the one they all clear.
//
// This is the half that catches a rule keyed on the wrong noun: the three
// answers above it are policies about a subshell's *shape*, so a shape-keyed
// reading of this one is the natural mistake, and it is invisible until the
// shapes are varied with the monitor held fixed. A `&` body is in the table
// deliberately — it is the row the other three do not even ask about.
func TestUnderTheMonitorEveryBoundarySeesThem(t *testing.T) {
	const tail = `
x=; read x <s.txt
case $x in "$first") echo parents;; "") echo none;; *) echo other;; esac
wait`
	for _, b := range []struct {
		name, body string
	}{
		{"an explicit subshell", `(jobs -p) >s.txt`},
		{"a simple command in a pipeline", `jobs -p | cat >s.txt`},
		{"a compound command in a pipeline", `{ jobs -p; } | cat >s.txt`},
		{"a subshell in a pipeline", `(jobs -p) | cat >s.txt`},
		{"a loop in a pipeline", `for i in 1; do jobs -p; done | cat >s.txt`},
		{"a command substitution", `printf '%s' "$(jobs -p)" >s.txt`},
		{"a function called in a subshell", `f() { jobs -p; }; (f) >s.txt`},
		{"a background job's own body", `{ /bin/sleep 0.1; jobs -p >s.txt; } & wait`},
	} {
		t.Run(b.name, func(t *testing.T) {
			src := "/bin/sleep 0.4 & first=$!\n" + b.body + tail
			if got := subshellMonitorRun(t, true, src); got != "parents" {
				t.Errorf("with the monitor on: got %q, want the parent's job", got)
			}
			if got := subshellMonitorRun(t, false, src); got != "none" {
				t.Errorf("with the monitor off: got %q, want nothing", got)
			}
		})
	}
}

// A job the subshell starts for itself replaces what it was looking at rather
// than joining it.
//
// Without this the listing below would be two rows, and the empty half of the
// pair above would also be satisfied by a `jobs` that does not work in a
// subshell at all — so this is the positive control for the instrument as much
// as a case of its own.
func TestASubshellThatStartsAJobStopsShowingTheParentsUnderTheMonitor(t *testing.T) {
	const src = `/bin/sleep 0.4 & first=$!
(/bin/sleep 0.4 & printf '%s\n' "$(jobs -p)" >s.txt; wait)
n=0; while read line; do n=$((n+1)); last=$line; done <s.txt
echo "n=$n"
case $last in "$first") echo parents;; "") echo none;; *) echo own;; esac
wait`
	if got := subshellMonitorRun(t, true, src); got != "n=1\nown" {
		t.Errorf("got %q, want one row and it the subshell's own", got)
	}
}

// The jobs are there to be *looked at*. The verbs that would act on one
// refuse, and they have to: a real shell forked and its subshell could not
// reach the parent's jobs at all, where a subshell here is a cloned Runner
// holding the parent's own job values — so a `wait` that got through would
// reap the parent's job out from under it and leave the parent listing one
// job fewer than it started.
func TestAnInheritedJobIsListedAndNotManipulable(t *testing.T) {
	const src = `/bin/sleep 0.4 & first=$!
(jobs -p >s.txt; wait %1; echo "wait=$?"; disown %1; echo "disown=$?")
x=; read x <s.txt
case $x in "$first") echo parents;; "") echo none;; *) echo other;; esac
jobs -p >t.txt
y=; read y <t.txt
case $y in "$first") echo still;; "") echo gone;; *) echo other;; esac
wait`
	got := subshellMonitorRun(t, true, src)
	for _, want := range []string{
		// The listing, which is what the inheritance is for.
		"parents",
		// And the two refusals, at the one measured status.
		"wait=1", "disown=1",
		// The parent's table, untouched by any of it.
		"still",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("got %q, want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "gone") {
		t.Errorf("the subshell reaped the parent's job:\n%s", got)
	}
}

// A bare `wait` in such a subshell waits for nothing. It has no jobs of its
// own, and the ones it can see are not its to wait for.
//
// **A status cannot tell the two readings apart** — a bare `wait` answers 0
// whether it waited for everything or for nothing — so the assertion is on
// what is still running when it returns. The parent's job is a one-second
// `sleep` entered a fifth of a second in, so a subshell that returned at once
// leaves it running and a subshell that waited it out leaves nothing for the
// `-r` filter to find. Eight tenths of a second of margin, and no clock in
// the test.
func TestABareWaitInAnInheritingSubshellWaitsForNothing(t *testing.T) {
	const src = `/bin/sleep 1 &
/bin/sleep 0.2
(wait; echo "bare=$?")
jobs -rp >t.txt
n=0; while read l; do n=$((n+1)); done <t.txt
echo "running=$n"
wait`
	got := subshellMonitorRun(t, true, src)
	if !strings.Contains(got, "bare=0") {
		t.Errorf("got %q, want a bare wait to answer 0", got)
	}
	if !strings.Contains(got, "running=1") {
		t.Errorf("got %q, want the parent's job still running after the subshell's `wait`", got)
	}
}

// And by number it is not this shell's child either, which is the complaint a
// number naming nothing has always had here — rather than the refusal a `%`
// spec gets, because the two spellings ask different questions and only one
// of them is about the table.
func TestWaitingForAnInheritedJobByNumber(t *testing.T) {
	const src = `/bin/sleep 0.4 & first=$!
(wait "$first"; echo "byid=$?")
wait`
	if got := subshellMonitorRun(t, true, src); !strings.Contains(got, "byid=127") {
		t.Errorf("got %q, want the id refused as not a child", got)
	}
}

// The value's name, because the String is what a refusal prints and what the
// axis sweep reads.
func TestSubshellJobsKeptUnderTheMonitorIsNamed(t *testing.T) {
	if got := SubshellJobsKeptUnderTheMonitor.String(); got != "kept under the monitor" {
		t.Errorf("String() = %q", got)
	}
	// And it is a value of its own rather than a spelling of one of the
	// three that were there before it.
	for _, other := range []SubshellJobTable{
		SubshellJobTableUnspecified, SubshellJobsCleared,
		SubshellJobsKept, SubshellJobsKeptOutsideACompound,
	} {
		if SubshellJobsKeptUnderTheMonitor == other {
			t.Errorf("collides with %v", other)
		}
	}
}

// `fg` and `bg` refuse too, and they take a different door: a shell holding
// nothing but the jobs its parent started is not running a monitor of its
// own, so what they give is the ordinary no-job-control refusal rather than
// the one `wait` and `disown` give.
//
// Runner.JobControl is turned on here and nowhere else in this file, because
// with it off the refusal arrives for a reason that has nothing to do with
// the table — and a case that would pass either way is not a case. With it
// on, a subshell whose guard was missing resumes the parent's job and waits
// it out, which the last assertion is what notices.
func TestFgAndBgRefuseAnInheritedJob(t *testing.T) {
	const src = `set -m
/bin/sleep 0.4 & first=$!
(fg %1; echo "fg=$?")
(bg %1; echo "bg=$?")
jobs -p >t.txt
y=; read y <t.txt
case $y in "$first") echo still;; "") echo gone;; *) echo other;; esac
wait`
	out, _ := run(t, src, func(r *Runner) {
		s := *r.Semantics
		s.SubshellJobTable = SubshellJobsKeptUnderTheMonitor
		// The announcement `&` writes, answered so the refusals below are
		// the only complaints in the output.
		s.AnnouncesBackgroundJob = No
		r.Semantics = &s
		r.Terminal, r.JobControl = true, true
		r.Dir = t.TempDir()
	})
	got := strings.TrimSpace(out)
	for _, want := range []string{
		// Refused, and refused as a shell with no job control refuses —
		// which is the door, and is what says the guard was reached rather
		// than the resume failing later for a reason of its own.
		"no job control", "fg=1", "bg=1",
		// The parent's table, untouched.
		"still",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("got %q, want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "gone") {
		t.Errorf("a subshell resumed the parent's job:\n%s", got)
	}
	// And it wrote nothing. `fg` names the job on the way in — that is how a
	// shell says which job it just put back in front of you — so a guard
	// that was not reached leaves the parent's command line on stdout of a
	// subshell that then failed anyway, which is #2657's failure one
	// boundary over.
	if strings.Contains(got, "\n/bin/sleep 0.4\n") {
		t.Errorf("the subshell announced the parent's job before failing:\n%s", got)
	}
}
