// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/blocks"
	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/interp"
)

// The ranked jump, through the builtin a prompt registers.
//
// The command is exercised rather than its pieces, because the piece that has
// gone wrong in this repository before is the wiring: a ranking that is right
// and a `cd` that is never reached looks exactly like a shell with no feature
// in it. jumppty_test.go is the other end of the same argument — this drives
// the builtin, that drives a person's terminal.

// minutesAgo is a start time inside the ranking's decay window.
//
// It is here rather than the epoch-based `at` the rest of this package uses,
// and the difference is load-bearing: a record dated 1970 decays to zero, so
// a store full of them ranks entirely on the tie-break and comes back in
// *recency* order. Every test below passed that way before this existed —
// against a build with the score removed as well as against this one, which
// is a probe that cannot tell the two apart.
func minutesAgo(m int) time.Time { return time.Now().Add(-time.Duration(m) * time.Minute) }

// jumpShell is a Runner the builtin can run in, pointed at a store.
func jumpShell(t *testing.T, store, dir string) (*interp.Runner, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errs bytes.Buffer
	r := &interp.Runner{
		Dir:    dir,
		Env:    []string{blocks.DirVar + "=" + store},
		Stdout: &out,
		Stderr: &errs,
	}
	return r, &out, &errs
}

// jumpTree makes a directory under root, and returns the path.
func jumpTree(t *testing.T, root string, parts ...string) string {
	t.Helper()
	p := filepath.Join(append([]string{root}, parts...)...)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// jump runs the builtin the way a prompt would.
func jump(t *testing.T, r *interp.Runner, args ...string) int {
	t.Helper()
	return jumpBuiltin(boundary.Boundary{}, "SESSION")(r, t.Context(), args)
}

// A word names the highest-ranked directory holding it, and the shell is
// actually there afterwards — through `cd`, which is what keeps $OLDPWD and
// $PWD the shell's own answer rather than this command's.
func TestTheJumpGoesToTheBestRankedMatch(t *testing.T) {
	root := t.TempDir()
	work := jumpTree(t, root, "src", "shellwork")
	rare := jumpTree(t, root, "src", "shellold")
	store, _ := seedStore(t,
		blocks.Record{Command: "ls", Cwd: rare, Start: minutesAgo(1)},
		blocks.Record{Command: "ls", Cwd: work, Start: minutesAgo(40)},
		blocks.Record{Command: "ls", Cwd: work, Start: minutesAgo(30)},
	)
	r, _, errs := jumpShell(t, store, root)
	if code := jump(t, r, "shell"); code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs.String())
	}
	if r.Dir != work {
		t.Fatalf("landed in %q, want %q", r.Dir, work)
	}
	if old, _ := r.GetVar("OLDPWD"); old != root {
		t.Fatalf("OLDPWD is %q, want %q — the move did not go through `cd`", old, root)
	}
}

// A word is nearly always the name of the place meant rather than of
// something it is inside, so the directory whose last segment holds it wins
// even where a directory that merely contains the word is used more.
//
// Both candidates hold the word, and the one that does *not* win is the one
// used more: without the last-segment tier the ranking alone decides and this
// lands in the wrong place.
func TestTheJumpPrefersTheDirectoryTheWordNames(t *testing.T) {
	root := t.TempDir()
	named := jumpTree(t, root, "notes")
	inside := jumpTree(t, root, "notes", "deep")
	store, _ := seedStore(t,
		blocks.Record{Command: "ls", Cwd: named, Start: minutesAgo(50)},
		blocks.Record{Command: "ls", Cwd: inside, Start: minutesAgo(30)},
		blocks.Record{Command: "ls", Cwd: inside, Start: minutesAgo(20)},
		blocks.Record{Command: "ls", Cwd: inside, Start: minutesAgo(10)},
	)
	r, _, errs := jumpShell(t, store, root)
	if code := jump(t, r, "notes"); code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs.String())
	}
	if r.Dir != named {
		t.Fatalf("landed in %q, want %q", r.Dir, named)
	}
}

