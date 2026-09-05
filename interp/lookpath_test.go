// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// These name no shell. Every rule here was measured across the panel and is
// unanimous, so there is no axis to be had — only the wording differs, and
// that belongs in dialect/.

// script writes an executable that prints what it is, and returns its
// directory.
func script(t *testing.T, dir, name, says string, executable bool) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	body := "#!/bin/sh\necho " + says + "\n"
	mode := os.FileMode(0o600)
	if executable {
		mode = 0o755
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

// lookRun runs src with an explicit PATH and working directory, and no
// inherited environment beyond what the test sets. That is the point: a test
// that let the process's PATH through would pass no matter what the code did.
func lookRun(t *testing.T, dir, path, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh",
		Vars: map[string]string{"PATH": path},
		// An empty Env is not nil: nil means "the process's own", and this
		// whole file is about not borrowing that.
		Env: []string{"PATH=" + path},
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error(), -1
	}
	return buf.String(), st
}

// TestTheRunnersPathGovernsLookup is the bug this file exists for.
//
// os/exec's LookPath reads the *process's* PATH. A Runner holds its own, and a
// shell where setting PATH does nothing is not a shell — so the lookup has to
// ask the runner. Before this, the assertion below failed and the one after it
// passed for the wrong reason.
func TestTheRunnersPathGovernsLookup(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	script(t, bin, "mycmd", "on-my-path", true)

	out, st := lookRun(t, dir, bin, `mycmd`)
	if st != 0 || strings.TrimSpace(out) != "on-my-path" {
		t.Errorf("output = %q status %d, want on-my-path", out, st)
	}
}

// TestAnEmptyPathFindsNothing is the same bug from the other side, and the
// worse half: a script that clears PATH to control what it can reach must not
// still reach everything on the machine.
func TestAnEmptyPathFindsNothing(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	script(t, bin, "mycmd", "reached", true)

	// Present when PATH has it…
	if out, _ := lookRun(t, dir, bin, `mycmd`); !strings.Contains(out, "reached") {
		t.Fatalf("setup: expected to find the command, got %q", out)
	}
	// …and gone when it does not. The command lives in a *subdirectory*, which
	// is the arrangement where the panel agrees: an empty PATH is one empty
	// element in three of the four, so a command in the current directory
	// would still be found and would prove the opposite of what this asserts.
	out, st := lookRun(t, dir, "", `mycmd`)
	if strings.Contains(out, "reached") {
		t.Errorf("an empty PATH must not reach a subdirectory: %q", out)
	}
	if st != 127 {
		t.Errorf("status = %d, want 127", st)
	}
}

func TestABuiltinIsNotLookedUpAtAll(t *testing.T) {
	// The contrast that makes the previous test a finding rather than a
	// broken shell.
	out, st := lookRun(t, t.TempDir(), "", `echo builtin-ok`)
	if st != 0 || strings.TrimSpace(out) != "builtin-ok" {
		t.Errorf("output = %q status %d, want builtin-ok", out, st)
	}
}

func TestPathIsSearchedInOrder(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	script(t, a, "dup", "from-a", true)
	script(t, b, "dup", "from-b", true)

	if out, _ := lookRun(t, dir, a+":"+b, `dup`); strings.TrimSpace(out) != "from-a" {
		t.Errorf("output = %q, want from-a", out)
	}
	if out, _ := lookRun(t, dir, b+":"+a, `dup`); strings.TrimSpace(out) != "from-b" {
		t.Errorf("output = %q, want from-b", out)
	}
}

// TestAnEmptyElementIsTheRunnersDirectory covers two things at once: an empty
// PATH element means the current directory, and *which* current directory that
// is. It is the runner's, not the process's — a Runner given a Dir of its own
// must not resolve a relative PATH entry against wherever the program happens
// to have been started.
func TestAnEmptyElementIsTheRunnersDirectory(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	script(t, a, "dup", "from-a", true)
	script(t, dir, "dup", "from-the-runners-dir", true)

	out, _ := lookRun(t, dir, ":"+a, `dup`)
	if strings.TrimSpace(out) != "from-the-runners-dir" {
		t.Errorf("output = %q, want the runner's own directory to win", out)
	}
}

