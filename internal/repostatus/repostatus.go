// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package repostatus answers what repository the shell is standing in,
// from a resident cache that a filesystem watch keeps honest.
//
// It is a **shell capability and not a theme**, which is #1314's whole
// argument: a prompt is only as fast as the slowest thing it asks about, and
// in practice that is repository status. Making it fast once makes every
// prompt fast — a hand-written parameter, a cross-shell prompt, and this
// tree's own theme engine alike — rather than making one theme fast.
//
// # What it answers, and what it deliberately does not
//
// The branch, or the short commit of a detached head, and whether an
// operation is half-finished. Those are a handful of stats and one small
// read, which is why they can be answered on the way to drawing a prompt.
//
// **Not the counts** — modified, untracked, ahead, behind. Those need a full
// index-versus-working-tree walk, which is the expensive half and the reason
// every fast prompt in existence either caches it or hands it to a separate
// process. docs/spec/prompt-theme.md says what a prompt with no scanner
// attached shows: the branch and no counts, and it never waits for either.
// This is that, and the scanner is the first real job for the plugin segment
// role the spec reserves.
//
// # Watched, not polled
//
// A poller either burns a core or serves a stale answer. So a repository is
// watched — internal/fswatch, written against the standard library because
// this module links nothing outside itself — and a change wakes a background
// refresh that publishes when the answer is different. That is what redraws
// a prompt somebody is sitting in front of when a branch moves in another
// terminal.
//
// Where a watch cannot be had, the answer degrades to something stated
// rather than to something silently stale: every lookup re-stats the two
// files that would change the answer, so a change is seen at the next prompt
// rather than immediately, and Trouble says so out loud. A prompt that
// confidently shows the wrong branch is the worst failure this repository
// has, and "one prompt late" is a different thing from "wrong".
package repostatus

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/blairham/sh/internal/fswatch"
)

// Status is what this package knows about the repository a directory is in.
type Status struct {
	// Root is the working tree's top, and Git is the directory holding the
	// repository's own files — the same path except in a linked working tree
	// or a submodule, where `.git` is a file naming somewhere else.
	Root string
	Git  string

	// Branch is the branch a commit would land on, empty when the head is
	// detached. Commit is the short form of what a detached head points at.
	Branch string
	Commit string

	// Operation is a half-finished thing the repository is in the middle of
	// — a merge, a rebase, a cherry-pick, a revert, a bisect — and empty
	// otherwise. It is the state a person most needs a prompt to tell them
	// about, because it is the one they can forget they are in.
	Operation string
}

// Detached reports whether the head points at a commit rather than a branch.
func (s Status) Detached() bool { return s.Branch == "" && s.Commit != "" }

// entry is one repository's cached answer, and the stat data that says
// whether it is still good.
type entry struct {
	status Status

	// gitMod and headMod are the two witnesses. A branch moves by a file
	// being renamed into place, which changes the directory's own time, and
	// the head is rewritten the same way — so these two cover every change
	// to what this package answers, at two stats rather than a walk.
	gitMod  time.Time
	headMod time.Time
	headSz  int64
}

// Cache is the resident per-repository cache.
//
// One per session. It holds an answer per repository root rather than per
// directory, because moving between two directories of one repository does
// not change the answer and should not cost a second read.
type Cache struct {
	publish func()

	mu      sync.Mutex
	repos   map[string]*entry
	watcher *fswatch.Watcher
	watched map[string]bool
	// trouble is why there is no watch, where there is none. Kept rather
	// than logged: the prompt reports what it could not do, once, where a
	// person configuring a prompt will read it.
	trouble string
	tried   bool
	closed  bool
}

// New returns a cache that calls publish when a repository it is watching
// has changed into something a prompt would draw differently.
//
// publish may be nil, which is a cache that answers from a lookup and never
// redraws anything — what a caller with no way to redraw should get.
//
// A nil *Cache is usable and answers "not in a repository" to everything. A
// prompt is drawn from whatever is wired into it, and a caller that was built
// without this one must get an answer rather than a panic on the most visible
// line on the screen.
func New(publish func()) *Cache {
	return &Cache{publish: publish, repos: map[string]*entry{}, watched: map[string]bool{}}
}

