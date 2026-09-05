// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The parts of this package only an embedder reaches.
//
// No binary sets any of them, so the conformance harness cannot see them: a
// dialect binary fills in the three vectors and the hooks, and everything
// here is what is left over. What breaks in this file breaks for someone
// building on the package and for nobody running a shell, which is the worst
// way for it to break.

func embed(t *testing.T, r *Runner, src string) int {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	status, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return status
}

// A Runner with nothing filled in but its streams runs a script.
//
// Nothing has to opt in to being usable: no Semantics, no Dialect, no hooks.
// A caller embedding a shell in a program starts here, and if this stops
// working there is no smaller thing to fall back to.
func TestTheZeroValueRuns(t *testing.T) {
	var out, errs strings.Builder
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &errs})
	if status := embed(t, r, "echo hi; x=1; echo $x; /bin/echo external"); status != 0 {
		t.Errorf("status %d, want 0", status)
	}
	if got := out.String(); got != "hi\n1\nexternal\n" {
		t.Errorf("out = %q", got)
	}
	if errs.String() != "" {
		t.Errorf("said %q, want nothing", errs.String())
	}
}

// State handed in before the run is state the script has.
func TestVariablesAndArraysHandedIn(t *testing.T) {
	var out strings.Builder
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Stdout: &out, Stderr: &strings.Builder{},
		Vars:   map[string]string{"PRESET": "value", "PATH": "/bin:/usr/bin"},
		Arrays: map[string]Array{"LIST": {0: "a", 1: "b"}},
	})
	embed(t, r, `echo [$PRESET]; PRESET=changed; echo [$PRESET]; unset PRESET; echo [$PRESET]`+
		"\n"+`echo [${LIST[0]}][${LIST[1]}]`)
	if got := out.String(); got != "[value]\n[changed]\n[]\n[a][b]\n" {
		t.Errorf("out = %q, want a handed-in variable to read, assign and unset like any other", got)
	}
}

// A produced parameter answers freshly each time it is read, and an
// assignment does not stand in front of it.
//
// That is the same rule RANDOM lives by — `RANDOM=5` seeds the generator and
// the next read is still a new number — and it is the reason a producer is a
// separate table rather than a value in Vars.
func TestAProducedParameter(t *testing.T) {
	var out strings.Builder
	sem := PosixSemantics()
	n := 0
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Stdout: &out, Stderr: &strings.Builder{},
		Dynamic: map[string]func(*Runner) string{
			"COUNTER": func(*Runner) string { n++; return string(rune('0' + n)) },
		},
	})
	embed(t, r, "echo [$COUNTER][$COUNTER]; COUNTER=fixed; echo [$COUNTER]; unset COUNTER; echo [$COUNTER]")
	if got := out.String(); got != "[1][2]\n[3]\n[]\n" {
		t.Errorf("out = %q, want fresh each read, unshadowed by assignment, and gone once unset", got)
	}
}

// A registered builtin gets the operands without the name it was called by,
// shadows one of the package's own, and writes where the redirection points.
func TestARegisteredBuiltin(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out")
	var out strings.Builder
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{Semantics: &sem, Stdout: &out, Stderr: &strings.Builder{}})
	var got []string
	r.Register("greet", func(rr *Runner, _ context.Context, args []string) int {
		got = args
		_, _ = rr.Out().Write([]byte("hello " + strings.Join(args, ",") + "\n"))
		return 3
	})
	r.Register("true", func(rr *Runner, _ context.Context, args []string) int {
		_, _ = rr.Out().Write([]byte("shadowed\n"))
		return 0
	})
	if status := embed(t, r, "greet a b"); status != 3 {
		t.Errorf("status %d, want the builtin's own 3", status)
	}
	if strings.Join(got, ",") != "a,b" {
		t.Errorf("args = %q, want the operands without the name", got)
	}
	embed(t, r, "true")
	embed(t, r, "greet x > "+target)
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello x\n" {
		t.Errorf("redirected to %q, want the builtin's output in the file", body)
	}
	if want := "hello a,b\nshadowed\n"; out.String() != want {
		t.Errorf("out = %q, want %q — the second call shadowed a builtin of ours", out.String(), want)
	}
}

// Streams that are not files behave like files.
//
// A shell embedded in a program writes into a buffer, not onto a terminal, and
// os/exec has to copy for a child rather than handing it a descriptor. The
// order that copying produces is the thing to check: a builtin and a program
// writing in turn have to come out in turn.
func TestStreamsThatAreNotFiles(t *testing.T) {
	var out, errs strings.Builder
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{Semantics: &sem, Stdout: &out, Stderr: &errs})
	embed(t, r, "echo one; /bin/echo two; echo three; /bin/sh -c 'echo four >&2'")
	if got := out.String(); got != "one\ntwo\nthree\n" {
		t.Errorf("out = %q, want the builtin and the program in the order they ran", got)
	}
	if got := errs.String(); got != "four\n" {
		t.Errorf("errs = %q", got)
	}
}

