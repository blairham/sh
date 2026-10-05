// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// spellRun is lookRun with the spelling axis chosen.
func spellRun(t *testing.T, dir, path, src string, spelling PathHitSpelling) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.PathHitSpelled = spelling
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh",
		Vars: map[string]string{"PATH": path},
		Env:  []string{"PATH=" + path},
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return buf.String()
}

// Semantics.PathHitSpelled: a PATH hit is written back one of three ways,
// and the command hash keeps that spelling, so `command -v` says the same
// thing after the command has run as before it (#6044). This file names no
// shell; the panel is on the axis.
func TestAPathHitIsSpelledTheSameBeforeAndAfterItRuns(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	script(t, filepath.Join(dir, "bin"), "zz", "ran", true)
	bin := filepath.Join(dir, "bin")
	for _, tc := range []struct {
		path     string
		spelling PathHitSpelling
		want     string
	}{
		// Nobody cleans a `..`, in any reading.
		{"<d>/../" + filepath.Base(dir) + "/bin", PathHitJoinedOnce, "<d>/../" + filepath.Base(dir) + "/bin/zz"},
		{"<d>/../" + filepath.Base(dir) + "/bin", PathHitAsWritten, "<d>/../" + filepath.Base(dir) + "/bin/zz"},
		{"<d>/../" + filepath.Base(dir) + "/bin", PathHitFromTheWorkingDirectory, "<d>/../" + filepath.Base(dir) + "/bin/zz"},
		// A trailing slash.
		{"<b>/", PathHitJoinedOnce, "<b>/zz"},
		{"<b>/", PathHitAsWritten, "<b>//zz"},
		{"<b>/", PathHitFromTheWorkingDirectory, "<b>/zz"},
		// A relative entry, and an empty one.
		{"bin", PathHitJoinedOnce, "bin/zz"},
		{"bin", PathHitAsWritten, "bin/zz"},
		{"bin", PathHitFromTheWorkingDirectory, "<d>/bin/zz"},
		{"./bin/", PathHitAsWritten, "./bin//zz"},
		{"./bin/", PathHitFromTheWorkingDirectory, "<d>/./bin/zz"},
	} {
		path := strings.NewReplacer("<b>", bin, "<d>", dir).Replace(tc.path)
		want := strings.NewReplacer("<b>", bin, "<d>", dir).Replace(tc.want)
		// Before the run, and in a fresh shell after it: the report must not
		// be what hashed the name, or the run's own entry is never read.
		for _, src := range []string{"command -v zz", "zz >/dev/null; command -v zz"} {
			if got := spellRun(t, dir, path, src, tc.spelling); got != want+"\n" {
				t.Errorf("PATH=%s under %d, %s: got %q, want %q", tc.path, tc.spelling, src, got, want)
			}
		}
	}
}

// And an empty entry, which is the current directory: the bare name, `./`
// and the name, or the directory itself.
func TestAnEmptyPathEntryIsSpelledByTheAxis(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	script(t, dir, "zz", "ran", true)
	for spelling, want := range map[PathHitSpelling]string{
		PathHitJoinedOnce:              "./zz",
		PathHitAsWritten:               "zz",
		PathHitFromTheWorkingDirectory: dir + "/zz",
	} {
		for _, src := range []string{"command -v zz", "zz >/dev/null; command -v zz"} {
			if got := spellRun(t, dir, ":/nonexistent", src, spelling); got != want+"\n" {
				t.Errorf("under %d, %s: got %q, want %q", spelling, src, got, want)
			}
		}
	}
}
