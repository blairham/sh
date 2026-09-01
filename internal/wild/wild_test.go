// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package wild_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/wild"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// What counts as a shell script is the shebang, read the way the kernel reads
// it: the interpreter's base name, or the word after `env`.
func TestFindReadsTheShebang(t *testing.T) {
	dir := t.TempDir()
	want := map[string]bool{}
	for _, tc := range []struct{ name, head string }{
		{"plain-sh", "#!/bin/sh\n"},
		{"plain-bash", "#!/bin/bash\n"},
		{"with-flags", "#!/bin/bash -e\n"},
		{"via-env", "#!/usr/bin/env bash\n"},
		{"via-env-sh", "#!/usr/bin/env sh\n"},
		{"local-bash", "#!/usr/local/bin/bash\n"},
	} {
		want[write(t, dir, tc.name, tc.head+"echo hi\n")] = true
	}
	for _, tc := range []struct{ name, head string }{
		{"python", "#!/usr/bin/python3\n"},
		{"perl-via-env", "#!/usr/bin/env perl\n"},
		{"no-shebang", "echo hi\n"},
		{"empty", ""},
		{"binary-ish", "\x7fELF\x02\x01\x01\x00"},
	} {
		write(t, dir, tc.name, tc.head)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o700); err != nil {
		t.Fatal(err)
	}

	got := wild.Find([]string{dir, "/nonexistent-directory"})
	if len(got) != len(want) {
		t.Fatalf("found %d scripts, want %d: %v", len(got), len(want), got)
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("found %s, which is not a shell script", p)
		}
	}
}

// A file whose shebang says shell is not necessarily shell. The reference is
// what tells them apart, and a file it also refuses must not be counted
// against this parser — a Tcl program that execs its real interpreter from
// the first line is the usual case.
func TestTheReferenceDecidesWhatIsNotAScript(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "good", "#!/bin/sh\necho hi\n")
	write(t, dir, "bad", "#!/bin/sh\n{ fi; }\n")

	ctx := context.Background()
	// A reference that accepts everything makes the refusal ours.
	rep := wild.Sweep(ctx, []string{dir}, bash.Dialect(), "/usr/bin/true")
	if rep.Scanned != 2 || rep.Parsed != 1 || rep.NotShell != 0 || len(rep.Failures) != 1 {
		t.Errorf("accepting reference: %+v", rep)
	}
	// One that refuses everything makes it the file's.
	rep = wild.Sweep(ctx, []string{dir}, bash.Dialect(), "/usr/bin/false")
	if rep.Scanned != 2 || rep.Parsed != 1 || rep.NotShell != 1 || len(rep.Failures) != 0 {
		t.Errorf("refusing reference: %+v", rep)
	}
	// And no reference at all trusts the shebang.
	rep = wild.Sweep(ctx, []string{dir}, bash.Dialect(), "")
	if len(rep.Failures) != 1 {
		t.Errorf("no reference: %+v", rep)
	}
}

// The sweep reads and never runs, which is what makes it safe to point at
// /usr/bin. A script that would leave a mark if executed must not leave one.
func TestTheSweepDoesNotRunAnything(t *testing.T) {
	dir := t.TempDir()
	mark := filepath.Join(dir, "ran")
	write(t, dir, "sideeffect", "#!/bin/sh\ntouch "+mark+"\n")

	wild.Sweep(context.Background(), []string{dir}, bash.Dialect(), "bash")
	if _, err := os.Stat(mark); err == nil {
		t.Error("the sweep executed a script")
	}
}
