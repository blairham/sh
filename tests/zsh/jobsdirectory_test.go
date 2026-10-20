// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// `jobs -d` is this shell's alone, and it writes the directory each job was
// started in on a line under that job's row.
//
// The letter used to ride Diagnostics.UnimplementedOptionLetters, so the
// listing answered `jobs: -d is not implemented yet` — which is what zsh's
// own `W02jobs.ztst` stops on, six chunks in (#4507).
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — with `-f`, from a script file:
//
//	cd /tmp/x; sleep 5 &; cd /tmp/y; jobs -d
//	   [1]  + running    sleep 5
//	   (pwd : /tmp/x)
//
// The row is the ordinary state row and the `cd` does not move the line,
// which is the whole discriminating case: the directory is the **job**'s.
//
// The panel has no second reading of the letter for an axis to switch
// between. Measured the same day, `sleep 3 & jobs -d` is refused by every
// other column — `jobs: -d: invalid option` in bash 5.3.15 and in 3.2.57,
// `jobs: -d: unknown option` in ksh93u+, `jobs: Illegal option -d` in dash,
// and `jobs: illegal option -d` in BusyBox 1.37.0 inside the pinned alpine
// image — so Semantics.JobsOptions carrying the letter here, and nowhere
// else, is the whole of the dialect's answer.

// jobsDirTree makes three directories and hands back the one the shell starts
// in together with the other two.
func jobsDirTree(t *testing.T) (start, second, third string) {
	t.Helper()
	base := t.TempDir()
	// A temporary directory on a Mac is reached through a symlink, and the
	// shell prints the path it was handed rather than the link's target.
	if real, err := filepath.EvalSymlinks(base); err == nil {
		base = real
	}
	a, b, c := filepath.Join(base, "a"), filepath.Join(base, "b"), filepath.Join(base, "c")
	for _, d := range []string{a, b, c} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatalf("making %s: %v", d, err)
		}
	}
	return a, b, c
}

// longRow matches a row with the process id in it, which is where `-l` and
// this shell's `-p` put one: between the marker and the state word.
var longRow = regexp.MustCompile(`^\[[0-9]+\]  . [0-9]+ `)

// pwdLines is the `(pwd : …)` lines of a listing, in order, unwrapped.
func pwdLines(out string) []string {
	var dirs []string
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "(pwd : "); ok {
			dirs = append(dirs, strings.TrimSuffix(rest, ")"))
		}
	}
	return dirs
}

// The job held fixed while the shell moves, which is the case a listing that
// read the shell's own directory gets wrong — and the only one it does.
func TestJobsDashDNamesWhereTheJobStarted(t *testing.T) {
	a, b, _ := jobsDirTree(t)
	out, st := runZsh(t, a, `/bin/sleep 0.3 &
cd `+b+`
jobs -d
wait`)
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	if got := pwdLines(out); len(got) != 1 || got[0] != a {
		t.Errorf("got %q, want the one starting directory %q —\n%s", got, a, out)
	}
	if strings.Contains(out, "not implemented") {
		t.Errorf("the letter is still refused:\n%s", out)
	}
	// The row above the line is the ordinary state row and is unchanged by
	// the letter — `-d` adds a line rather than replacing one.
	if !strings.Contains(out, "[1]  + running    /bin/sleep 0.3\n(pwd : ") {
		t.Errorf("got %q, want the state row with the directory under it", out)
	}
}

// The shell held fixed while the jobs move. A shell that kept one directory
// for the table rather than one per job passes the case above and fails this.
func TestJobsDashDNamesEachJobsOwnDirectory(t *testing.T) {
	a, b, c := jobsDirTree(t)
	out, st := runZsh(t, a, `/bin/sleep 0.3 &
cd `+b+`
/bin/sleep 0.3 &
cd `+c+`
jobs -d
wait`)
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	if got := pwdLines(out); len(got) != 2 || got[0] != a || got[1] != b {
		t.Errorf("got %q, want %q then %q —\n%s", got, a, b, out)
	}
	if strings.Contains(out, c) {
		t.Errorf("a job was named by the directory the listing was asked from:\n%s", out)
	}
}