// And a reader that is not a file is shared between the shell and a child.
//
// `read` takes a line and the program after it takes the rest. Handing the
// whole reader to each would give the child everything or nothing.
func TestAReaderThatIsNotAFileIsShared(t *testing.T) {
	var out strings.Builder
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{
		Semantics: &sem,
		Stdin:     strings.NewReader("line one\nline two\nline three\n"),
		Stdout:    &out, Stderr: &strings.Builder{},
	})
	embed(t, r, "read a; echo got=$a; /bin/cat")
	if got := out.String(); got != "got=line one\nline two\nline three\n" {
		t.Errorf("out = %q, want the shell to take one line and the child the rest", got)
	}
}

// The working directory a caller hands in is the one everything uses.
func TestTheWorkingDirectoryHandedIn(t *testing.T) {
	dir := t.TempDir()
	var out strings.Builder
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{Semantics: &sem, Dir: dir, Stdout: &out, Stderr: &strings.Builder{}})
	embed(t, r, "pwd; echo hi > relative.txt")
	if got := strings.TrimSpace(out.String()); got != dir {
		t.Errorf("pwd said %q, want %q", got, dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "relative.txt")); err != nil {
		t.Errorf("the relative redirect did not land in the directory: %v", err)
	}
}

// Two Runners in one process each keep their own working directory.
//
// This is the scenario the purity rule protects: a host program embedding two
// shells. The working directory is per Runner rather than per process, so one
// shell's globs, $PWD and `cd` never see or move the other's — which is why
// the core's `cd` sets r.Dir and nothing here ever asks os.Getwd.
func TestTwoRunnersDoNotShareAWorkingDirectory(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	for dir, name := range map[string]string{dirA: "alpha.txt", dirB: "beta.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dirB, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	var outA, outB strings.Builder
	semA, semB := PosixSemantics(), PosixSemantics()
	a := newTestRunner(t, &Runner{Semantics: &semA, Dir: dirA, Stdout: &outA, Stderr: &strings.Builder{}})
	b := newTestRunner(t, &Runner{Semantics: &semB, Dir: dirB, Stdout: &outB, Stderr: &strings.Builder{}})

	embed(t, a, "echo *.txt; echo $PWD")
	embed(t, b, "echo *.txt; echo $PWD")
	if got, want := outA.String(), "alpha.txt\n"+dirA+"\n"; got != want {
		t.Errorf("first runner saw %q, want %q", got, want)
	}
	if got, want := outB.String(), "beta.txt\n"+dirB+"\n"; got != want {
		t.Errorf("second runner saw %q, want %q", got, want)
	}

	// One shell moves; the other must not feel it.
	outA.Reset()
	embed(t, b, "cd sub")
	embed(t, a, "echo *.txt; pwd")
	if got, want := outA.String(), "alpha.txt\n"+dirA+"\n"; got != want {
		t.Errorf("after the other runner's cd, got %q, want %q", got, want)
	}
	if b.Dir != filepath.Join(dirB, "sub") {
		t.Errorf("the runner that moved is at %q, want %q", b.Dir, filepath.Join(dirB, "sub"))
	}
}

// A Runner handed no directory stays relative.
//
// Empty Dir does not mean "wherever the process is, resolved now" — that
// answer is process-wide and asking for it is how two Runners come to share a
// cwd. It means paths stay as written and the operating system resolves each
// use, so `pwd` and $PWD report `.`. A caller that wants absolute answers
// hands one in; driver does.
func TestNoDirectoryHandedInStaysRelative(t *testing.T) {
	var out strings.Builder
	sem := PosixSemantics()
	// testrunner:bare — an unset Dir is the subject here, so this is the one
	// place the helper's directory would be the thing under test going missing.
	// It writes nothing, so there is nothing for it to write into the tree.
	r := &Runner{Semantics: &sem, Stdout: &out, Stderr: &strings.Builder{}}
	embed(t, r, "pwd; echo $PWD")
	if got := out.String(); got != ".\n.\n" {
		t.Errorf("out = %q, want %q", got, ".\n.\n")
	}
}

// The environment a caller hands in is the environment, and export adds to it.
func TestTheEnvironmentHandedIn(t *testing.T) {
	var out strings.Builder
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{
		Semantics: &sem,
		Env:       []string{"ONLY=one", "PATH=/bin:/usr/bin"},
		Stdout:    &out, Stderr: &strings.Builder{},
	})
	embed(t, r, "echo [$ONLY]; export ADDED=x; /usr/bin/env | grep -c '^ADDED=x$'")
	if got := out.String(); got != "[one]\n1\n" {
		t.Errorf("out = %q, want the handed-in value read and the exported one reaching a child", got)
	}
}
