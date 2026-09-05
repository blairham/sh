// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a subshell sees of the jobs its parent started.
//
// The parent's job is a real `sleep`, and every case waits for it, so nothing
// here leaves a process behind. The listing is compared against `$!` rather
// than printed, because a process id is not the same twice.

// subshellJobsRun runs src with one answer to the axis and returns what it
// printed. The job is started, looked at from inside a subshell, and waited
// for, all within src.
func subshellJobsRun(t *testing.T, answer SubshellJobTable, src string) string {
	t.Helper()
	out, _ := run(t, src, func(r *Runner) {
		s := *r.Semantics
		s.SubshellJobTable = answer
		r.Semantics = &s
		// The listing goes to a file, and a relative path is resolved
		// against the Runner's directory. Left unset it is the process's,
		// which under `go test` is the package directory — so these tests
		// wrote s.txt into the source tree, and one of them was committed.
		r.Dir = t.TempDir()
	})
	return out
}

// TestASubshellsViewOfTheParentsJobs pins all three answers against all three
// boundaries the axis distinguishes.
//
// Written as a table of boundaries rather than one case each, because the
// whole point of the axis is that no two answers agree about the same set of
// boundaries: one keeps the parent's jobs everywhere, one nowhere, and one
// keeps them for a simple command and a substitution while clearing them for
// a compound.
func TestASubshellsViewOfTheParentsJobs(t *testing.T) {
	const tail = `; x=; read x <s.txt; case $x in "$first") echo parents;; "") echo none;; *) echo other;; esac; wait`
	for _, b := range []struct {
		name string
		body string
		// want, in the order the answers are listed below.
		cleared, kept, outsideCompound string
	}{
		{
			name: "an explicit subshell",
			body: `(jobs -p) >s.txt`,
			// The one boundary where the third answer sides with the first.
			cleared: "none", kept: "parents", outsideCompound: "none",
		},
		{
			name: "a simple command in a pipeline",
			body: `jobs -p | cat >s.txt`,
			// And here it sides with the second, which is the whole reason
			// two answers were not enough.
			cleared: "none", kept: "parents", outsideCompound: "parents",
		},
		{
			name: "a compound command in a pipeline",
			body: `{ jobs -p; } | cat >s.txt`,
			// The same pipeline with braces around the same builtin.
			cleared: "none", kept: "parents", outsideCompound: "none",
		},
		{
			name:    "a command substitution",
			body:    `printf '%s' "$(jobs -p)" >s.txt`,
			cleared: "none", kept: "parents", outsideCompound: "parents",
		},
	} {
		t.Run(b.name, func(t *testing.T) {
			src := `/bin/sleep 0.4 & first=$!; ` + b.body + tail
			for _, a := range []struct {
				name   string
				answer SubshellJobTable
				want   string
			}{
				{"cleared", SubshellJobsCleared, b.cleared},
				{"kept", SubshellJobsKept, b.kept},
				{"kept outside a compound", SubshellJobsKeptOutsideACompound, b.outsideCompound},
			} {
				t.Run(a.name, func(t *testing.T) {
					if got := strings.TrimSpace(subshellJobsRun(t, a.answer, src)); got != a.want {
						t.Errorf("got %q, want %q", got, a.want)
					}
				})
			}
		})
	}
}

// A background job sees nothing whatever the axis says, which is why that
// boundary does not consult it: the panel is unanimous there, and an axis
// asked where nothing disagrees is an axis that can be answered wrongly.
func TestABackgroundJobNeverSeesTheParentsJobs(t *testing.T) {
	const src = `/bin/sleep 0.4 & first=$!; { /bin/sleep 0.1; jobs -p >s.txt; } & wait; ` +
		`x=; read x <s.txt; case $x in "$first") echo parents;; "") echo none;; *) echo other;; esac`
	for _, a := range []struct {
		name   string
		answer SubshellJobTable
	}{
		{"cleared", SubshellJobsCleared},
		{"kept", SubshellJobsKept},
		{"kept outside a compound", SubshellJobsKeptOutsideACompound},
	} {
		t.Run(a.name, func(t *testing.T) {
			if got := strings.TrimSpace(subshellJobsRun(t, a.answer, src)); got != "none" {
				t.Errorf("got %q, want none whatever the answer", got)
			}
		})
	}
}

// A job the subshell starts for itself is its own, whatever it was handed of
// the parent's. Without this an empty listing would be evidence that `jobs`
// does not work in a subshell rather than that the table was emptied.
func TestASubshellListsAJobItStartedItself(t *testing.T) {
	const src = `/bin/sleep 0.4 & first=$!; (/bin/sleep 0.4 & jobs -p >s.txt; wait); ` +
		`x=; read x <s.txt; case $x in "$first") echo parents;; "") echo none;; *) echo own;; esac; wait`
	for _, a := range []struct {
		name   string
		answer SubshellJobTable
	}{
		{"cleared", SubshellJobsCleared},
		{"kept outside a compound", SubshellJobsKeptOutsideACompound},
	} {
		t.Run(a.name, func(t *testing.T) {
			if got := strings.TrimSpace(subshellJobsRun(t, a.answer, src)); got != "own" {
				t.Errorf("got %q, want the subshell's own job", got)
			}
		})
	}
}

// The axis is not consulted by a script that never backgrounded anything, so
// leaving it unanswered is not a way to break every pipeline.
func TestAnUnansweredSubshellJobAxisIsNotReachedWithoutAJob(t *testing.T) {
	out, st := run(t, `printf hi | cat`, func(r *Runner) {
		s := *r.Semantics
		s.SubshellJobTable = SubshellJobTableUnspecified
		r.Semantics = &s
	})
	if out != "hi" {
		t.Errorf("got %q, want the pipeline to have run", out)
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}

// And a shell that does have a job says so rather than picking a side.
func TestAnUnansweredSubshellJobAxisIsRefused(t *testing.T) {
	out, _ := run(t, `/bin/sleep 0.4 & jobs -p | cat >/dev/null; wait`, func(r *Runner) {
		s := *r.Semantics
		s.SubshellJobTable = SubshellJobTableUnspecified
		r.Semantics = &s
	})
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("got %q, want the axis refused", out)
	}
}