// Status answers for a directory, and never blocks on anything but the stats
// it takes itself.
//
// The second value is false where the directory is not in a repository,
// which is a segment declining rather than an error: most directories are
// not in one and a prompt should cost nothing there.
func (c *Cache) Status(dir string) (Status, bool) {
	if c == nil {
		// A caller that was built without one asks the same question and
		// gets the honest answer, rather than a panic from a prompt.
		return Status{}, false
	}
	root, git, ok := discover(dir)
	if !ok {
		return Status{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return read(root, git), true
	}
	c.arm(git)
	if held, ok := c.repos[root]; ok && fresh(held, git) {
		return held.status, true
	}
	return c.refresh(root, git).status, true
}

// Watching reports whether a filesystem watch is keeping the answers honest,
// which is the difference between a prompt that is redrawn when a branch
// moves and one that catches up at the next prompt.
func (c *Cache) Watching() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.watcher != nil
}

// Trouble is why there is no watch, or empty where there is one or where
// nothing has needed one yet.
func (c *Cache) Trouble() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.trouble
}

// Close stops the watch. Answers afterwards are read fresh rather than
// cached, because a cache nothing is invalidating is the silently-stale
// answer this package exists to avoid.
func (c *Cache) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	watcher := c.watcher
	c.watcher, c.closed = nil, true
	c.repos = map[string]*entry{}
	c.watched = map[string]bool{}
	c.mu.Unlock()
	if watcher == nil {
		return nil
	}
	return watcher.Close()
}

// arm opens the watch on first use and points it at one repository. Called
// with the lock held.
//
// On first use rather than at construction, so that a session that never
// stands in a repository — or never configures a prompt that asks — opens no
// descriptor and starts no goroutine.
func (c *Cache) arm(git string) {
	if c.watcher == nil && !c.tried {
		c.tried = true
		w, err := fswatch.Open(c.changed)
		if err != nil {
			c.trouble = "no filesystem watch here (" + err.Error() +
				"); the repository is re-read at each prompt instead"
			return
		}
		c.watcher = w
	}
	if c.watcher == nil || c.watched[git] {
		return
	}
	c.watched[git] = true
	if err := c.watcher.Add(git); err != nil {
		c.trouble = "cannot watch " + git + " (" + err.Error() +
			"); the repository is re-read at each prompt instead"
	}
}

// changed is the watcher's goroutine telling the cache that something moved.
//
// It re-reads every repository it holds and publishes only where the answer
// a prompt would draw is different. Nothing here reports *which* directory
// changed — the watch does not say, and a cache holding a handful of entries
// is cheaper to re-read than a message would be to route.
func (c *Cache) changed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	differs := false
	for root, held := range c.repos {
		before := held.status
		if c.refresh(root, held.status.Git).status != before {
			differs = true
		}
	}
	if differs && c.publish != nil {
		// A statement that a redraw would differ, not a request for one. The
		// session decides, and a prompt that renders the same is not written
		// to the screen at all — see repl/promptasync.go.
		c.publish()
	}
}

// refresh re-reads one repository and stores it. Called with the lock held.
func (c *Cache) refresh(root, git string) *entry {
	e := &entry{status: read(root, git)}
	if info, err := os.Stat(git); err == nil {
		e.gitMod = info.ModTime()
	}
	if info, err := os.Stat(filepath.Join(git, "HEAD")); err == nil {
		e.headMod, e.headSz = info.ModTime(), info.Size()
	}
	c.repos[root] = e
	return e
}

// fresh reports whether a held answer still matches what is on disk, at two
// stats.
//
// Both witnesses, because either alone has a hole. The head's own time misses
// a file system whose timestamps are coarse enough for two writes in one
// tick, which is the ordinary case for a branch switched twice in a second;
// the directory's misses nothing about the head but says nothing about a head
// rewritten in place. The size is there for the same reason a length is
// checked beside a time anywhere else: two names of the same length written
// in one tick are the case a time cannot see.
func fresh(held *entry, git string) bool {
	info, err := os.Stat(git)
	if err != nil || !info.ModTime().Equal(held.gitMod) {
		return false
	}
	head, err := os.Stat(filepath.Join(git, "HEAD"))
	if err != nil {
		return held.headMod.IsZero()
	}
	return head.ModTime().Equal(held.headMod) && head.Size() == held.headSz
}
