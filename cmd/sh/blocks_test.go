// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/blocks"
	"github.com/blairham/sh/internal/boundary"
)

// Reading a store back, through the flags a person types.
//
// Through run() rather than a reassembly of what main does, for the reason
// sandbox_test.go states: the failure worth guarding against is wiring that
// exists and is not reached, and a test that never enters main's own path
// cannot see it.
//
// The environment is redirected with t.Setenv, so nothing here can find the
// machine's own store — which would be a different answer on every machine and
// would be the user's own file besides.

// seedStore fills a store in a temporary directory and points the environment
// at it, returning where it is and the ids it wrote, oldest first.
func seedStore(t *testing.T, recs ...blocks.Record) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	// HOME as well, so a rule that stopped consulting SH_BLOCKS_DIR would fall
	// back into the temporary directory rather than into a real home. And a
	// HISTFILE that is set but not empty, because an empty one is the switch
	// that turns the whole store off.
	t.Setenv("HOME", dir)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HISTFILE", filepath.Join(dir, "hist"))
	t.Setenv(blocks.DirVar, dir)

	s := blocks.Open(dir, boundary.Boundary{}, "SESSION")
	t.Cleanup(func() { _ = s.Close() })
	var ids []string
	for _, r := range recs {
		if r.ID == "" {
			r.ID = blocks.NewID(r.Start)
		}
		ids = append(ids, r.ID)
		if err := s.Record(t.Context(), r, blocks.Output{}); err != nil {
			t.Fatal(err)
		}
	}
	return dir, ids
}

func at(sec int64) time.Time { return time.Unix(sec, 0).UTC() }

// A listing says what ran and how it went, most recent last.
func TestBlocksListShowsWhatRan(t *testing.T) {
	_, _ = seedStore(t,
		blocks.Record{Command: "make check", Cwd: "/src", Start: at(1000), DurationMs: 8421, Status: 2},
		blocks.Record{Command: "git status", Cwd: "/src", Start: at(2000), DurationMs: 41},
	)
	got := sandboxed(t, "", "-blocks-list")
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.errs)
	}
	lines := strings.Split(strings.TrimSuffix(got.out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("printed %d lines: %q", len(lines), got.out)
	}
	for i, want := range []string{"make check", "git status"} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d is %q, want %q in it", i, lines[i], want)
		}
	}
	// The status is on the line, because a block that failed is the one
	// somebody came looking for.
	if !strings.Contains(lines[0], "  2  ") {
		t.Errorf("the failing block's line is %q, want its status on it", lines[0])
	}
	if !strings.Contains(lines[0], "8421ms") {
		t.Errorf("the line is %q, want the duration on it", lines[0])
	}
}

// A count asks for fewer, and it is attached rather than a following word, so
// a script operand can never be eaten by it.
func TestBlocksListTakesACount(t *testing.T) {
	_, _ = seedStore(t,
		blocks.Record{Command: "first", Start: at(1000)},
		blocks.Record{Command: "second", Start: at(2000)},
		blocks.Record{Command: "third", Start: at(3000)},
	)
	got := sandboxed(t, "", "-blocks-list=1")
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.errs)
	}
	if n := strings.Count(got.out, "\n"); n != 1 {
		t.Fatalf("printed %d lines for a count of 1: %q", n, got.out)
	}
	if !strings.Contains(got.out, "third") {
		t.Errorf("printed %q, want the most recent", got.out)
	}
	if bad := sandboxed(t, "", "-blocks-list=nope"); bad.code == 0 {
		t.Error("a count that is not a number was accepted")
	}
}

