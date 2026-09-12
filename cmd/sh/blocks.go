// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/blocks"
	"github.com/blairham/sh/internal/boundary"
)

// Reading back the block store, which is the other half of recording one.
//
// Here rather than as a builtin, and following the route -trace-events and
// -deny already established: the seam is exercised by the binary that exists
// to exercise seams. A builtin would have to live in interp, which has no
// history, no store and no business acquiring either — the substrate does not
// know that a prompt exists — and it is not a dialect's builtin either,
// because no shell in the panel has one and inventing a `blocks` command
// inside the bash dialect would make that dialect not-bash. When there is an
// interactive shell built on this substrate, the command belongs to that
// shell.

// blocksLimit is how far back naming a block looks.
//
// Bounded so that `-blocks-show 1` costs the same on a store somebody has been
// filling for a year as on one from this morning. It is larger than any
// listing anyone reads by eye and small enough to be a bounded read.
const blocksLimit = 10000

// blocksListDefault is how many a bare listing shows. A screenful and a bit,
// on the argument that a person asking what they just did means recently.
const blocksListDefault = 20

// showBlocks writes the recent blocks, one per line.
//
// The time first, because this is a log and that is how a log is scanned, and
// the status second, because a block that failed is the one somebody came
// looking for. The command is last, since it is the only field with no bound
// on its length.
func showBlocks(sh driver.Shell, w io.Writer, n int) error {
	store, err := openBlocks(sh)
	if err != nil {
		return err
	}
	for _, r := range store.Load(context.Background(), n) {
		kept := ""
		if r.Output != "" {
			kept = fmt.Sprintf(" [%d bytes%s]", r.OutputBytes, truncatedMark(r.Truncated))
		}
		if _, err := fmt.Fprintf(w, "%s  %3d  %6dms  %s  %s  %s%s\n",
			r.Start.UTC().Format(time.RFC3339), r.Status, r.DurationMs,
			r.ID, r.Cwd, r.Command, kept); err != nil {
			return err
		}
	}
	return nil
}

// showBlock writes one block: its record, then what it printed.
//
// The record as labeled lines rather than as the JSON it is stored as. The
// stored form is one `grep` away for anything that wants to parse it, and this
// route exists for a person.
func showBlock(sh driver.Shell, w io.Writer, name string) error {
	store, err := openBlocks(sh)
	if err != nil {
		return err
	}
	ctx := context.Background()
	r, err := store.Find(ctx, name, blocksLimit)
	if err != nil {
		return fmt.Errorf("%w: %s", err, name)
	}
	fields := [][2]string{
		{"id", r.ID},
		{"session", r.Session},
		{"command", r.Command},
		{"cwd", r.Cwd},
		{"start", r.Start.UTC().Format(time.RFC3339Nano)},
		{"duration", fmt.Sprintf("%dms", r.DurationMs)},
		{"status", fmt.Sprint(r.Status)},
		{"output", describeOutput(r)},
	}
	for _, f := range fields {
		if _, err := fmt.Fprintf(w, "%-9s %s\n", f[0]+":", f[1]); err != nil {
			return err
		}
	}
	body, ok := store.Body(ctx, r)
	if !ok {
		return nil
	}
	_, err = fmt.Fprintf(w, "\n%s", body)
	return err
}

// describeOutput says where a block's body is, or that there is not one.
//
// A block recorded without capture and a block whose body has since been
// removed are the same state on purpose, so this says what the record claims
// and lets the body's absence show up as an empty body.
func describeOutput(r blocks.Record) string {
	if r.Output == "" {
		return "none kept"
	}
	return fmt.Sprintf("%s (%d bytes%s)", r.Output, r.OutputBytes, truncatedMark(r.Truncated))
}

func truncatedMark(truncated bool) string {
	if truncated {
		return ", truncated"
	}
	return ""
}

// openBlocks is the store this invocation reads, through the same gate and
// sink the shell itself would use.
//
// The environment rather than a Runner's variables, because there is no
// session here — this route never builds one. The rules are the store's, so a
// person's `SH_BLOCKS_DIR` means the same thing to the tool that reads their
// blocks as it does to the shell that wrote them.
func openBlocks(sh driver.Shell) (*blocks.Store, error) {
	dir := blocks.DirFrom(os.LookupEnv)
	if dir == "" {
		// "or HOME" until #2274, when a home stopped being an invitation.
		// Saying so is the whole of the help: the reason a person sees this
		// is almost always that they never turned the store on.
		return nil, errors.New("no block store: SH_BLOCKS_DIR names one, and nothing else does")
	}
	// No session, on both halves. This route reads a store rather than being a
	// shell, so nothing here builds one and the front end never defaults one in
	// — the records it makes name no run because there is no run to name.
	return blocks.Open(dir,
		boundary.Boundary{Gate: sh.Gate, Events: sh.Events, Session: sh.Session},
		sh.Session), nil
}