// The home directory is written `~`, as a prompt writes one, and the
// shortening is decided when the line is printed: measured on 5.9.2, a job
// started under `$HOME` and listed after `HOME` moved elsewhere is named by
// its full path.
func TestJobsDashDWritesTheHomeDirectoryAsATilde(t *testing.T) {
	a, b, _ := jobsDirTree(t)
	out, st := runZsh(t, a, "HOME="+a+"\n/bin/sleep 0.3 &\njobs -d\nwait")
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	if got := pwdLines(out); len(got) != 1 || got[0] != "~" {
		t.Errorf("got %q, want `~` —\n%s", got, out)
	}

	out, st = runZsh(t, a, "HOME="+a+"\n/bin/sleep 0.3 &\nHOME="+b+"\njobs -d\nwait")
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	if got := pwdLines(out); len(got) != 1 || got[0] != a {
		t.Errorf("got %q, want the full path %q once HOME has moved —\n%s", got, a, out)
	}
}

// `-d` composes with the letters this shell already had rather than standing
// in for any of them. `-l` still writes the long row, `-p` is still this
// shell's reading of the letter — the job's process group inside an ordinary
// listing — and the state filters keep or drop a job's row and its directory
// line as a pair.
func TestJobsDashDComposesWithTheOtherLetters(t *testing.T) {
	a, _, _ := jobsDirTree(t)
	for _, c := range []struct {
		name, letters string
		wantID        bool
		wantRow       bool
	}{
		{"the state row", "-d", false, true},
		{"the long row, d last", "-ld", true, true},
		{"the long row, l last", "-dl", true, true},
		{"this shell's -p is a listing too", "-pd", true, true},
		{"the running filter keeps it", "-rd", false, true},
		{"the stopped filter drops the pair", "-sd", false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, a, "/bin/sleep 0.3 &\njobs "+c.letters+"\nwait")
			if st != 0 {
				t.Fatalf("status %d: %s", st, out)
			}
			got := pwdLines(out)
			if !c.wantRow {
				if len(got) != 0 || strings.Contains(out, "[1]") {
					t.Errorf("got %q, want nothing at all for a running job under -s", out)
				}
				return
			}
			if len(got) != 1 || got[0] != a {
				t.Errorf("got %q, want the one directory %q —\n%s", got, a, out)
			}
			// The id sits between the marker and the state word, so the
			// two shapes are told apart there rather than by looking for a
			// digit anywhere in the row — the command carries one.
			row, _, _ := strings.Cut(out, "\n")
			if longRow.MatchString(row) != c.wantID {
				t.Errorf("row %q: want a process id after the marker: %v", row, c.wantID)
			}
		})
	}
}

// The letter came off Diagnostics.UnimplementedOptionLetters, and the two
// that are still missing stayed on it. A wholesale clearing of the entry
// would pass every case above and quietly accept `-z` and `-Z`, which this
// shell reads as instructions about the process title and this engine does
// not have.
func TestOnlyTheDirectoryLetterCameOffTheMissingList(t *testing.T) {
	if got := zsh.Diagnostics().UnimplementedOptionLetters["jobs"]; got != "zZ" {
		t.Errorf("UnimplementedOptionLetters[jobs] = %q, want the process-title pair alone", got)
	}
	if got := zsh.Semantics().JobsOptions; !strings.ContainsRune(got, 'd') {
		t.Errorf("JobsOptions = %q, want the `d` this shell has", got)
	}
	// The wording is the dialect's rather than the engine's fallback, which
	// is what makes the engine's fallback replaceable at all.
	if got := zsh.Diagnostics().JobDirectoryLine; got == "" {
		t.Error("JobDirectoryLine is empty; the one shell with the letter spells the line")
	} else if want := "(pwd : x)"; interp.Wording(got, "", "x") != want {
		t.Errorf("JobDirectoryLine renders %q, want %q", interp.Wording(got, "", "x"), want)
	}
	// And `-n` and `-x` are still bad options here rather than missing ones:
	// bash has both letters and this shell has neither.
	for _, letter := range "nx" {
		if strings.ContainsRune(zsh.Semantics().JobsOptions, letter) {
			t.Errorf("JobsOptions = %q, which claims bash's -%c", zsh.Semantics().JobsOptions, letter)
		}
	}
}
