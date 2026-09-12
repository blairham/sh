// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package smoke

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/blairham/sh/internal/blocks"
	"github.com/blairham/sh/internal/boundary"
)

// Reading back what a session recorded, which is the other half of the rows
// that ask whether it recorded anything.
//
// The store is read with the store's own reader rather than by parsing
// `index.jsonl` here. A row that hand-rolled the format would keep passing
// after the format changed underneath it, which is the failure this suite is
// least able to afford: it would report a shell that records as one that does,
// for a reason unrelated to whether it does.

// blocksStoreDir is where a session's store lives, given its scratch home.
//
// Named explicitly in the environment rather than left to the store's fallback
// chain. The fallback would land under the scratch home anyway — `HOME` is the
// session's own directory — so this is not about keeping the real store safe.
// It is about what a red row would then mean: these rows ask whether a session
// *records*, and one that also depended on where an unnamed store goes by
// default would go red for a question it was not asking.
func blocksStoreDir(home string) string { return filepath.Join(home, "blocks") }

// How much of the store a row reads, and how often it looks again.
//
// The read is bounded because a row wants the line it just ran, not a history:
// a session that has run everything above it has a store of tens, and a bound
// well past that costs nothing and cannot be outgrown by a longer suite.
const (
	blocksRead     = 200
	blocksPollWait = 25 * time.Millisecond
)

// blockFor finds the record of one line, and its kept output.
//
// Polled rather than read once, and that is not defensive padding. A row
// synchronizes on a mark in the command's *output*, and the record is written
// when the line finishes — so the mark reaches the screen slightly before the
// record reaches the disk. A single read at that moment is a race that fails a
// working shell now and then, and an intermittently red row in a suite whose
// whole purpose is to be believed is worse than no row.
//
// Matched on the command text exactly as typed, which is what the store keeps:
// the line before expansion. That is also what makes the match unambiguous
// against the other lines this suite runs.
func (s *session) blockFor(ctx context.Context, line string) (blocks.Record, string, error) {
	store := blocks.Open(blocksStoreDir(s.home), boundary.Boundary{}, "smoke")
	deadline := time.Now().Add(budget)
	for {
		records := store.Load(ctx, blocksRead)
		for _, r := range records {
			if r.Command == line {
				body, _ := store.Body(ctx, r)
				return r, body, nil
			}
		}
		if time.Now().After(deadline) {
			return blocks.Record{}, "", fmt.Errorf(
				"the store holds no record of %s after %s; it has %d record(s)",
				quote(line), budget, len(records))
		}
		select {
		case <-ctx.Done():
			return blocks.Record{}, "", ctx.Err()
		case <-time.After(blocksPollWait):
		}
	}
}

// noBlockFor reports that nothing in the store holds this line.
//
// Read once rather than polled, and the asymmetry with blockFor above is the
// whole of what makes the answer worth anything. A poll cannot establish an
// absence: it would report "not yet" and "never" as the same thing, so the
// caller has to arrange that a record written *after* the one being asked
// about has already arrived — the index is append-only and written in order,
// so once a later line is there, an earlier one that is not was never written.
//
// Asking directly, without that arrangement, is a check that passes for the
// wrong reason: a session that recorded nothing at all — because it crashed,
// or because the store was never opened — answers exactly the way a session
// that correctly declined to record one line does.
func (s *session) noBlockFor(ctx context.Context, line string) error {
	store := blocks.Open(blocksStoreDir(s.home), boundary.Boundary{}, "smoke")
	for _, r := range store.Load(ctx, blocksRead) {
		if r.Command == line {
			return fmt.Errorf(
				"the store kept a record of %s, which the session was told to forget", quote(line))
		}
	}
	return nil
}
