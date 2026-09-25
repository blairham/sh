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

// `jobs -d` names, under each job's row, the directory that **job** was
// started in.
//
// The noun is the job and not the shell, and the whole of this file is two
// ways of holding one of them fixed while the other moves. A listing that
// read the shell's directory at print time would agree with every row below
// where nothing moved — which is most listings anybody ever writes — so
// neither case is evidence on its own:
//
//   - the job held fixed and the shell moved: one job, a `cd` between
//     starting it and listing it. The line must not follow the `cd`.
//   - the shell held fixed and the job moved: two jobs started in two
//     directories and both listed from a third. Each names its own.
//
// The letter reaches this only where the dialect has it — see
// Semantics.JobsOptions — and the line's wording is Diagnostics'. What is
// asked here is the engine's question: which directory, and where the line
// goes.

// jobsDirTree is three directories under the runner's own, and a runner whose
// working directory starts at the first of them.
func jobsDirTree(t *testing.T) (setup func(*Runner), dirs func() (a, b, c string)) {
	t.Helper()
	base := t.TempDir()
	// Resolved, because a temporary directory on a Mac is reached through a
	// symlink and the shell's own answer is the path it was handed. A test
	// comparing the two spellings would fail on the link rather than on the
	// rule.
	if real, err := filepath.EvalSymlinks(base); err == nil {
		base = real
	}
	a, b, c := filepath.Join(base, "a"), filepath.Join(base, "b"), filepath.Join(base, "c")
	for _, d := range []string{a, b, c} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatalf("making %s: %v", d, err)
		}
	}
	return func(r *Runner) {
			sem := jobsSemantics("dlprs")
			r.Semantics = &sem
			r.Dir = a
		}, func() (string, string, string) {
			return a, b, c
		}
}

// jobsDirLines is the `(pwd : …)` lines of a listing, in the order they were
// written, with the wording taken off.
func jobsDirLines(t *testing.T, out string) []string {
	t.Helper()
	var dirs []string
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "(pwd : "); ok {
			dirs = append(dirs, strings.TrimSuffix(rest, ")"))
		}
	}
	return dirs
}

// The job held fixed while the shell moves. This is the discriminating half:
// it is the only one of the two that a listing reading the shell's own
// directory gets wrong.
func TestJobsDirectoryStaysWhereTheJobStarted(t *testing.T) {
	setup, dirs := jobsDirTree(t)
	a, b, _ := dirs()
	out, st := run(t, `sleep 0.3 &
cd `+b+`
jobs -d
wait`, setup)
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	got := jobsDirLines(t, out)
	if len(got) != 1 || got[0] != a {
		t.Errorf("the listing named %q, want the one directory %q the job started in —\n%s",
			got, a, out)
	}
	if strings.Contains(out, b) {
		t.Errorf("the listing followed the `cd` to %s:\n%s", b, out)
	}
}

// The shell held fixed while the jobs move. A shell that recorded one
// directory for the whole table — the first, the last, or its own — answers
// this one wrong and the case above right.
func TestTwoJobsNameTheirOwnDirectories(t *testing.T) {
	setup, dirs := jobsDirTree(t)
	a, b, c := dirs()
	out, st := run(t, `sleep 0.3 &
cd `+b+`
sleep 0.3 &
cd `+c+`
jobs -d
wait`, setup)
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	got := jobsDirLines(t, out)
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Errorf("the listing named %q, want %q then %q —\n%s", got, a, b, out)
	}
}

// The line goes *under* the row rather than into it, and `-d` composes with
// the other letters rather than replacing what they do: `-l` still writes the
// long row, and a state filter keeps or drops a job's row and its directory
// line together.
func TestJobsDirectoryLineComposesWithTheOtherLetters(t *testing.T) {
	setup, dirs := jobsDirTree(t)
	a, _, _ := dirs()

	t.Run("under the state row", func(t *testing.T) {
		out, st := run(t, "sleep 0.3 &\njobs -d\nwait", setup)
		if st != 0 {
			t.Fatalf("status %d: %s", st, out)
		}
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if len(lines) != 2 || !strings.HasPrefix(lines[0], "[1]") || lines[1] != "(pwd : "+a+")" {
			t.Errorf("got %q, want the job's row and then its directory", out)
		}
	})

	t.Run("under the long row", func(t *testing.T) {
		out, st := run(t, "sleep 0.3 &\njobs -ld\nwait", setup)
		if st != 0 {
			t.Fatalf("status %d: %s", st, out)
		}
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		// The long row carries the process id, which is what says `-l` was
		// not quietly dropped on the way to the new letter.
		if len(lines) != 2 || !strings.ContainsAny(lines[0], "0123456789") ||
			lines[1] != "(pwd : "+a+")" {
			t.Errorf("got %q, want the long row and then its directory", out)
		}
		if lines[0] == "[1]  "+strings.TrimPrefix(lines[0], "[1]  ") &&
			!strings.Contains(lines[0], "sleep") {
			t.Errorf("got %q, want the command still in the row", out)
		}
	})

	t.Run("a filtered-out job takes its directory with it", func(t *testing.T) {
		out, st := run(t, "sleep 0.3 &\njobs -sd\nwait", setup)
		if st != 0 {
			t.Fatalf("status %d: %s", st, out)
		}
		if strings.TrimSpace(out) != "" {
			t.Errorf("got %q, want nothing: a running job is not a stopped one", out)
		}
	})
}

