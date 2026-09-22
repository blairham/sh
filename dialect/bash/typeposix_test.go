// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"testing"
)

// This shell calls a special builtin *special* only while it is in POSIX
// mode, and the mode is a state rather than a preset: `set -o posix` moves it
// and `set +o posix` puts its own answer back.
//
// `echo` is the control on every line — it is a builtin and is not special,
// so a shell using a longer phrase for every builtin would fail here rather
// than pass.
func TestTypeCallsASpecialBuiltinSpecialOnlyInPosixMode(t *testing.T) {
	dir := t.TempDir()
	out, st := runBashPrelude(t, dir, `type break
type echo
set -o posix
type break
type echo
set +o posix
type break`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	wantWholeLines(t, out,
		"break is a shell builtin",
		"echo is a shell builtin",
		"break is a special shell builtin",
	)
	if n := countLines(out, "break is a special shell builtin"); n != 1 {
		t.Errorf("output = %q, want the special wording exactly once, got %d", out, n)
	}
	if n := countLines(out, "break is a shell builtin"); n != 2 {
		t.Errorf("output = %q, want the plain wording twice, got %d", out, n)
	}
}

// A PATH entry that is relative is written back as the script wrote it.
//
// The path that *runs* has to be absolute — os/exec resolves a path with a
// separator in it against the process's directory rather than the shell's —
// so the two answers are two strings, and this is the one that reaches a
// person. Measured 2026-09-22 on bash 5.3.20.
func TestALookupWritesTheRelativePathEntryBack(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "zzprog"), []byte("#!/bin/sh\necho ran\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, st := runBashPrelude(t, dir, `PATH=.:sub:./sub
cd sub
type -p zzprog
command -v zzprog
cd ..
type -p zzprog
type zzprog`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	wantWholeLines(t, out,
		"./zzprog",
		"sub/zzprog",
		"zzprog is sub/zzprog",
	)
}

func countLines(out, want string) int {
	n := 0
	for line := range splitLines(out) {
		if line == want {
			n++
		}
	}
	return n
}

func splitLines(out string) func(func(string) bool) {
	return func(yield func(string) bool) {
		start := 0
		for i := 0; i <= len(out); i++ {
			if i == len(out) || out[i] == '\n' {
				if !yield(out[start:i]) {
					return
				}
				start = i + 1
			}
		}
	}
}
