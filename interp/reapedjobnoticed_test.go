// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"
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
	afterAJobsProgramIsReaped = func() { time.Sleep(300 * time.Millisecond) }
	t.Cleanup(func() { afterAJobsProgramIsReaped = nil })
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
