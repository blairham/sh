// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package fswatch says when a directory has changed, without asking.
//
// It exists for the prompt's repository status, and the shape of it is that
// issue's load-bearing choice written down: **watch, never poll.** A poller
// either burns a core or serves a stale answer, and a prompt that confidently
// shows the wrong branch is the worst failure this repository has.
//
// # Why this is written rather than imported
//
// There is no filesystem watch in the Go standard library, and the obvious
// package would be this module's **first runtime dependency** — see
// internal/depsurface, where the list is empty and the emptiness is an
// argument the rest of the tree rests on. `AGENTS.md`'s habit is the same one
// internal/eastasian and internal/unorm already followed: generate or write
// the small thing rather than import the large one.
//
// And it is small, because the question is small. Nothing here reports *what*
// changed, which is where a general watcher's complexity lives — a rename
// that has to be paired with its other half, a directory tree that has to be
// re-walked as it grows. The one caller asks "is what I computed still
// good", and the honest answer to any event at all is no.
//
// # What it does not do
//
// It does not recurse. A caller names the directories it cares about, which
// for a repository is a handful of them, and a tree that grows a directory
// after the watch was opened is a directory nobody is watching — so a caller
// that needs one re-adds it when something it *is* watching says the tree
// moved.
//
// It does not promise to work. Some filesystems have no watch worth the
// name, and some systems have none at all: Open answers an error and the
// caller degrades to something honest rather than to a silently stale
// answer. That is a requirement from #1314 and not a convenience here.
package fswatch

import "errors"

// ErrUnsupported is Open's answer where this system has no watch.
//
// A distinct error rather than a general one, because the caller's response
// to it is different in kind: not "try again" but "say so, and fall back to
// the weaker guarantee".
var ErrUnsupported = errors.New("no filesystem watch on this system")
