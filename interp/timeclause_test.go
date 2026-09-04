// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// The figures `time` prints are real timings and cannot be asserted on; the
// shapes can, which is the same discipline the `times` tests use. Every test
// here keeps the two streams apart, because which stream the report lands on
// — and that no redirection inside the pipeline moves it — is the measured
// fact the keyword exists to honor.

// timeRun runs src under d with the given vectors, streams kept apart.
func timeRun(t *testing.T, src string, d syntax.Dialect, sem Semantics, dg Diagnostics,
	setup func(*Runner),
) (out, errs string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	r := &Runner{Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &dg, Name: "testsh"}
	if setup != nil {
		setup(r)
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return o.String(), e.String(), st
}

// timeSem answers the axes a timed pipeline can reach, so a test about the
// report is not tripped by an unanswered pipeline question.
func timeSem() Semantics {
	s := permissive()
	s.LastPipelineElementInCurrentShell = No
	return s
}

var defaultReport = regexp.MustCompile(`^\nreal\t\d+m\d+\.\d{3}s\nuser\t\d+m\d+\.\d{3}s\nsys\t\d+m\d+\.\d{3}s\n$`)

// TestTimeReportsOnTheShellsStderr pins the fact the issue was filed over: an
// element's redirection does not catch the report. `time true 2>&1` sends the
// element's stderr to stdout, and the report still lands on the shell's own.
func TestTimeReportsOnTheShellsStderr(t *testing.T) {
	out, errs, st := timeRun(t, `time true 2>&1`, syntax.Core(), timeSem(), Diagnostics{}, nil)
	if st != 0 {
		t.Errorf("status = %d, want the pipeline's 0", st)
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty — the element's redirect must not catch the report", out)
	}
	if !defaultReport.MatchString(errs) {
		t.Errorf("stderr = %q, want a blank line then real/user/sys with three decimals", errs)
	}
}

// TestTimeTimesTheWholePipeline: one report for the pipeline, and the status
// is the pipeline's own — its last element's.
func TestTimeTimesTheWholePipeline(t *testing.T) {
	out, errs, st := timeRun(t, `time true | false; echo "st=$?"`, syntax.Core(), timeSem(), Diagnostics{}, nil)
	if out != "st=1\n" {
		t.Errorf("stdout = %q, want st=1 — the timed pipeline's status is its last element's", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	if !defaultReport.MatchString(errs) {
		t.Errorf("stderr = %q, want exactly one report for the whole pipeline", errs)
	}
}

// TestTimeDecimalsAreTheDialects: same layout, a different number of decimal
// places — the whole difference between the two dialects that share it.
func TestTimeDecimalsAreTheDialects(t *testing.T) {
	twoDecimals := regexp.MustCompile(`^\nreal\t\d+m\d+\.\d{2}s\nuser\t\d+m\d+\.\d{2}s\nsys\t\d+m\d+\.\d{2}s\n$`)
	_, errs, _ := timeRun(t, `time true`, syntax.Core(), timeSem(), Diagnostics{TimeDecimals: 2}, nil)
	if !twoDecimals.MatchString(errs) {
		t.Errorf("stderr = %q, want two decimal places", errs)
	}
}

// TestTimePosixFlagFormat: `-p` selects the POSIX line format — seconds only,
// one space, two decimals, no leading blank line — wherever the dialect reads
// the flag at all.
func TestTimePosixFlagFormat(t *testing.T) {
	d := syntax.Core()
	d.TimePosixFlag = true
	posix := regexp.MustCompile(`^real \d+\.\d{2}\nuser \d+\.\d{2}\nsys \d+\.\d{2}\n$`)
	_, errs, st := timeRun(t, `time -p true`, d, timeSem(), Diagnostics{}, nil)
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	if !posix.MatchString(errs) {
		t.Errorf("stderr = %q, want the POSIX format", errs)
	}
}

// TestTimePerCommandStaysSilentWithoutAFork: the per-element layout reports
// only elements that ran an external — a lone builtin prints nothing at all,
// which is what the shell this layout was measured from does for an element
// that did not fork.
func TestTimePerCommandStaysSilentWithoutAFork(t *testing.T) {
	out, errs, st := timeRun(t, `time true`, syntax.Core(), timeSem(),
		Diagnostics{TimeLayout: TimePerCommand}, nil)
	if st != 0 || out != "" {
		t.Errorf("status %d stdout %q, want 0 and empty", st, out)
	}
	if errs != "" {
		t.Errorf("stderr = %q, want nothing — no element forked", errs)
	}
}

// TestTimePerCommandReportsAnExternal: an element that ran an external gets
// one line, labeled with the element as written, in the measured shape.
func TestTimePerCommandReportsAnExternal(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "x")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	line := regexp.MustCompile(`^x  \d+\.\d{2}s user \d+\.\d{2}s system \d+% cpu \d+\.\d{3} total\n$`)
	_, errs, st := timeRun(t, `time x`, syntax.Core(), timeSem(),
		Diagnostics{TimeLayout: TimePerCommand},
		func(r *Runner) { r.Env = []string{"PATH=" + dir} })
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	if !line.MatchString(errs) {
		t.Errorf("stderr = %q, want one per-command line labeled `x`", errs)
	}
}

// TestBareTimeReportsAndResetsTheStatus: `time` with no pipeline still
// reports — a run of nothing under the default answer — and leaves 0 behind
// whatever came before it. Measured, unanimous among the shells that have
// the keyword.
func TestBareTimeReportsAndResetsTheStatus(t *testing.T) {
	out, errs, _ := timeRun(t, `false; time; echo "st=$?"`, syntax.Core(), timeSem(), Diagnostics{}, nil)
	if out != "st=0\n" {
		t.Errorf("stdout = %q, want st=0 — a bare time resets the status", out)
	}
	if !defaultReport.MatchString(errs) {
		t.Errorf("stderr = %q, want the ordinary report over a run of nothing", errs)
	}
}

// TestBareTimeLayoutsAreTheDialects covers the two answers that are not a run
// of nothing: the shell's own user and sys with no real, and a shell line
// with a children line in the per-command shape.
func TestBareTimeLayoutsAreTheDialects(t *testing.T) {
	userSys := regexp.MustCompile(`^user\t\d+m\d+\.\d{2}s\nsys\t\d+m\d+\.\d{2}s\n$`)
	_, errs, _ := timeRun(t, `time`, syntax.Core(), timeSem(),
		Diagnostics{TimeBare: TimeBareShellUserSys, TimeDecimals: 2}, nil)
	if !userSys.MatchString(errs) {
		t.Errorf("stderr = %q, want labeled user and sys lines and nothing else", errs)
	}

	shellChildren := regexp.MustCompile(`^shell  .* total\nchildren  .* total\n$`)
	_, errs, _ = timeRun(t, `time`, syntax.Core(), timeSem(),
		Diagnostics{TimeBare: TimeBareShellAndChildren}, nil)
	if !shellChildren.MatchString(errs) {
		t.Errorf("stderr = %q, want a shell line and a children line", errs)
	}
}

// TestTimeNegationOnEitherSide: a bang before or after `time` inverts the
// pipeline's status, and the report still prints.
func TestTimeNegationOnEitherSide(t *testing.T) {
	out, errs, _ := timeRun(t, `time ! false; echo "st=$?"`, syntax.Core(), timeSem(), Diagnostics{}, nil)
	if out != "st=0\n" {
		t.Errorf("`time ! false`: stdout = %q, want st=0", out)
	}
	if !defaultReport.MatchString(errs) {
		t.Errorf("`time ! false`: stderr = %q, want the report", errs)
	}

	out, errs, _ = timeRun(t, `! time true; echo "st=$?"`, syntax.Core(), timeSem(), Diagnostics{}, nil)
	if out != "st=1\n" {
		t.Errorf("`! time true`: stdout = %q, want st=1", out)
	}
	if !defaultReport.MatchString(errs) {
		t.Errorf("`! time true`: stderr = %q, want the report", errs)
	}
}