func TestARelativeElementResolvesAgainstTheRunnersDirectory(t *testing.T) {
	dir := t.TempDir()
	script(t, filepath.Join(dir, "sub"), "relcmd", "from-sub", true)

	out, st := lookRun(t, dir, "sub", `relcmd`)
	if st != 0 || strings.TrimSpace(out) != "from-sub" {
		t.Errorf("output = %q status %d, want from-sub", out, st)
	}
}

// TestANonExecutableDoesNotEndTheSearch is the rule an implementation that
// stops at the first name match gets wrong.
func TestANonExecutableDoesNotEndTheSearch(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	script(t, a, "dup", "from-a", false) // present, not executable
	script(t, b, "dup", "from-b", true)

	out, st := lookRun(t, dir, a+":"+b, `dup`)
	if st != 0 || strings.TrimSpace(out) != "from-b" {
		t.Errorf("output = %q status %d, want from-b: the walk must carry on", out, st)
	}
}

// TestADirectoryIsSkippedToo — a directory with the command's name is not the
// command.
func TestADirectoryIsSkippedToo(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := os.MkdirAll(filepath.Join(a, "dup"), 0o755); err != nil {
		t.Fatal(err)
	}
	script(t, b, "dup", "from-b", true)

	out, st := lookRun(t, dir, a+":"+b, `dup`)
	if st != 0 || strings.TrimSpace(out) != "from-b" {
		t.Errorf("output = %q status %d, want from-b", out, st)
	}
}

// TestACommandWordSeparatesMissingFromUnrunnable is the distinction a single
// "command not found" collapses: 127 for a name that resolved to nothing, 126
// for a file that is there and will not start.
func TestACommandWordSeparatesMissingFromUnrunnable(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	script(t, bin, "noexec", "never", false)
	if err := os.MkdirAll(filepath.Join(dir, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	script(t, dir, "cannotrun", "never", false)

	for _, tc := range []struct {
		name string
		src  string
		want int
	}{
		{"a bare name that is nowhere", `nosuchcmd-xyz`, 127},
		{"a path that does not exist", `./nope`, 127},
		{"a bare name found but not executable", `noexec`, 126},
		{"a path that will not run", `./cannotrun`, 126},
		{"a directory named as a command", `./adir`, 126},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, st := lookRun(t, dir, bin, tc.src)
			if st != tc.want {
				t.Errorf("status = %d, want %d", st, tc.want)
			}
		})
	}
}

// TestASlashBypassesPath — a name with a separator is a path, and PATH is not
// consulted at all.
func TestASlashBypassesPath(t *testing.T) {
	dir := t.TempDir()
	script(t, filepath.Join(dir, "sub"), "direct", "by-path", true)

	out, st := lookRun(t, dir, "", `./sub/direct`)
	if st != 0 || strings.TrimSpace(out) != "by-path" {
		t.Errorf("output = %q status %d, want by-path even with an empty PATH", out, st)
	}
}

// TestAFoundCommandIsRunnableByOsExec is the trap the corpus caught and the
// unit tests missed.
//
// os/exec runs LookPath *again* on any path with no separator in it. An empty
// PATH element means the current directory, and filepath.Join(".", "dup") is
// plain "dup" — so a command found that way went back through the *process's*
// PATH and died with Go's own "executable file not found in $PATH". Resolving
// to an absolute path is what makes what lookPath returns actually runnable.
func TestAFoundCommandIsRunnableByOsExec(t *testing.T) {
	dir := t.TempDir()
	script(t, dir, "viaempty", "ran-via-empty-element", true)

	// An empty PATH element is the only way to reach the bare-name form.
	// `PATH=":"` rather than `PATH=""`: two empty elements is unanimous, where
	// an empty *variable* is the one thing the panel splits on.
	out, st := lookRun(t, dir, ":", `viaempty`)
	if strings.Contains(out, "executable file not found in $PATH") {
		t.Errorf("Go's own lookup error leaked: %q", out)
	}
	if st != 0 || strings.TrimSpace(out) != "ran-via-empty-element" {
		t.Errorf("output = %q status %d, want it to run", out, st)
	}
}

