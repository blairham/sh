// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"bytes"
	"context"
	"io"
	"iter"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/blairham/sh/internal/boundary"
)

// IndexDir is the directory the index is sharded under, by date, exactly as
// the bodies are.
//
// The bodies were sharded from the first version and the index was not, and
// docs/design/blocks.md put trimming under "what is deliberately not here" on
// the strength of it: nothing in the shell deletes a record, so `rm -rf
// body/2026/08` and `find -mtime` are a person's answer. That argument was
// only ever true of the half it was written about. A single flat `index.jsonl`
// cannot be cut with `rm`, so the one file that grew without bound was the one
// file the documented escape hatch did not work on (#2275).
//
// Giving the index the same sharding makes the document's own answer true, and
// makes it true of the whole store in one sweep: `rm -rf index/2026/08
// body/2026/08` drops a month of both halves, and a record whose body is gone
// already reads as a block with no output kept.
//
// It is also what makes a bounded read possible. A reader that wants the last
// twenty records can open the newest shard and stop; on a flat file the only
// way to the end is through the whole of it.
const IndexDir = "index"

// indexShardLayout is the date path a shard is named by, under IndexDir.
//
// The same layout as a body's directory and for the same two reasons: it is
// the sharding somebody can act on with `rm`, and it is fixed-width, so a
// lexical sort of the names is chronological. Sorting is how a reader walks
// backwards, so the second property is load-bearing rather than tidy.
const indexShardLayout = "2006/01/02"

// IndexPath is the shard a record belongs in, relative to the store.
//
// By the time the block *started* rather than the time the record is written,
// so a record and its body always land under the same date. A block that began
// at 23:59 and ran for two minutes is filed on the day a person would look for
// it, and one sweep takes both halves of it.
func IndexPath(t time.Time) string {
	return filepath.ToSlash(filepath.Join(IndexDir, t.UTC().Format(indexShardLayout)+".jsonl"))
}

// tailChunk is how much of a shard is read at a time, walking backwards.
//
// Large enough that the common ask — a screenful of recent blocks — is one
// read, and small enough that the cost of naming a block does not scale with
// the store. A record is a couple of hundred bytes, so 64 KiB is a few hundred
// of them.
const tailChunk = 64 * 1024

// maxRecordLine is the longest line that can still be a record.
//
// The same bound the forward reader used on its scanner, kept because the
// reason is unchanged: a record holds a command line and a pasted command can
// be very long. It matters more here — a backwards reader accumulates the tail
// of a line until it finds where the line starts, so without a bound a file
// with no newline in it would be read into memory whole, which is the defect
// this file exists to fix.
const maxRecordLine = 4 * 1024 * 1024

// shards lists the store's index files, newest first, ending with the flat
// index older stores wrote.
//
// Lazily, and that is the point: a reader after the last twenty records
// descends into this year, this month, and today, and never lists the rest.
// An eager walk would read every month directory of every year to answer a
// question the newest shard already answers.
//
// The legacy `index.jsonl` comes last because it is oldest: it holds
// everything written before the store was sharded and nothing after, since
// this version never appends to it. A store that has been read but not written
// since the change still reads in full and in order.
func (s *Store) shards(ctx context.Context) iter.Seq[string] {
	return func(yield func(string) bool) {
		root := filepath.Join(s.dir, IndexDir)
		for _, year := range s.descending(ctx, root, isYear, true) {
			months := filepath.Join(root, year)
			for _, month := range s.descending(ctx, months, isTwoDigit, true) {
				days := filepath.Join(months, month)
				for _, day := range s.descending(ctx, days, isShardFile, false) {
					if !yield(path.Join(IndexDir, year, month, day)) {
						return
					}
				}
			}
		}
		yield(IndexName)
	}
}