// Words match in the order they were written, so two of them narrow rather
// than widen.
//
// Both candidates hold both words and neither holds the last one in its final
// segment, so the tier above cannot decide this and the order is the only
// thing that can: the one that holds them the wrong way round is the one
// ranked higher.
func TestSeveralWordsMatchInOrder(t *testing.T) {
	root := t.TempDir()
	want := jumpTree(t, root, "red", "blue", "leaf")
	other := jumpTree(t, root, "blue", "red", "leaf")
	store, _ := seedStore(t,
		blocks.Record{Command: "ls", Cwd: other, Start: minutesAgo(40)},
		blocks.Record{Command: "ls", Cwd: other, Start: minutesAgo(30)},
		blocks.Record{Command: "ls", Cwd: want, Start: minutesAgo(10)},
	)
	r, _, errs := jumpShell(t, store, root)
	if code := jump(t, r, "red", "blue"); code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs.String())
	}
	if r.Dir != want {
		t.Fatalf("landed in %q, want %q", r.Dir, want)
	}
}

// A directory that has been deleted since it was recorded is skipped, and the
// next one that is still there answers. The walk stops at the first that
// exists rather than statting the whole ranking.
func TestADeletedDirectoryIsSkipped(t *testing.T) {
	root := t.TempDir()
	gone := jumpTree(t, root, "projgone")
	kept := jumpTree(t, root, "projkept")
	store, _ := seedStore(t,
		blocks.Record{Command: "ls", Cwd: kept, Start: minutesAgo(50)},
		blocks.Record{Command: "ls", Cwd: gone, Start: minutesAgo(20)},
		blocks.Record{Command: "ls", Cwd: gone, Start: minutesAgo(10)},
	)
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	r, _, errs := jumpShell(t, store, root)
	if code := jump(t, r, "proj"); code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs.String())
	}
	if r.Dir != kept {
		t.Fatalf("landed in %q, want %q", r.Dir, kept)
	}
}

// And a match that is entirely gone is said differently from a word nothing
// ever matched, because they are different things to have to fix.
func TestEveryMatchGoneIsItsOwnRefusal(t *testing.T) {
	root := t.TempDir()
	gone := jumpTree(t, root, "vanished")
	store, _ := seedStore(t, blocks.Record{Command: "ls", Cwd: gone, Start: minutesAgo(10)})
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	r, _, errs := jumpShell(t, store, root)
	if code := jump(t, r, "vanished"); code != 1 {
		t.Fatalf("status %d, want 1", code)
	}
	if !strings.Contains(errs.String(), "is gone") {
		t.Fatalf("stderr %q does not say the match is gone", errs.String())
	}
	errs.Reset()
	if code := jump(t, r, "neverseen"); code != 1 {
		t.Fatalf("status %d, want 1", code)
	}
	if !strings.Contains(errs.String(), "no ranked directory matches") {
		t.Fatalf("stderr %q does not say nothing matched", errs.String())
	}
}

// Standing in a directory is not a reason to be sent back to it: the match a
// person meant is the next one down.
func TestTheJumpDoesNotGoWhereItAlreadyIs(t *testing.T) {
	root := t.TempDir()
	here := jumpTree(t, root, "workhere")
	there := jumpTree(t, root, "workthere")
	store, _ := seedStore(t,
		blocks.Record{Command: "ls", Cwd: there, Start: minutesAgo(50)},
		blocks.Record{Command: "ls", Cwd: here, Start: minutesAgo(20)},
		blocks.Record{Command: "ls", Cwd: here, Start: minutesAgo(10)},
	)
	r, _, errs := jumpShell(t, store, here)
	if code := jump(t, r, "work"); code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs.String())
	}
	if r.Dir != there {
		t.Fatalf("landed in %q, want %q", r.Dir, there)
	}
}

