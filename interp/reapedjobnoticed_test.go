// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blairham/sh/syntax"
)

// A background job whose one program has been reaped is a job that has ended,
// whatever the goroutine running it is doing just then (#5611, #5602). The
// hook holds that goroutine open between the reaping and the job's own record
// for longer than the script takes to look, which is the window a loaded
// machine opened by chance — so the test is deterministic in the direction
// that matters: without the wait on the reaped program, `jobs` finds the job
// still running every time.
func TestAReapedProgramsJobIsNoticedBeforeTheTableIsRead(t *testing.T) {
	holdTheWindow(t, &afterAJobsProgramIsReaped, 300*time.Millisecond)
	const src = `/bin/sh -c 'exit 3' & p=$!
/bin/sh -c 'i=0; while kill -0 $1 2>/dev/null && [ $i -lt 3000 ]; do /bin/sleep 0.01; i=$((i+1)); done' poll $p
jobs
echo end`
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	sem.JobsListFinishedJobs = Yes
	sem.JobsShowBackgroundCommand = Yes
	out := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: out, Env: []string{"PATH=/usr/bin:/bin"},
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "Done") || strings.Contains(got, "Running") || !strings.HasSuffix(got, "end\n") {
		t.Errorf("got %q, want the job listed as ended at 3", got)
	}
}

// The same job seen in the earlier window: the kernel has reaped the program,
// so `kill -0` already fails, and the goroutine has not yet said so (#5651).
// Without the kernel's answer, `jobs` finds the job still running every time
// the hook holds this window open.
func TestAProgramTheKernelReapedIsNoticedBeforeTheGoroutineSaysSo(t *testing.T) {
	holdTheWindow(t, &beforeAJobsProgramIsSaidReaped, 300*time.Millisecond)
	const src = `/bin/sh -c 'exit 3' & p=$!
/bin/sh -c 'i=0; while kill -0 $1 2>/dev/null && [ $i -lt 3000 ]; do /bin/sleep 0.01; i=$((i+1)); done' poll $p
jobs
echo end`
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	sem.JobsListFinishedJobs = Yes
	sem.JobsShowBackgroundCommand = Yes
	out := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: out, Env: []string{"PATH=/usr/bin:/bin"},
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "Done") || strings.Contains(got, "Running") || !strings.HasSuffix(got, "end\n") {
		t.Errorf("got %q, want the job listed as ended at 3", got)
	}
}

// And the window before either: the program has exited and nothing has reaped
// it yet, because the goroutine whose wait it is has not run (#5854). A zombie
// still answers `kill -0`, so the script watches /proc for the Z instead, and
// the hook holds the goroutine off its wait for longer than that takes.
// Without the waitid peek, `jobs` finds the job still running every time.
// Linux only, because the peek is: elsewhere the window is the signal's.
func TestAProgramThatExitedUnreapedIsNoticedBeforeTheTableIsRead(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the waitid peek and /proc are Linux's")
	}
	holdTheWindow(t, &beforeAJobsProgramIsWaitedFor, 2*time.Second)
	const src = `/bin/sh -c 'exit 3' & p=$!
/bin/sh -c 'i=0; until grep -q "^[0-9]* ([^)]*) Z" /proc/$1/stat 2>/dev/null || [ $i -ge 1000 ]; do /bin/sleep 0.01; i=$((i+1)); done' poll $p
jobs
echo end`
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	sem.JobsListFinishedJobs = Yes
	sem.JobsShowBackgroundCommand = Yes
	out := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: out, Env: []string{"PATH=/usr/bin:/bin"},
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "Done") || strings.Contains(got, "Running") || !strings.HasSuffix(got, "end\n") {
		t.Errorf("got %q, want the job listed as ended at 3", got)
	}
}

// The wait is for a job whose program was its whole body, and only for that:
// a function the job calls runs a program and goes on to the next, so that
// program's end is not the job's. The second program here runs for seconds,
// and `jobs` must not wait for it — a wait keyed on any program the job ran
// would hold the shell until the function returned.
func TestAProgramAFunctionRunsIsNotTheJobsEnd(t *testing.T) {
	const src = `f() { /bin/sh -c 'exit 0'; /bin/sleep 5; }
f &
/bin/sleep 0.3
jobs
kill %1
echo end`
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	sem.JobsListFinishedJobs = Yes
	sem.JobsShowBackgroundCommand = Yes
	out := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: out, Env: []string{"PATH=/usr/bin:/bin"},
	})
	begun := time.Now()
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(begun); took > 3*time.Second || !strings.Contains(out.String(), "Running") {
		t.Errorf("took %v and wrote %q, want the job listed running at once", took, out.String())
	}
}

// The same, where the function's program has ended and nothing it does next
// starts another at its own depth: it waits on a job of its own. A pid keyed
// on any program the job ran would be the ended one, and `jobs` would wait
// for the function to return (#5651).
func TestAProgramAFunctionRanThatEndedIsNotTheJobsEnd(t *testing.T) {
	const src = `f() { /bin/sh -c 'exit 0'; /bin/sleep 4 & wait; }
f &
/bin/sleep 0.3
jobs
kill %1
echo end`
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	sem.JobsListFinishedJobs = Yes
	sem.JobsShowBackgroundCommand = Yes
	out := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: out, Env: []string{"PATH=/usr/bin:/bin"},
	})
	begun := time.Now()
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(begun); took > 2*time.Second || !strings.Contains(out.String(), "Running") {
		t.Errorf("took %v and wrote %q, want the job listed running at once", took, out.String())
	}
}

// holdTheWindow sets a window hook to hold a job's goroutine for d, until the
// test ends.
func holdTheWindow(t *testing.T, hook *atomic.Pointer[func()], d time.Duration) {
	t.Helper()
	hold := func() { time.Sleep(d) }
	hook.Store(&hold)
	t.Cleanup(func() { hook.Store(nil) })
}
