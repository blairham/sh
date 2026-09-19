// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/internal/testenv"
)

// TestADirectoryEarlierOnPathSuppressesALaterNonExecutable is #3578.
//
// Measured 2026-09-18 against bash 5.3.20 at /opt/homebrew, script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin `/dev/null`, in a fresh
// directory. `$d` holds a **directory** named `zzcmd`, `$g` holds a
// **non-executable file** of that name, and `$e` is empty:
//
//	PATH    bash 5.3.20                        this shell, before
//	$d      zzcmd: command not found, 127      same
//	$e:$d   the same                           same
//	$d:$e   the same                           same
//	$g:$d   <path>/zzcmd: Permission denied, 126   same
//	$g:$e   the same                           same
//	$e:$g   the same                           same
//	$d:$g   zzcmd: command not found, 127      **Permission denied, 126**
//
// One row. `Semantics.DirectoryOnPathIsACandidate` is No here, which this
// engine read as "a directory is not a candidate at all", so the walk passed
// over `$d` and the later non-executable became the first interesting
// candidate. Real bash does not pass over it: it keeps the first entry that
// **existed**, whatever it was, and then reports `command not found` when
// that entry was a directory. The `$g:$d` control confirms it from the other
// side — with the file first, bash reports the file.
//
// That is a third reading beside the two `PathCandidateReport` held, and a
// directory alone on PATH cannot see it: this shell and ksh93 agree on every
// row of that shape. See Semantics.PathCandidateReported.
//
// A runnable program later on PATH is unaffected in every row, which is what
// makes a shim directory early on PATH work at all.
func TestADirectoryEarlierOnPathSuppressesALaterNonExecutable(t *testing.T) {
	base := t.TempDir()
	mk := func(name string, build func(dir string)) string {
		t.Helper()
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
		if err := testenv.WriteExecutable(filepath.Join(dir, "zzcmd"),
			[]byte("#!/bin/sh\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	runs := mk("runs", func(dir string) {
		if err := testenv.WriteExecutable(filepath.Join(dir, "zzcmd"),
			[]byte("#!/bin/sh\necho RAN\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	})
	empty := mk("empty", func(string) {})

	for _, c := range []struct {
		name, path, want string
		status           int
	}{
		{"the directory alone", isDir, "command not found", 127},
		{"an empty entry in front of it", empty + ":" + isDir, "command not found", 127},
		{"an empty entry behind it", isDir + ":" + empty, "command not found", 127},
		{"a non-executable in front of it", noExec + ":" + isDir, "Permission denied", 126},
		{"a non-executable and an empty entry", noExec + ":" + empty, "Permission denied", 126},
		{"an empty entry in front of a non-executable", empty + ":" + noExec, "Permission denied", 126},
		// The row this is about.
		{"a non-executable behind it", isDir + ":" + noExec, "command not found", 127},
		// And the search still finds something runnable past either.
		{"something runnable behind it", isDir + ":" + runs, "RAN", 0},
		{"something runnable behind a non-executable", noExec + ":" + runs, "RAN", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, err := preset.Combined(t, dialecttest.Base{
				Dir: base,
				Env: []string{"PATH=" + c.path},
			}, `zzcmd`)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("PATH %s: said %q, want %q in it", c.name, out, c.want)
			}
			if st != c.status {
				t.Errorf("PATH %s: reported %d, want %d", c.name, st, c.status)
			}
		})
	}
}