// A session with no store is the default one since #2274, so the refusal is
// the ordinary answer rather than a corner: by name, with the one thing that
// fixes it, and never a silent `cd`.
func TestTheJumpRefusesWithNoStore(t *testing.T) {
	root := t.TempDir()
	r, _, errs := jumpShell(t, "", root)
	if code := jump(t, r, "anything"); code != 1 {
		t.Fatalf("status %d, want 1", code)
	}
	if !strings.Contains(errs.String(), blocks.DirVar) {
		t.Fatalf("stderr %q does not name the variable that turns the store on", errs.String())
	}
	if r.Dir != root {
		t.Fatalf("moved to %q with no store to rank", r.Dir)
	}
}

// An option this does not have, and a jump with nothing to match, are usage
// errors rather than guesses.
func TestTheJumpRefusesWhatItDoesNotDo(t *testing.T) {
	root := t.TempDir()
	store, _ := seedStore(t)
	for _, tc := range []struct {
		name, want string
		args       []string
	}{
		{"unknown option", "unknown option", []string{"-x", "word"}},
		{"nothing to match", "needs a word", nil},
	} {
		r, _, errs := jumpShell(t, store, root)
		if code := jump(t, r, tc.args...); code != 2 {
			t.Fatalf("%s: status %d, want 2", tc.name, code)
		}
		if !strings.Contains(errs.String(), tc.want) {
			t.Fatalf("%s: stderr %q does not say %q", tc.name, errs.String(), tc.want)
		}
		if !strings.Contains(errs.String(), jumpUsage) {
			t.Fatalf("%s: stderr %q has no usage line", tc.name, errs.String())
		}
	}
}

// The listing is what makes the ranking arguable: the score, the count that
// produced it, and the directory.
func TestTheListingShowsWhatTheRankingThinks(t *testing.T) {
	root := t.TempDir()
	one := jumpTree(t, root, "listone")
	two := jumpTree(t, root, "listtwo")
	store, _ := seedStore(t,
		blocks.Record{Command: "ls", Cwd: two, Start: minutesAgo(50)},
		blocks.Record{Command: "ls", Cwd: one, Start: minutesAgo(20)},
		blocks.Record{Command: "ls", Cwd: one, Start: minutesAgo(10)},
	)
	r, out, errs := jumpShell(t, store, root)
	if code := jump(t, r, "-l"); code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs.String())
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("listing %q, want two rows", out.String())
	}
	if !strings.HasSuffix(lines[0], one) || !strings.Contains(lines[0], "2") {
		t.Fatalf("first row %q, want %q with its two visits", lines[0], one)
	}
	if !strings.HasSuffix(lines[1], two) {
		t.Fatalf("second row %q, want %q", lines[1], two)
	}
	if r.Dir != root {
		t.Fatalf("a listing moved the shell to %q", r.Dir)
	}
}

// The wiring: the builtin is installed on a line an interactive session
// starts, and it is not there for a script. The registration is what has gone
// missing in this repository before — a feature that is written and never
// reached.
func TestTheJumpIsInstalledAtAPromptAndNotInAScript(t *testing.T) {
	sh, err := pickDialect("bash")
	if err != nil {
		t.Fatal(err)
	}
	sh = withDirJump(sh)
	r := &interp.Runner{}
	if _, ok := r.Builtin(jumpCommand); ok {
		t.Fatalf("%s is a builtin before a line has started", jumpCommand)
	}
	if sh.StartLine == nil {
		t.Fatal("no StartLine, so nothing ever registers the jump")
	}
	sh.StartLine(r)
	if _, ok := r.Builtin(jumpCommand); !ok {
		t.Fatalf("%s is not a builtin after a line started", jumpCommand)
	}
}

// And whatever the dialect already cleared at the start of a line still is.
func TestTheJumpKeepsTheDialectsOwnStartLine(t *testing.T) {
	called := 0
	sh := withDirJump(driver.Shell{StartLine: func(*interp.Runner) { called++ }})
	sh.StartLine(&interp.Runner{})
	if called != 1 {
		t.Fatalf("the dialect's StartLine ran %d times, want 1", called)
	}
}
