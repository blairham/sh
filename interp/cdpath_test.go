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

// `cd -L` and `cd -P`, which decide whether the shell remembers the name a
// directory was reached by or where it actually is.
//
// Unanimous across the panel, measured through a symlink: the default and
// `-L` print the link's path afterwards and `-P` prints the real one. We had
// neither, and a leading dash word was read as somewhere to go — `cd -L /tmp`
// looked for a directory called `-L`.
func TestCdKeepsOrResolvesTheNameItWasGiven(t *testing.T) {
	for _, c := range []struct {
		name     string
		cmd      string
		resolved bool
	}{
		{"the default keeps the link", "cd link/sub", false},
		{"and so does -L", "cd -L link/sub", false},
		{"-P resolves it", "cd -P link/sub", true},
		{"-- ends the options", "cd -- link/sub", false},
		{"clustered, resolving", "cd -LP link/sub", true},
		{"clustered, keeping", "cd -PL link/sub", false},
		{"repeated", "cd -PP link/sub", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, real := cdTree(t)
			sem := PosixSemantics()
			sem.CdRefusesUnknownOption = Yes
			sem.CdLastPathOptionWins = Yes
			out := &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
				Stdout: out, Stderr: &strings.Builder{},
			})
			runCd(t, r, c.cmd+"\npwd\n")
			got := strings.TrimSpace(out.String())
			if isResolved := !strings.Contains(got, "link"); isResolved != c.resolved {
				t.Errorf("%s left %q; resolved = %v, want %v", c.cmd, got, isResolved, c.resolved)
			}
			if c.resolved && !strings.HasPrefix(got, real) {
				t.Errorf("%s left %q, want it under %q", c.cmd, got, real)
			}
		})
	}
}

// Which of the two decides when both are given. Three of the panel let the
// last one win; zsh gives `-P` the answer wherever it appears.
func TestCdMayLetTheLastPathOptionWinOrNot(t *testing.T) {
	for _, c := range []struct {
		name     string
		cmd      string
		lastWins Answer
		resolved bool
	}{
		{"last wins, keeping", "cd -P -L link/sub", Yes, false},
		{"last wins, resolving", "cd -L -P link/sub", Yes, true},
		{"-P wins wherever it is", "cd -P -L link/sub", No, true},
		{"and in the other order", "cd -L -P link/sub", No, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, _ := cdTree(t)
			sem := PosixSemantics()
			sem.CdRefusesUnknownOption = Yes
			sem.CdLastPathOptionWins = c.lastWins
			out := &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
				Stdout: out, Stderr: &strings.Builder{},
			})
			runCd(t, r, c.cmd+"\npwd\n")
			got := strings.TrimSpace(out.String())
			if isResolved := !strings.Contains(got, "link"); isResolved != c.resolved {
				t.Errorf("%s left %q; resolved = %v, want %v", c.cmd, got, isResolved, c.resolved)
			}
		})
	}
}

// A letter `cd` does not have. Three of the panel refuse it; one reads the
// word as somewhere to go, because its `cd` takes two operands and a leading
// dash word is the first of them there.
func TestCdMayRefuseAnOptionItDoesNotHave(t *testing.T) {
	for _, c := range []struct {
		name    string
		refuses Answer
		want    string
		unwant  string
		status  int
	}{
		// Asserted from both sides. "cd: -Q: invalid option" contains the
		// word `-Q` as surely as the other message does, so checking only
		// that the operand is named would pass whichever the code did — and
		// did, until a mutant that always refused walked straight through.
		{"refused as an option", Yes, "invalid option", "No such file", 2},
		{"read as somewhere to go", No, "No such file", "invalid option", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, _ := cdTree(t)
			sem := PosixSemantics()
			sem.CdRefusesUnknownOption = c.refuses
			sem.CdLastPathOptionWins = Yes
			errs := &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
				Stdout: &strings.Builder{}, Stderr: errs,
			})
			runCd(t, r, "cd -Q link/sub\n")
			if got := errs.String(); !strings.Contains(got, c.want) || strings.Contains(got, c.unwant) {
				t.Errorf("said %q, want %q in it and %q not", got, c.want, c.unwant)
			}
			if got := r.ExitStatus(); got != c.status {
				t.Errorf("status = %d, want %d", got, c.status)
			}
		})
	}
}

// Which of `-L` and `-P` decides is a question only when both were given, so
// one of them alone must not put it to the dialect. A shell with no answer
// recorded still has to run `cd -P`.
func TestCdWithOnePathOptionAsksNothing(t *testing.T) {
	dir, real := cdTree(t)
	sem := PosixSemantics()
	sem.CdRefusesUnknownOption = Yes
	// Deliberately left unspecified: were it asked here, this would refuse.
	sem.CdLastPathOptionWins = Unspecified
	out, errs := &strings.Builder{}, &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
		Stdout: out, Stderr: errs,
	})
	runCd(t, r, "cd -P link/sub\npwd\n")
	if errs.Len() != 0 {
		t.Errorf("said %q, want nothing to be asked", errs.String())
	}
	if got := strings.TrimSpace(out.String()); !strings.HasPrefix(got, real) || strings.Contains(got, "link") {
		t.Errorf("cd -P left %q, want it resolved under %q", got, real)
	}
}

// A lone `-` is the previous directory and never an option, which the two
// above must not have taken away.
func TestCdDashIsStillThePreviousDirectory(t *testing.T) {
	dir, _ := cdTree(t)
	sem := PosixSemantics()
	sem.CdRefusesUnknownOption = Yes
	sem.CdLastPathOptionWins = Yes
	sem.CdDashPrintsTheDirectory = No
	out := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
		Stdout: out, Stderr: &strings.Builder{},
	})
	runCd(t, r, "cd real\ncd real/../sub\ncd -\npwd\n")
	if got := strings.TrimSpace(out.String()); !strings.HasSuffix(got, "real") {
		t.Errorf("cd - left %q, want it back in real", got)
	}
}

// A directory reached through a symlink, which is the only way to tell the
// two apart.
func cdTree(t *testing.T) (dir, real string) {
	t.Helper()
	dir = t.TempDir()
	// The temp directory itself may be reached through a link — /var is one
	// on this machine — so what `-P` should produce is measured rather than
	// assumed.
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "real", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	return dir, real
}

func runCd(t *testing.T, r *Runner, src string) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
}
