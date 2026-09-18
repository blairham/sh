// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/internal/testenv"
)

// Which failed candidate a PATH search names once it has found nothing it
// could run — see Semantics.PathCandidateReported.
//
// The fixtures are the ones the panel measurement used: a directory holding a
// *directory* of the name, a directory holding a non-executable file of the
// name, an empty directory, and a name that is on PATH and is not there at
// all. Every row is asked under both answers, which is what makes it a
// measurement of the axis rather than of one column — and the rows where the
// two agree are as load-bearing as the rows where they part, since an
// implementation that simply reported the last thing it touched would fail
// them.
func TestWhichFailedPathCandidateIsReported(t *testing.T) {
	base := t.TempDir()
	mk := func(name string, build func(dir string)) string {
		dir := filepath.Join(base, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		build(dir)
		return dir
	}
	// A directory named zzcmd, which is a candidate that exists and will not
	// run.
	isDir := mk("isdir", func(dir string) {
		if err := os.MkdirAll(filepath.Join(dir, "zzcmd"), 0o755); err != nil {
			t.Fatal(err)
		}
	})
	// A file named zzcmd with no execute bit.
	noExec := mk("noexec", func(dir string) {
		if err := testenv.WriteExecutable(filepath.Join(dir, "zzcmd"), []byte("#!/bin/sh\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	empty := mk("empty", func(string) {})
	gone := filepath.Join(base, "nosuchdir")

	for _, c := range []struct {
		name, path string
		// what each answer reports: "dir", "perm" or "missing".
		first, last string
	}{
		{"the directory alone", isDir, "dir", "dir"},
		{"an empty entry in front of it", empty + ":" + isDir, "dir", "dir"},
		{"an empty entry behind it", isDir + ":" + empty, "dir", "missing"},
		{"a non-executable behind it", isDir + ":" + noExec, "perm", "perm"},
		{"and in front of it", noExec + ":" + isDir, "perm", "dir"},
		{"a missing entry behind it is not searched", isDir + ":" + gone, "dir", "dir"},
		{"an empty entry behind a non-executable", noExec + ":" + empty, "perm", "missing"},
		{"and in front of one", empty + ":" + noExec, "perm", "perm"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, tc := range []struct {
				report PathCandidateReport
				want   string
			}{
				{FirstInterestingCandidate, c.first},
				{LastSearchedEntry, c.last},
			} {
				setup := func(r *Runner) {
					r.Env = []string{"PATH=" + c.path}
					sem := *r.Semantics
					sem.PathCandidateReported = tc.report
					sem.DirectoryOnPathIsACandidate = Yes
					r.Semantics = &sem
				}
				_, st := run(t, `zzcmd 2>/dev/null`, setup)
				got := map[int]string{126: "ran", 127: "missing"}[st]
				if st == 126 {
					// 126 is both readings of "there and will not run", so
					// the wording is what tells them apart.
					out, _ := run(t, `zzcmd 2>&1 >/dev/null`, setup)
					got = "perm"
					if strings.Contains(out, "directory") {
						got = "dir"
					}
				}
				if got != tc.want {
					t.Errorf("report %d, PATH %s: got %q at %d, want %q", tc.report, c.name, got, st, tc.want)
				}
			}
		})
	}
}