// One block, named by how far back it is or by its id, with what it printed.
func TestBlocksShowNamesOneBlock(t *testing.T) {
	_, ids := seedStore(t,
		blocks.Record{Command: "older", Start: at(1000)},
		blocks.Record{Command: "newer", Start: at(2000)},
	)
	byRecency := sandboxed(t, "", "-blocks-show", "1")
	if byRecency.code != 0 {
		t.Fatalf("exit %d: %s", byRecency.code, byRecency.errs)
	}
	if !strings.Contains(byRecency.out, "command:  newer") {
		t.Errorf("1 gave %q, want the most recent", byRecency.out)
	}
	byID := sandboxed(t, "", "-blocks-show", ids[0])
	if !strings.Contains(byID.out, "command:  older") {
		t.Errorf("an id gave %q", byID.out)
	}
	// A block that is not there is a failure with a name in it rather than
	// silence, which is the difference between a typo and an empty store.
	missing := sandboxed(t, "", "-blocks-show", "99")
	if missing.code == 0 || !strings.Contains(missing.errs, "99") {
		t.Errorf("a missing block gave exit %d, %q", missing.code, missing.errs)
	}
}

// The body follows the record, so one command is a command and its output.
func TestBlocksShowPrintsTheBody(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("HISTFILE", "/dev/null")
	t.Setenv(blocks.DirVar, dir)
	s := blocks.Open(dir, boundary.Boundary{}, "SESSION")
	t.Cleanup(func() { _ = s.Close() })
	err := s.Record(t.Context(),
		blocks.Record{ID: blocks.NewID(at(1000)), Command: "build", Start: at(1000)},
		blocks.Output{Text: "compiling\nfailed\n", Bytes: 9000, Truncated: true})
	if err != nil {
		t.Fatal(err)
	}
	got := sandboxed(t, "", "-blocks-show", "1")
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.errs)
	}
	if !strings.Contains(got.out, "compiling\nfailed\n") {
		t.Errorf("printed %q, want the body after the record", got.out)
	}
	// The true size and the fact that it is not all of it, so nobody reads a
	// truncated body as the whole of one.
	if !strings.Contains(got.out, "9000 bytes, truncated") {
		t.Errorf("printed %q, want the true size and the truncation said", got.out)
	}
}

// A block recorded without capture says so rather than showing an empty body
// that might be a body.
func TestBlocksShowSaysWhenNoOutputWasKept(t *testing.T) {
	_, _ = seedStore(t, blocks.Record{Command: "quiet", Start: at(1000)})
	got := sandboxed(t, "", "-blocks-show", "1")
	if !strings.Contains(got.out, "output:   none kept") {
		t.Errorf("printed %q, want it saying no output was kept", got.out)
	}
}

// Reading a store is not running a shell, so it ends the invocation: a script
// named on the same line is not run.
func TestBlocksFlagsEndTheInvocation(t *testing.T) {
	_, _ = seedStore(t, blocks.Record{Command: "recorded", Start: at(1000)})
	got := sandboxed(t, "", "-blocks-list", "-c", "echo ran")
	if strings.Contains(got.out, "ran") {
		t.Errorf("the shell ran anyway: %q", got.out)
	}
	if !strings.Contains(got.out, "recorded") {
		t.Errorf("printed %q, want the listing", got.out)
	}
}

// A policy that hides the store hides it from the tool that reads it too. The
// gate the same line asked for is the gate this route goes through, so a
// refusal reads as an empty store rather than as a crash.
func TestADeniedStoreReadsAsEmpty(t *testing.T) {
	dir, _ := seedStore(t, blocks.Record{Command: "recorded", Start: at(1000)})
	if got := sandboxed(t, "", "-blocks-list"); !strings.Contains(got.out, "recorded") {
		t.Fatalf("the store did not list without a policy: %q", got.out)
	}
	got := sandboxed(t, "", "-deny", dir, "-blocks-list")
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.errs)
	}
	if got.out != "" {
		t.Errorf("a refused store listed %q", got.out)
	}
}

// With nowhere to look, the complaint names the variable rather than printing
// nothing and exiting zero.
func TestNoStoreIsAnError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv(blocks.DirVar, "")
	got := sandboxed(t, "", "-blocks-list")
	if got.code == 0 {
		t.Fatalf("exit 0 with no store: %q", got.out)
	}
	if !strings.Contains(got.errs, blocks.DirVar) {
		t.Errorf("complained %q, want the variable named", got.errs)
	}
}
