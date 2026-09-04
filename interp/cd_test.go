// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `cd` onto a file and `cd` onto nothing are two reasons, not one.
//
// This reported "no such directory" for both — a sentence no shell in the
// panel prints, and an answer two of them can tell is wrong.
func TestCdReportsTheReasonTheSystemGave(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "afile", "")
	sem := CoreSemantics()
	dg := Diagnostics{CdCannotChange: "cd: %[1]s: %[2]s"}

	for _, tc := range []struct{ name, src, want string }{
		{"nothing there", "cd ./nope", "no such file or directory"},
		{"not a directory", "cd ./afile", "not a directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, func(r *Runner) {
				r.Semantics, r.Diagnostics, r.Dir = &sem, &dg, dir
			})
			if !strings.Contains(strings.ToLower(out), tc.want) {
				t.Errorf("got %q, want it to contain %q", out, tc.want)
			}
			if st == 0 {
				t.Error("status 0, want a failure")
			}
		})
	}
}

// Two dialects call `cd` with nowhere to go an error and two stay where they
// are and report success.
func TestCdWithNowhereToGoIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		status int
		says   bool
	}{
		{"an error", Yes, 1, true},
		{"quietly nothing", No, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			sem.CdWithoutHomeIsAnError = tc.answer
			dg := Diagnostics{CdHomeNotSet: "cd: HOME not set"}
			out, st := run(t, `unset HOME; cd`, func(r *Runner) {
				r.Semantics, r.Diagnostics = &sem, &dg
			})
			if st != tc.status {
				t.Errorf("status %d, want %d", st, tc.status)
			}
			if said := strings.Contains(out, "HOME not set"); said != tc.says {
				t.Errorf("said %v, want %v (out %q)", said, tc.says, out)
			}
		})
	}
}

// `cd -` announces where it went in three of the four.
func TestCdDashPrintsTheDirectoryIsAnAxis(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name   string
		answer Answer
		want   bool
	}{
		{"announced", Yes, true},
		{"silent", No, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			sem.CdDashPrintsTheDirectory = tc.answer
			out, _ := run(t, `cd /; cd -`, func(r *Runner) {
				r.Semantics, r.Dir = &sem, dir
			})
			if printed := strings.TrimSpace(out) != ""; printed != tc.want {
				t.Errorf("printed %v, want %v (out %q)", printed, tc.want, out)
			}
		})
	}
}

// `unset` takes away a name that came from the environment, which deleting it
// from the shell's own table does not do: a lookup reads both, so `unset PATH`
// left PATH exactly where it was.
func TestUnsetRemovesAnEnvironmentName(t *testing.T) {
	// A real PATH, because two of these run `env` — with a made-up one the
	// command is not found and the assertions pass for the wrong reason,
	// which is what they did until this line changed.
	env := []string{"PATH=" + os.Getenv("PATH"), "GONE=yes"}
	out, _ := run(t, `unset GONE; echo "[${GONE-removed}]"`, func(r *Runner) { r.Env = env })
	if strings.TrimSpace(out) != "[removed]" {
		t.Errorf("got %q, want the name gone", out)
	}
	// And it does not reach a command either, or the child would see what
	// the parent cannot.
	out, _ = run(t, `unset GONE; env`, func(r *Runner) { r.Env = env })
	if strings.Contains(out, "GONE") {
		t.Errorf("the environment still carries it: %q", out)
	}
	// Assigning it brings it back, because a lookup reads the shell's own
	// table before the environment — which is also why nothing has to undo
	// the record.
	out, _ = run(t, `unset GONE; GONE=again; echo "[$GONE]"`, func(r *Runner) { r.Env = env })
	if strings.TrimSpace(out) != "[again]" {
		t.Errorf("got %q, want the name back", out)
	}
	// And an exported one still reaches a command.
	out, _ = run(t, `unset GONE; export GONE=again; env`, func(r *Runner) { r.Env = env })
	if !strings.Contains(out, "GONE=again") {
		t.Errorf("the child should see the new value: %q", out)
	}
}

// TestPwdPathOptions — `pwd -P` reports where the directory is, a plain
// `pwd` the name it was reached by, and the last of `-L -P` decides. All
// three are unanimous across the panel.
func TestPwdPathOptions(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("a", "b"), filepath.Join(dir, "l")); err != nil {
		t.Fatal(err)
	}
	sem := CoreSemantics()
	for _, tc := range []struct {
		src  string
		want string
	}{
		{`cd l; pwd`, "/l"},
		{`cd l; pwd -P`, "/a/b"},
		{`cd l; pwd -L -P`, "/a/b"},
		{`cd l; pwd -P -L`, "/l"},
	} {
		out, st := run(t, tc.src, func(r *Runner) {
			r.Semantics, r.Dir = &sem, dir
		})
		if st != 0 {
			t.Errorf("%s: status %d (output %q)", tc.src, st, out)
			continue
		}
		if !strings.HasSuffix(strings.TrimSpace(out), tc.want) {
			t.Errorf("%s = %q, want suffix %q", tc.src, strings.TrimSpace(out), tc.want)
		}
	}
}
