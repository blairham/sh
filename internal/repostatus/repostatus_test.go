// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repostatus_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blairham/sh/internal/repostatus"
)

// A repository is built out of the files that describe one rather than by
// running anything. The formats are documented and the point of this package
// is that it never starts a process, so a fixture that started one would be
// measuring something else.

func repo(t *testing.T, head string) string {
	t.Helper()
	root := t.TempDir()
	git := filepath.Join(root, ".git")
	if err := os.MkdirAll(git, 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(git, "HEAD"), head)
	return root
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// cache is a cache closed on the way out, so that nothing a test started is
// still running when it ends.
func cache(t *testing.T, publish func()) *repostatus.Cache {
	t.Helper()
	c := repostatus.New(publish)
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return c
}

func TestADirectoryOutsideARepositoryDeclines(t *testing.T) {
	t.Parallel()
	// Most directories are not in one, and a prompt should cost nothing
	// there. The walk stops at the filesystem root rather than looping.
	if _, ok := cache(t, nil).Status(t.TempDir()); ok {
		t.Error("a directory with no repository above it answered as one")
	}
	if _, ok := cache(t, nil).Status(""); ok {
		t.Error("no directory at all answered as a repository")
	}
}

func TestTheBranchIsReadFromTheHeadAndFoundFromBelow(t *testing.T) {
	t.Parallel()
	root := repo(t, "ref: refs/heads/work\n")
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	c := cache(t, nil)
	for _, dir := range []string{root, deep} {
		status, ok := c.Status(dir)
		if !ok || status.Branch != "work" {
			t.Errorf("%s answered %q (found %v)", dir, status.Branch, ok)
		}
		if status.Root != root {
			t.Errorf("%s answered root %q, want %q", dir, status.Root, root)
		}
	}
}

func TestADetachedHeadIsTheCommitAndSaysSo(t *testing.T) {
	t.Parallel()
	root := repo(t, "1234567890abcdef1234567890abcdef12345678\n")
	status, ok := cache(t, nil).Status(root)
	if !ok {
		t.Fatal("a repository with a detached head was not found")
	}
	if status.Branch != "" || status.Commit != "1234567" || !status.Detached() {
		t.Errorf("a detached head answered branch %q commit %q", status.Branch, status.Commit)
	}
}

func TestAnOperationInProgressIsNamed(t *testing.T) {
	t.Parallel()
	// The state a person most needs a prompt to tell them about, because it
	// is the one they can forget they are in.
	for _, row := range []struct{ marker, want string }{
		{"MERGE_HEAD", "merge"},
		{"CHERRY_PICK_HEAD", "cherry-pick"},
		{"REVERT_HEAD", "revert"},
		{"BISECT_LOG", "bisect"},
	} {
		root := repo(t, "ref: refs/heads/work\n")
		write(t, filepath.Join(root, ".git", row.marker), "")
		status, _ := cache(t, nil).Status(root)
		if status.Operation != row.want {
			t.Errorf("%s answered operation %q, want %q", row.marker, status.Operation, row.want)
		}
	}
}

func TestARebaseKeepsTheBranchItIsPuttingBack(t *testing.T) {
	t.Parallel()
	// A rebase detaches the head. A prompt that said "detached" through one
	// would be telling a person they had lost their place.
	root := repo(t, "1234567890abcdef1234567890abcdef12345678\n")
	write(t, filepath.Join(root, ".git", "rebase-merge", "head-name"), "refs/heads/work\n")
	status, _ := cache(t, nil).Status(root)
	if status.Branch != "work" || status.Operation != "rebase" {
		t.Errorf("a rebase answered branch %q operation %q", status.Branch, status.Operation)
	}
}

func TestAGitFileNamesTheRepositoryElsewhere(t *testing.T) {
	t.Parallel()
	// A linked working tree and a submodule both say where their repository
	// is with a one-line file rather than a directory, and a reader that only
	// tested for a directory would report neither.
	root := t.TempDir()
	elsewhere := filepath.Join(root, "store", "worktrees", "one")
	write(t, filepath.Join(elsewhere, "HEAD"), "ref: refs/heads/linked\n")
	write(t, filepath.Join(root, "tree", ".git"), "gitdir: ../store/worktrees/one\n")

	status, ok := cache(t, nil).Status(filepath.Join(root, "tree"))
	if !ok || status.Branch != "linked" {
		t.Errorf("a linked working tree answered %q (found %v)", status.Branch, ok)
	}
}

func TestAChangedHeadIsSeenAtTheNextLookupWithNoWatchInvolved(t *testing.T) {
	t.Parallel()
	// The honest degradation. Where a watch cannot be had the answer is one
	// prompt late rather than wrong, and this is what makes that true: every
	// lookup re-stats the two files that would change the answer.
	root := repo(t, "ref: refs/heads/one\n")
	c := cache(t, nil)
	if status, _ := c.Status(root); status.Branch != "one" {
		t.Fatalf("the first answer was %q", status.Branch)
	}

	head := filepath.Join(root, ".git", "HEAD")
	write(t, head, "ref: refs/heads/twotwo\n")
	// Explicitly, because two writes inside one clock tick are exactly the
	// case a coarse timestamp cannot see and a test that raced it would be
	// flaky rather than wrong.
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(head, future, future); err != nil {
		t.Fatal(err)
	}
	if status, _ := c.Status(root); status.Branch != "twotwo" {
		t.Errorf("after the head moved the answer was still %q", status.Branch)
	}
}

func TestAWatchedRepositoryPublishesWhenTheBranchMoves(t *testing.T) {
	t.Parallel()
	// The capability's whole point: a branch moved in another terminal
	// redraws the prompt somebody is sitting in front of, with nothing
	// polling and nobody typing.
	root := repo(t, "ref: refs/heads/one\n")
	published := make(chan struct{}, 8)
	c := cache(t, func() {
		select {
		case published <- struct{}{}:
		default:
		}
	})
	if status, _ := c.Status(root); status.Branch != "one" {
		t.Fatalf("the first answer was %q", status.Branch)
	}
	if !c.Watching() {
		t.Skipf("no filesystem watch here: %s", c.Trouble())
	}

	// Renamed into place, which is how a branch actually moves.
	tmp := filepath.Join(root, ".git", "HEAD.lock")
	write(t, tmp, "ref: refs/heads/two\n")
	if err := os.Rename(tmp, filepath.Join(root, ".git", "HEAD")); err != nil {
		t.Fatal(err)
	}

	select {
	case <-published:
	case <-time.After(10 * time.Second):
		t.Fatal("the branch moved and nothing was published")
	}
	if status, _ := c.Status(root); status.Branch != "two" {
		t.Errorf("after the publish the answer was %q", status.Branch)
	}
}

func TestNothingIsPublishedForAChangeThatDrawsTheSame(t *testing.T) {
	t.Parallel()
	// A repository writes files constantly — an index, a log, a lock — and
	// almost none of it changes what a prompt draws. A capability that
	// published on every one of them would redraw a prompt for nothing.
	root := repo(t, "ref: refs/heads/one\n")
	published := make(chan struct{}, 8)
	c := cache(t, func() {
		select {
		case published <- struct{}{}:
		default:
		}
	})
	if _, ok := c.Status(root); !ok {
		t.Fatal("the repository was not found")
	}
	if !c.Watching() {
		t.Skipf("no filesystem watch here: %s", c.Trouble())
	}

	write(t, filepath.Join(root, ".git", "index"), "not really an index")
	select {
	case <-published:
		t.Error("a write that changes nothing a prompt draws published anyway")
	case <-time.After(time.Second):
	}
}

func TestAClosedCacheStopsWatchingAndStillAnswers(t *testing.T) {
	t.Parallel()
	root := repo(t, "ref: refs/heads/one\n")
	published := make(chan struct{}, 8)
	c := repostatus.New(func() {
		select {
		case published <- struct{}{}:
		default:
		}
	})
	if _, ok := c.Status(root); !ok {
		t.Fatal("the repository was not found")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if c.Watching() {
		t.Error("a closed cache is still watching")
	}
	// And still answers. A closed cache is a cache with no watch, which is
	// the same position a system with no watch at all is in: the two stats
	// every lookup takes are what keep the answer right, one prompt behind.
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/after\n")
	if status, ok := c.Status(root); !ok || status.Branch != "after" {
		t.Errorf("a closed cache answered %q (found %v)", status.Branch, ok)
	}
	select {
	case <-published:
		t.Error("a closed cache published")
	case <-time.After(300 * time.Millisecond):
	}
}

// BenchmarkStatusFromTheCache is the latency budget, in the place the spec
// says a budget belongs: a benchmark rather than a test, so that a number
// nobody can assert is still one command away.
//
// What it measures is the ordinary prompt — a repository already in the
// cache, nothing changed — which is the upward walk, two stats and a map
// lookup. The cold case is the same work plus one small read, and it happens
// once per repository per session.
func BenchmarkStatusFromTheCache(b *testing.B) {
	root := b.TempDir()
	git := filepath.Join(root, ".git")
	if err := os.MkdirAll(git, 0o700); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(git, "HEAD"), []byte("ref: refs/heads/work\n"), 0o600); err != nil {
		b.Fatal(err)
	}
	c := repostatus.New(nil)
	b.Cleanup(func() { _ = c.Close() })
	if _, ok := c.Status(root); !ok {
		b.Fatal("the repository was not found")
	}
	b.ResetTimer()
	for b.Loop() {
		if _, ok := c.Status(root); !ok {
			b.Fatal("the repository stopped being found")
		}
	}
}