// The wording is the dialect's and the verb is the directory. Asserted
// through a Diagnostics of this test's own, so that the rule above is not
// also an assertion about one shell's punctuation.
func TestJobsDirectoryLineIsWorded(t *testing.T) {
	setup, dirs := jobsDirTree(t)
	a, _, _ := dirs()
	out, st := run(t, "sleep 0.3 &\njobs -d\nwait", func(r *Runner) {
		setup(r)
		dg := Diagnostics{JobDirectoryLine: "started in <%[1]s>"}
		r.Diagnostics = &dg
	})
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	if !strings.Contains(out, "started in <"+a+">") {
		t.Errorf("got %q, want the dialect's own wording carrying %q", out, a)
	}
	if strings.Contains(out, "(pwd") {
		t.Errorf("got %q, want the fallback wording replaced", out)
	}
}

// The home directory is written `~`, and that is decided when the line is
// *printed* rather than when the job started: a job begun under one HOME and
// listed under another is named by its full path.
//
// The same reading a prompt's own directory code gives, and the same helper —
// a second one would drift from it.
func TestJobsDirectoryAbbreviatesTheHomeDirectoryAtPrintTime(t *testing.T) {
	setup, dirs := jobsDirTree(t)
	a, b, _ := dirs()

	t.Run("under the home directory", func(t *testing.T) {
		out, st := run(t, "HOME="+a+"\nsleep 0.3 &\njobs -d\nwait", setup)
		if st != 0 {
			t.Fatalf("status %d: %s", st, out)
		}
		if got := jobsDirLines(t, out); len(got) != 1 || got[0] != "~" {
			t.Errorf("got %q, want the home directory written `~` —\n%s", got, out)
		}
	})

	t.Run("HOME moved after the job started", func(t *testing.T) {
		out, st := run(t, "HOME="+a+"\nsleep 0.3 &\nHOME="+b+"\njobs -d\nwait", setup)
		if st != 0 {
			t.Fatalf("status %d: %s", st, out)
		}
		if got := jobsDirLines(t, out); len(got) != 1 || got[0] != a {
			t.Errorf("got %q, want the full path %q: the shortening is not recorded with the job —\n%s",
				got, a, out)
		}
	})
}

// A directory that has been taken away since the job started is still the
// directory the job started in. Nothing here goes back to the filesystem to
// ask, which is what makes that true — and is worth pinning, because a
// listing that resolved the path would refuse or print nothing instead.
func TestJobsDirectorySurvivesTheDirectoryItNames(t *testing.T) {
	setup, dirs := jobsDirTree(t)
	a, b, _ := dirs()
	out, st := run(t, `sleep 0.3 &
cd `+b+`
rmdir `+a+`
jobs -d
wait`, func(r *Runner) {
		setup(r)
		// `rmdir` is not a builtin here, so the script reaches for the one
		// on this machine; testPATH is already on the runner.
	})
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	if got := jobsDirLines(t, out); len(got) != 1 || got[0] != a {
		t.Errorf("got %q, want the gone directory %q named all the same —\n%s", got, a, out)
	}
}

// The letter is the dialect's: a listing in a shell whose JobsOptions does
// not carry `d` refuses it, and refuses it as an unknown letter rather than
// silently listing without the line.
func TestJobsDirectoryLetterIsTheDialectsToHave(t *testing.T) {
	// The status is read where the refusal happened: a `wait` after it is a
	// command of its own and leaves its own 0 behind.
	out, st := jobsRun(t, "sleep 0.3 & jobs -d; echo d=$?; wait", nil)
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	if !strings.Contains(out, "d=2") {
		t.Errorf("got %q, want the letter refused: `lprs` has no `d` in it", out)
	}
	if strings.Contains(out, "(pwd") {
		t.Errorf("got %q, want no directory line from a shell without the letter", out)
	}
}
