// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// The list an interactive session starts with, which reaches the dialect
// through interp.Runner.SeedHistoryEntries rather than through the read a
// script's first `set -o history` does. The two used to leave the shell in
// different states, and `history -n` is what told them apart.

// historySeedRun runs src with $F naming a history file holding lines, with
// those same lines seeded the way a front end seeds them.
//
// Register is the seam a test has here: it is the call driver makes to hand
// the dialect a new Runner, so a seed added behind it lands at the moment
// repl's does — before the first line runs and after the builtin is
// registered. See repl.Shell.Run, where the seed and the file are read
// together. HISTFILE comes from the environment for the same reason: an
// assignment on the script's first line would be one line too late, the seed
// having already run.
//
// No `set -o history` anywhere, which is the point of the harness rather
// than an omission: turning the list on is the *other* route to a file, and
// historyStartFile marks it read on its own — so a script that used it could
// not tell whether the seed had marked anything. The list the assertions
// read is therefore the seeded lines and whatever `-n` adds, and the
// script's own lines are not in it.
func historySeedRun(t *testing.T, src string, lines ...string) string {
	t.Helper()
	return historySeedFileRun(t, src, lines, lines)
}

// historySeedFileRun is the same with the file's *physical* lines and the
// entries a front end decoded out of them told apart, which they are wherever
// the file carries times.
func historySeedFileRun(t *testing.T, src string, lines, seeded []string) string {
	t.Helper()
	dir := t.TempDir()
	f := filepath.Join(dir, "hf")
	if err := os.WriteFile(f, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HISTFILE", f)
	path := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(src, "$F", f)), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	sh := bashShell(&out, &errs)
	register := sh.Register
	sh.Register = func(r *interp.Runner) {
		register(r)
		r.SeedHistoryEntries(seeded)
	}
	if code := driver.MainArgs(sh, []string{"bash", path}); code != 0 || errs.Len() != 0 {
		t.Fatalf("status %d with %q on stderr", code, errs.String())
	}
	return strings.ReplaceAll(out.String(), f, "$F")
}

// Measured 2026-09-23 on bash 5.3.20, interactive against a three-line
// HISTFILE with `history -n` and `history` typed: the three lines are listed
// once. This shell listed them twice, the seeded session having left the
// file marked as read from nothing (#4363).
func TestASeededSessionHasAlreadyReadItsHistoryFile(t *testing.T) {
	out := historySeedRun(t, "history -n\nhistory\n", "alpha one", "beta two", "gamma three")
	want := "    1  alpha one\n    2  beta two\n    3  gamma three\n"
	if out != want {
		t.Errorf("listing\n%q\nwant\n%q", out, want)
	}
}

// And the other half of what the letter means, which is the half a mark laid
// down too far along would break: `-n` still reads what the file has grown by
// since. Measured on the same shell, the fourth line reaching the list.
func TestASeededSessionStillReadsWhatTheFileGrewBy(t *testing.T) {
	out := historySeedRun(t, "echo 'delta four' >> $F\nhistory -n\nhistory\n",
		"alpha one", "beta two", "gamma three")
	want := "    1  alpha one\n    2  beta two\n    3  gamma three\n    4  delta four\n"
	if out != want {
		t.Errorf("listing\n%q\nwant\n%q", out, want)
	}
}

// A file carrying times is where a mark counted in entries and one counted in
// physical lines come apart: `#<epoch>` is a line the reader hands back no
// entry for (#4013), so two commands are four lines. A mark of two would
// leave `-n` reading the second command a second time, which is the shape
// this pins and the reason historySeedReadMark asks the file rather than
// counting what it was handed.
func TestTheSeedsMarkCountsTheFilesLinesAndNotItsEntries(t *testing.T) {
	out := historySeedFileRun(t, "history -n\nhistory\n",
		[]string{"#100", "alpha one", "#200", "beta two"},
		[]string{"alpha one", "beta two"})
	if want := "    1  alpha one\n    2  beta two\n"; out != want {
		t.Errorf("listing\n%q\nwant\n%q", out, want)
	}
}