// TestTwoRunnersDoNotShareOnePath is the library property underneath all of
// this. Two Runners in one program each have their own PATH, and neither can
// see the other's — which is the same reason `cd` sets r.Dir instead of
// calling os.Chdir.
func TestTwoRunnersDoNotShareOnePath(t *testing.T) {
	dir := t.TempDir()
	one, two := filepath.Join(dir, "one"), filepath.Join(dir, "two")
	script(t, one, "who", "runner-one", true)
	script(t, two, "who", "runner-two", true)

	outA, _ := lookRun(t, dir, one, `who`)
	outB, _ := lookRun(t, dir, two, `who`)
	if strings.TrimSpace(outA) != "runner-one" {
		t.Errorf("first runner = %q, want runner-one", outA)
	}
	if strings.TrimSpace(outB) != "runner-two" {
		t.Errorf("second runner = %q, want runner-two", outB)
	}
}

// TestAnEmptyPathIsAnAxis covers the one rule here the panel disagrees about.
//
// `PATH=` reads like "nowhere" and is not: an empty PATH is one empty element
// in dash, bash and zsh, and an empty element means the current directory — so
// a command sitting next to the script is still found. ksh93 alone treats it
// as no elements at all.
//
// The command has to be in the runner's *own* directory to tell the answers
// apart. Anywhere else and all four report not-found, which is how a corpus
// case and then a unit test both missed it and "fixed" working code.
// TestADirectoryAsTheOnlyMatchIsAnAxis — the search walks past a directory
// unanimously (TestADirectoryIsSkippedToo), but when the directory was all
// PATH had, the panel splits: report the name as never found, or keep the
// directory as the failed candidate — and among the keepers, which status.
func TestADirectoryAsTheOnlyMatchIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name       string
		axis       Answer
		status     int
		wantStatus int
		named      bool
	}{
		{"skipped silently, never found", No, 0, 127, false},
		{"kept as the candidate", Yes, 0, 126, true},
		{"kept, with the not-found status", Yes, 127, 127, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			a := filepath.Join(dir, "a")
			if err := os.MkdirAll(filepath.Join(a, "dirmatch"), 0o755); err != nil {
				t.Fatal(err)
			}

			f, err := syntax.Parse(`dirmatch`, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			sem := permissive()
			sem.DirectoryOnPathIsACandidate = tc.axis
			dg := Diagnostics{DirectoryOnPathStatus: tc.status}
			r := newTestRunner(t, &Runner{
				Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
				Dir: dir, Name: "testsh",
				Vars: map[string]string{"PATH": a},
				Env:  []string{"PATH=" + a},
			})
			st, rerr := r.Run(context.Background(), f)
			if rerr != nil {
				t.Fatal(rerr)
			}
			if st != tc.wantStatus {
				t.Errorf("status = %d, want %d (output %q)", st, tc.wantStatus, buf.String())
			}
			if named := strings.Contains(buf.String(), "directory"); named != tc.named {
				t.Errorf("names the directory = %v, want %v (output %q)", named, tc.named, buf.String())
			}
		})
	}
}

func TestAnEmptyPathIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name  string
		axis  Answer
		found bool
	}{
		{"an empty PATH is the current directory", Yes, true},
		{"an empty PATH is nowhere", No, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			script(t, dir, "beside", "next-to-the-script", true)

			f, err := syntax.Parse(`beside`, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			sem := permissive()
			sem.EmptyPathIsTheCurrentDirectory = tc.axis
			dg := Diagnostics{}
			r := newTestRunner(t, &Runner{
				Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
				Dir: dir, Name: "testsh",
				Vars: map[string]string{"PATH": ""},
				Env:  []string{"PATH="},
			})
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(buf.String(), "next-to-the-script"); got != tc.found {
				t.Errorf("found = %v, want %v (output %q)", got, tc.found, buf.String())
			}
		})
	}
}
