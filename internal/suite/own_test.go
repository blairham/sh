// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAFetchedColumnCannotNameAFile is the guard [Suite.attribute]'s comment
// names, and the reason the no-path rule is scoped rather than dropped.
//
// Another project's suite is its expression: CLEANROOM.md's red list covers
// it, and a path in a report is an invitation to go look. That reasoning is
// about *their* files. Ours are committed, Apache-2.0 and meant to be opened,
// and a report that would not say which of our own cases failed is not a work
// list. So the difference has to be built rather than remembered — which
// means something has to fail when somebody builds it wrong.
func TestAFetchedColumnCannotNameAFile(t *testing.T) {
	for _, s := range Panel {
		if got := s.attribute("array.tests"); got != "" {
			t.Errorf("the fetched %s column named a file: %q", s.Name, got)
		}
	}
	for _, s := range Ours {
		if got := s.attribute("status.tests"); got != "status.tests" {
			t.Errorf("our own %s column would not name its own case: %q", s.Name, got)
		}
	}
}

// TestOurColumnsClaimOnlyDirectoriesThatExist keeps the tiers honest.
//
// A column that claims a directory with no files in it runs nothing there and
// reports a column that ran and agreed. That is the same mistake as a silent
// skip one level down, and it is the one this whole instrument is arranged to
// make impossible — so ext/ and the per-dialect directories arrive with cases
// in them rather than as placeholders.
func TestOurColumnsClaimOnlyDirectoriesThatExist(t *testing.T) {
	root := filepath.Join("..", "..", OurRoot)
	for _, s := range Ours {
		if missing := s.Missing(root); len(missing) > 0 {
			t.Errorf("the %s column claims %v under %s and they are not there", s.Name, missing, OurRoot)
		}
		for _, dir := range s.Dirs {
			names, err := Files(filepath.Join(root, dir), OurExt)
			if err != nil {
				t.Errorf("%s: %v", s.Name, err)
				continue
			}
			if len(names) == 0 {
				t.Errorf("the %s column claims %s/ and there is nothing in it to run, "+
					"so the column would report that it ran and agreed", s.Name, dir)
			}
		}
	}
	for _, tier := range Tiers {
		if _, err := os.Stat(filepath.Join(root, tier)); err != nil {
			t.Errorf("the %s tier is cross-checked and is not there: %v", tier, err)
		}
	}
}

// TestShellIsResolvedWhereThePersonTypingItIs is the bug this guard was
// written for, and it is a wrong answer rather than an error.
//
// Every run gets a directory of its own to ruin and the file is run from
// inside it, so a *relative* path to the binary under test resolves against
// that directory and not against the caller's. `make suite` passes absolute
// paths and worked; the same command typed by hand with `-own-bin
// bash=build/own-bash` started nothing, and every column printed 0/10 strict
// as though four shells had disagreed.
func TestShellIsResolvedWhereThePersonTypingItIs(t *testing.T) {
	dir := t.TempDir()
	bin := fakeShell(t, dir, "ashell", "exit 0")
	rel, err := filepath.Rel(mustWd(t), bin)
	if err != nil {
		t.Skipf("no relative path from the working directory to %s", bin)
	}
	got, err := Shell(rel)
	if err != nil {
		t.Fatalf("Shell(%q): %v", rel, err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("Shell(%q) = %q, which a run in a directory of its own cannot open", rel, got)
	}
	if _, err := Shell(filepath.Join(dir, "nosuchshell")); err == nil {
		t.Error("a binary that is not there resolved; a missing binary is a fact about the " +
			"invocation and belongs in the invocation's diagnostic, not in a column of failures")
	}
	if _, err := Shell(dir); err == nil {
		t.Error("a directory resolved as a shell")
	}
}

func mustWd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}