// descending lists the entries of one directory that look like part of the
// layout, newest first.
//
// The shape test is not decoration. A store is a directory a person is invited
// to take `rm` to, so whatever else ends up in it — an editor's backup, a
// `.DS_Store`, a directory somebody made by hand — must not be descended into
// or parsed. And the ordering guarantee only holds for the fixed-width numeric
// names, so a name that is not one of those has no place in the sequence at
// all.
//
// A directory that is not there is not an error: it is a store nothing has
// written to yet, which is the first session anybody runs.
func (s *Store) descending(ctx context.Context, dir string, ok func(string) bool, want bool) []string {
	entries, err := s.bound.ReadDir(ctx, dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() == want && ok(e.Name()) {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	slices.Reverse(names)
	return names
}

// isYear reports whether a name is a four-digit year.
func isYear(name string) bool { return len(name) == 4 && digits(name) }

// isTwoDigit reports whether a name is a two-digit month or day.
func isTwoDigit(name string) bool { return len(name) == 2 && digits(name) }

// isShardFile reports whether a name is a day's shard.
func isShardFile(name string) bool {
	return isTwoDigit(strings.TrimSuffix(name, ".jsonl")) && strings.HasSuffix(name, ".jsonl")
}

// digits reports whether every byte is a decimal digit.
func digits(s string) bool {
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

// tail reads the last records of one shard, newest first, appending to out
// until it holds n of them.
//
// A file that is not there is the ordinary case rather than an error: every
// read walks back through dates the store may simply not have, and the legacy
// flat index is absent on every store written since the sharding.
func (s *Store) tail(ctx context.Context, rel string, n int, out []Record) []Record {
	full := filepath.Join(s.dir, filepath.FromSlash(rel))
	f, err := s.bound.OpenFile(ctx, boundary.File{Path: full})
	if err != nil {
		// A shard that is not there, or one a policy hides. Complaining about
		// either would be the first thing somebody saw.
		return out
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return out
	}
	return tailRecords(f, info.Size(), n, out)
}

// tailRecords walks a shard backwards, decoding records newest first.
//
// Taking a ReaderAt and a size rather than the open file is what lets a test
// count the bytes this reads, which is the whole claim: `Load` used to decode
// every record in the store and *then* take the last n, so a bounded result
// came out of an unbounded read — 1.1 GB of resident memory to print one line
// on a store of 1.3M records, against a comment in cmd/sh promising that
// naming a block costs the same after a year as on the first morning (#2275).
//
// The walk reads fixed chunks from the end. Within a chunk the lines after the
// first newline are complete; the bytes before it are the tail of a line that
// began earlier, and they are carried into the next read so the line is
// rejoined rather than lost. A line longer than any record we would accept is
// dropped instead of being accumulated, which is what keeps this bounded in
// memory as well as in time — and costs nothing real, because the head of it
// then fails to parse and is skipped by the rule a torn write already relies
// on.
func tailRecords(r io.ReaderAt, size int64, n int, out []Record) []Record {
	pos := size
	var pending []byte
	for pos > 0 && len(out) < n {
		size := min(int64(tailChunk), pos)
		pos -= size
		buf := make([]byte, size, size+int64(len(pending)))
		if _, err := r.ReadAt(buf, pos); err != nil {
			return out
		}
		region := append(buf, pending...)

		var complete []byte
		switch i := bytes.IndexByte(region, '\n'); {
		case pos == 0:
			// The front of the file, so nothing begins earlier: the leading
			// fragment is a whole line.
			complete, pending = region, nil
		case i >= 0:
			complete, pending = region[i+1:], region[:i]
		default:
			// No line boundary in this chunk at all.
			complete, pending = nil, region
		}
		if len(pending) > maxRecordLine {
			pending = nil
		}
		out = decodeBackwards(complete, n, out)
	}
	return out
}

// decodeBackwards decodes the complete lines of a chunk, last first.
func decodeBackwards(complete []byte, n int, out []Record) []Record {
	for len(complete) > 0 && len(out) < n {
		line := complete
		if i := bytes.LastIndexByte(complete, '\n'); i >= 0 {
			line, complete = complete[i+1:], complete[:i]
		} else {
			complete = nil
		}
		if r, ok := decode(strings.TrimSpace(string(line))); ok {
			out = append(out, r)
		}
	}
	return out
}
