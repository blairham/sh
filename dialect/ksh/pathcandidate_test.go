// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// This shell reports the failure of the **last PATH entry it really
// searched** rather than the first interesting candidate, which no probe with
// one PATH entry can see: with the directory alone, this column and zsh are
// byte for byte the same decision.
//
// Measured 2026-09-16 and again 2026-09-18 on ksh93u+ 2012-08-01, script
// files under `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin `/dev/null`.
// The fourth and fifth rows are the sharp ones: the same two entries in the
// two orders, answered differently (#3249).
func TestThePathCandidateReportedIsTheLastEntrySearched(t *testing.T) {
	base := t.TempDir()
	mk := func(name string, build func(dir string)) string {
		dir := filepath.Join(base, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		build(dir)
		return dir
	}
	isDir := mk("isdir", func(dir string) {
		if err := os.MkdirAll(filepath.Join(dir, "zzcmd"), 0o755); err != nil {
			t.Fatal(err)
		}
	})
	noExec := mk("noexec", func(dir string) {
		if err := os.WriteFile(filepath.Join(dir, "zzcmd"), []byte("#!/bin/sh\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	empty := mk("empty", func(string) {})
	gone := filepath.Join(base, "nosuchdir")

	for _, c := range []struct {
		name, path, want string
		status           int
	}{
		{"the directory alone", isDir, "zzcmd: cannot execute [Is a directory]\n", 126},
		{"an entry in front of it", empty + ":" + isDir, "zzcmd: cannot execute [Is a directory]\n", 126},
		{"an entry behind it", isDir + ":" + empty, "zzcmd: not found\n", 127},
		{"a non-executable behind it", isDir + ":" + noExec, "zzcmd: cannot execute [Permission denied]\n", 126},
		{"and the same two the other way round", noExec + ":" + isDir, "zzcmd: cannot execute [Is a directory]\n", 126},
		// An entry that is not an existing directory was never searched, so
		// it does not overwrite what the entry before it left. This is the
		// row an errno test cannot get right: a missing file and a missing
		// directory fail the same way.
		{"a missing entry behind it is not searched", isDir + ":" + gone, "zzcmd: cannot execute [Is a directory]\n", 126},
		{"nor is a relative one", isDir + ":relnope_zz", "zzcmd: cannot execute [Is a directory]\n", 126},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, err := preset.Combined(t, dialecttest.Base{
				Dir: base, Vars: map[string]string{"PATH": c.path},
			}, "zzcmd\n")
			if err != nil {
				t.Fatal(err)
			}
			if !hasSuffixLine(out, c.want) || st != c.status {
				t.Errorf("PATH %s: out %q status %d, want it to end %q at %d", c.name, out, st, c.want, c.status)
			}
		})
	}
}

// hasSuffixLine reports whether out ends in want, which is how these rows are
// asked: the diagnostic carries a location in front of the sentence and the
// sentence is what the axis decides.
func hasSuffixLine(out, want string) bool {
	return len(out) >= len(want) && out[len(out)-len(want):] == want
}