// TestAShellThatNeverStartedIsNotScored is the second half of the same bug.
//
// Resolving the path stops one way of handing the harness a binary it cannot
// run. It does not stop every way — the file can be deleted between the
// resolve and the run, or fail to execute on the machine — so a run that
// never started must be unable to reach the score at all. It agreed with
// nothing and disagreed with nothing.
//
// The two sides are kept apart for the reason the hung pair is: our binary
// failing to start is one fault and the reference failing to start is
// another, and neither is a finding about a shell.
func TestAShellThatNeverStartedIsNotScored(t *testing.T) {
	tests := testDir(t, map[string]string{"f.tests": "echo a\n"})
	absent := filepath.Join(t.TempDir(), "nosuchshell")

	res := gradeFile(t, tests, "f.tests", absent, "/bin/sh")
	if !res.DialectFailed {
		t.Errorf("our binary never started and it was recorded as %+v", res)
	}
	if res.Scored || res.Strict {
		t.Error("a run that never happened was scored, so a harness fault would " +
			"be reported as a shell that disagreed")
	}

	res = gradeFile(t, tests, "f.tests", "/bin/sh", absent)
	if !res.OracleFailed {
		t.Errorf("the reference never started and it was recorded as %+v", res)
	}
	if res.Scored || res.Strict {
		t.Error("a run the reference never started was scored")
	}
}

// TestASweepKeepsAFailedStartOutOfTheNumbers is the same claim one level up,
// because the number a person reads is the report's and not the result's.
func TestASweepKeepsAFailedStartOutOfTheNumbers(t *testing.T) {
	tests := testDir(t, map[string]string{"a.tests": "echo a\n", "b.tests": "echo b\n"})
	s := Suite{Name: "fake", Dialect: "bash", Ours: true, Ext: ".tests", Dirs: []string{"tests"}}
	absent := filepath.Join(t.TempDir(), "nosuchshell")

	rep, err := Sweep(context.Background(), s, filepath.Dir(tests), absent, "/bin/sh",
		Options{Timeout: 5 * time.Second, Jobs: 2})
	if err != nil {
		t.Fatal(err)
	}
	if rep.DialectFailed != rep.Files || rep.Files == 0 {
		t.Fatalf("%d of %d files recorded a binary that never started", rep.DialectFailed, rep.Files)
	}
	if rep.Scored != 0 || rep.Strict != 0 {
		t.Errorf("a sweep with no binary scored %d and called %d strict", rep.Scored, rep.Strict)
	}
	if rep.StrictRate() != 0 || rep.LineRate() != 0 {
		t.Error("a sweep that measured nothing produced a rate")
	}
}

// TestEveryDialectHasANativeColumn is the guard #2336 asks for, one
// instrument along.
//
// [Ours] is a hardcoded literal, and a hardcoded literal of dialects is how
// `axissweep.Targets()` came to be missing dialect/ash with a comment
// promising a compile error that does not exist. A dialect that is absent
// from this list is graded by nothing at all and there is no sign of it in
// the report: the column is not "not yet", it simply is not a column, and
// this instrument's whole argument is that a missing column reads as a
// column that passed.
//
// So the list is checked against the directories under dialect/, which is
// where a new shell actually arrives.
func TestEveryDialectHasANativeColumn(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "..", "dialect"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s, ok := FindOurs(e.Name())
		if !ok {
			t.Errorf("dialect/%s has no native column, so `make suite` grades it with "+
				"nothing and says nothing about not having done so", e.Name())
			continue
		}
		if _, ok := s.Syntax(); !ok {
			t.Errorf("the %s column has no parser configuration, so its parsed figure "+
				"would be a statement about no dialect at all", s.Name)
		}
	}
}

// TestTheCrossCheckAsksOnlyTheShellsThatClaimTheTier keeps a tier's claim the
// size it actually is: core/ is every reference, and a tier the measured
// holdouts do not claim must not be failed by them.
func TestTheCrossCheckAsksOnlyTheShellsThatClaimTheTier(t *testing.T) {
	refs := []Reference{{Name: "bash"}, {Name: "zsh"}, {Name: "ksh93"}, {Name: "dash"}}
	if got := CrossShells("core", refs); len(got) != len(refs) {
		t.Errorf("core/ was cross-checked against %d of %d references", len(got), len(refs))
	}
	if got := CrossShells("nosuchtier", refs); len(got) != 0 {
		t.Errorf("a tier no column claims was cross-checked against %d references", len(got))
	}
}
