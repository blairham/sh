// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

// TestACommandIsStartedByTheSpellingItWasFoundBy is #6090: the path handed to
// the kernel is the one the shell joined, and a `#!` script receives it as
// `$0`. It was the absolute path the lookup resolved, so every row below read
// `<dir>/z0` whatever was typed or searched.
//
// Each row varies one thing: the word typed with a slash in it, a relative
// PATH entry, a `.` entry under each answer of
// Semantics.PathHitFromTheCurrentDirectoryRunsBare, and the same `.` entry on
// a second run, when the table answers rather than the search.
func TestACommandIsStartedByTheSpellingItWasFoundBy(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(filepath.Join(sub, "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "z0"), []byte("#!/bin/sh\necho \"0=$0\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, src string
		bare      Answer
		spelled   PathHitSpelling
		want      string
	}{
		{"typed", "./deep/../z0; ../sub//z0", No, PathHitAsWritten, "0=./deep/../z0\n0=../sub//z0\n"},
		{"relative entry", "PATH=deep/..; z0", No, PathHitAsWritten, "0=deep/../z0\n"},
		{"dot entry written", "PATH=.; z0; z0", No, PathHitAsWritten, "0=./z0\n0=./z0\n"},
		{"dot entry bare", "PATH=.; z0; z0", Yes, PathHitAsWritten, "0=z0\n0=z0\n"},
		{"empty entry joined", "PATH=:; z0", No, PathHitJoinedOnce, "0=./z0\n"},
		// The reading whose written-back spelling is absolute, where the
		// second run used to read its own table entry as a copy that had
		// appeared in front of it — `$PWD/./z0` against `$PWD/z0`.
		{"dot entry bare from the directory", "PATH=.; z0; z0", Yes, PathHitFromTheWorkingDirectory, "0=z0\n0=z0\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := run(t, c.src, func(r *Runner) {
				sem := PosixSemantics()
				sem.PathHitFromTheCurrentDirectoryRunsBare = c.bare
				sem.PathHitSpelled = c.spelled
				sem.HashedPathShadowsAnEarlierDirectory = No
				r.Semantics, r.Dir = &sem, sub
			})
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
