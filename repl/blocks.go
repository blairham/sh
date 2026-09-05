// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blairham/sh/internal/blocks"
	"github.com/blairham/sh/internal/boundary"
)

// A block is a command and its output as one unit, and this is where a session
// records one.
//
// The command half only, for now: the line as typed, where it ran, when, for
// how long, and what it exited with. Every field is something the Runner
// already knows, so this is plumbing rather than measurement — which is the
// argument for doing it before the output half, since the output half costs a
// child its terminal and this costs nothing.
//
// docs/design/blocks.md is the design. The two decisions a reader of this file
// has to know are here rather than only there:
//
//   - The plain-line history file is untouched. The index is a second file
//     beside it, never a replacement, because the two answer different
//     questions — the line file is the recall list the up arrow walks, and the
//     index is the record of what happened. Either is disposable without
//     harming the other.
//   - An empty HISTFILE turns this off as well. That is a coupling and it is
//     deliberate: `HISTFILE=` is what a person types when they mean *do not
//     remember this session*, and a shell that honored it for the line file
//     and went on writing a richer record would be doing the opposite of what
//     was asked in the one moment it matters most.

// blocksDirVar names the store, and an empty value turns it off — the same
// idiom an empty HISTFILE already is, so there is one thing to learn.
//
// SH_-prefixed rather than HIST-shaped because it is ours: no shell in the
// panel has a variable by this name, so it cannot collide with something an
// existing rc file sets and means differently.
const blocksDirVar = "SH_BLOCKS_DIR"

// blocksStore is the store this session records into, and a store that is
// turned off when it should record nothing.
//
// Built once per session. The id is made here rather than by the store so that
// a test can see the same value the records carry.
func (s Shell) blocksStore() *blocks.Store {
	return blocks.Open(s.blocksDir(), boundary.Boundary{Gate: s.Gate, Events: s.Events},
		blocks.NewID(s.now()))
}

// blocksDir resolves where the store lives, and empty means no store.
//
// Through the shell's own variables rather than the process environment, for
// the reason HISTFILE is: a session can set them at the prompt and mean it.
//
// A state directory rather than a dotfile in the home directory, and the
// reason is the output half: the index is small and the bodies are not, so
// this is a growing directory of arbitrary size, which is what a state
// directory is for.
func (s Shell) blocksDir() string {
	if s.Runner == nil {
		return ""
	}
	// The one off switch that governs both files. Asked first, because a
	// session that is not recording lines must not be recording blocks
	// whatever else it was told.
	if h, ok := s.Runner.GetVar("HISTFILE"); ok && h == "" {
		return ""
	}
	if dir, ok := s.Runner.GetVar(blocksDirVar); ok {
		return dir
	}
	if state, ok := s.Runner.GetVar("XDG_STATE_HOME"); ok && state != "" {
		return filepath.Join(state, "sh", "blocks")
	}
	home, _ := s.Runner.GetVar("HOME")
	if home == "" {
		// Nowhere to put it, which is the same answer the history file gives.
		return ""
	}
	return filepath.Join(home, ".local", "state", "sh", "blocks")
}

// openBlock is what a session starts recording a block with: the moment it
// began and where it began.
//
// Taken before the line runs rather than after, because both answers change
// while it does. `cd /tmp` was typed somewhere else, and a command's duration
// is the whole point of recording when it started.
type openBlock struct {
	command string
	cwd     string
	start   time.Time
}

// beginBlock notes what is about to run.
func (s Shell) beginBlock(command string) openBlock {
	return openBlock{command: command, cwd: s.blockCwd(), start: s.now()}
}

// blockCwd is where the Runner says it is.
//
// r.Dir rather than os.Getwd, and that is not a preference: interp never
// changes the process's directory — `cd` sets r.Dir — so the process's answer
// is where the shell was started and not where it is. Falling back to the
// process is right only for a Runner that was never given a directory, which
// is the case where the two agree.
func (s Shell) blockCwd() string {
	if s.Runner == nil {
		return ""
	}
	if s.Runner.Dir != "" {
		return s.Runner.Dir
	}
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return dir
}

// closeBlock records what came of it.
//
// A blank line is not a block. Every shell in the panel treats a bare newline
// at the prompt as nothing happening, and recording one would fill the store
// with records of a person pressing return.
//
// Failure to write is not reported to the session. There is a difference
// between the two files here and it is deliberate: the history file's write
// happens once, at exit, and a failure there loses the whole session, so it is
// worth a complaint. A block is written after every command, so a store that
// cannot be written would complain after every command — which is a broken
// prompt rather than a useful diagnostic. The store is an addition to a shell
// that works without it.
func (s Shell) closeBlock(ctx context.Context, store *blocks.Store, b openBlock) {
	if strings.TrimSpace(b.command) == "" {
		return
	}
	end := s.now()
	_ = store.Append(ctx, blocks.Record{
		ID:      blocks.NewID(b.start),
		Command: b.command,
		Cwd:     b.cwd,
		Start:   b.start,
		// Milliseconds rather than nanoseconds. The measurement is wall clock
		// around a whole typed line, which includes the time a person's
		// terminal took to draw the output; a unit that implies more precision
		// than the thing being measured has is a unit that will be believed.
		DurationMs: end.Sub(b.start).Milliseconds(),
		Status:     s.status(),
	})
}
