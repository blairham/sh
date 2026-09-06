// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/blairham/sh/internal/blocks"
	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/event"
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

// blocksStore is the store this session records into, and a store that is
// turned off when it should record nothing.
//
// Built once per session. The session id is the front end's — the same string
// the Runner carries and the same one every event of this run is stamped with
// — rather than one made here, which is what lets a block and the audit
// records of the commands inside it be joined. Made here once, it agreed with
// nothing: a store and an audit stream from one session described the same
// commands under two identities.
//
// Empty is a session the front end gave no identity, which is honest rather
// than a reason to invent one; the records still read, they just cannot be
// joined to anything.
func (s Shell) blocksStore() *blocks.Store {
	return blocks.Open(s.blocksDir(),
		boundary.Boundary{Gate: s.Gate, Events: s.Events, Session: s.Session},
		s.Session)
}

// blocksDir resolves where the store lives, and empty means no store.
//
// Through the shell's own variables rather than the process environment, for
// the reason HISTFILE is: a session can set them at the prompt and mean it.
// The rules themselves are the store's — a tool that inspects a store asks the
// same function with the environment — so the two cannot answer differently
// about where a person's blocks are.
func (s Shell) blocksDir() string {
	if s.Runner == nil {
		return ""
	}
	return blocks.DirFrom(s.Runner.GetVar)
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

// captureOutput installs the capture this session keeps, or nothing.
//
// Installed once, at the start, rather than around each command, and the
// reason is background jobs rather than tidiness: what a child sees is decided
// by the writer the Runner holds when it is *started*, and a job started under
// capture goes on writing to that writer after the block it started in has
// closed. Swapping the writer between commands would make `sleep 10 &` lose
// its output to a sink nobody is reading. Installing once means a background
// job's output lands in whichever block is open when it arrives, which is what
// a terminal does with it too.
//
// The consequence is that setting SH_BLOCKS_OUTPUT at the prompt takes effect
// in the next session and not the next command.
func (s Shell) captureOutput() *blocks.Capture {
	if s.Runner == nil {
		return nil
	}
	on, max := blocks.CaptureFrom(s.Runner.GetVar)
	if !on {
		return nil
	}
	c := blocks.NewCapture(max)
	// The Runner's streams and not the session's. What the editor draws is not
	// a command's output, and a block that held the prompt would be recording
	// the shell talking to itself.
	s.Runner.Stdout = c.Stream(s.Runner.Stdout)
	s.Runner.Stderr = c.Stream(s.Runner.Stderr)
	return c
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
func (s Shell) closeBlock(ctx context.Context, store *blocks.Store, cap *blocks.Capture, b openBlock) {
	if strings.TrimSpace(b.command) == "" {
		// Taken anyway, so that whatever a background job printed while nobody
		// was typing joins the next real block instead of being attributed to
		// a newline.
		s.takeOutput(cap)
		return
	}
	end := s.now()
	// What the next prompt is told about the command that just ran. Here
	// rather than in the loops for the reason the record below is: this is the
	// one place that has both ends of the command and the rule for what counts
	// as one, and a second place applying that rule is a second place to
	// disagree about a blank line. It happens whether or not there is a store
	// to write to — a prompt provider is not a feature of the block index.
	s.counted().last = lastCommand{command: b.command, duration: end.Sub(b.start)}
	_ = store.Record(ctx, blocks.Record{
		ID:      event.NewID(b.start),
		Command: b.command,
		Cwd:     b.cwd,
		Start:   b.start,
		// Milliseconds rather than nanoseconds. The measurement is wall clock
		// around a whole typed line, which includes the time a person's
		// terminal took to draw the output; a unit that implies more precision
		// than the thing being measured has is a unit that will be believed.
		DurationMs: end.Sub(b.start).Milliseconds(),
		Status:     s.status(),
	}, s.takeOutput(cap))
}

// takeOutput is what this block printed, and nothing when the session is not
// capturing.
func (s Shell) takeOutput(cap *blocks.Capture) blocks.Output {
	if cap == nil {
		return blocks.Output{}
	}
	text, total, truncated := cap.Take()
	return blocks.Output{Text: text, Bytes: total, Truncated: truncated}
}
