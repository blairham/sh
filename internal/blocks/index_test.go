// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/interp"
)

// day is a time on a given date, so a test can say which shard it means.
func day(year int, month time.Month, d, hour int) time.Time {
	return time.Date(year, month, d, hour, 0, 0, 0, time.UTC)
}

// A record is filed under the date its block started, in the same shape the
// bodies already used.
//
// This is the whole of the retention fix: `index.jsonl` was one flat file, so
// the `rm -rf body/2026/08` the design document offers as the answer to a
// store that has grown worked on the bodies and not on the index — the one
// file that actually grows without bound was the one file the documented
// escape hatch could not touch (#2275).
func TestARecordIsFiledUnderItsDate(t *testing.T) {
	s, dir := newStore(t)
	mustAppend(t, s, Record{ID: "A", Command: "august", Start: day(2026, time.August, 4, 9)})
	mustAppend(t, s, Record{ID: "B", Command: "september", Start: day(2026, time.September, 14, 9)})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"index/2026/08/04.jsonl", "index/2026/09/14.jsonl"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(want))); err != nil {
			t.Errorf("no shard at %s: %v", want, err)
		}
	}
	// And the flat file is not written at all, so a store made by this version
	// has nothing in it that `rm` cannot take a slice of.
	if _, err := os.Stat(filepath.Join(dir, IndexName)); err == nil {
		t.Error("the flat index was written; nothing should append to it again")
	}
}

// A month removed with `rm -rf` takes that month's records and leaves the rest
// readable. That is the claim docs/design/blocks.md makes about retention, and
// until the index was sharded it was only true of the bodies.
func TestAMonthCanBeRemovedWithRm(t *testing.T) {
	s, dir := newStore(t)
	mustAppend(t, s, Record{ID: "A", Command: "august", Start: day(2026, time.August, 4, 9)})
	mustAppend(t, s, Record{ID: "B", Command: "september", Start: day(2026, time.September, 14, 9)})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "index", "2026", "08")); err != nil {
		t.Fatal(err)
	}
	got := Open(dir, boundary.Boundary{}, "READER").Load(t.Context(), 100)
	if len(got) != 1 || got[0].Command != "september" {
		t.Fatalf("after dropping August the store reads %v, want only September", commandsOf(got))
	}
}

// Records read back oldest first across shard boundaries, which is the order
// every caller was written against — a listing is a log and a recency number
// counts back from the end.
func TestShardsReadBackInOrder(t *testing.T) {
	s, dir := newStore(t)
	want := []string{"first", "second", "third", "fourth"}
	starts := []time.Time{
		day(2025, time.December, 31, 23),
		day(2026, time.January, 1, 0),
		day(2026, time.January, 1, 12),
		day(2026, time.February, 2, 8),
	}
	for i, c := range want {
		mustAppend(t, s, Record{ID: "X", Command: c, Start: starts[i]})
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	got := Open(dir, boundary.Boundary{}, "READER").Load(t.Context(), 100)
	if strings.Join(commandsOf(got), "|") != strings.Join(want, "|") {
		t.Errorf("read back %v, want %v", commandsOf(got), want)
	}
	// And a bounded ask takes the newest, still oldest first among them.
	got = Open(dir, boundary.Boundary{}, "READER").Load(t.Context(), 2)
	if strings.Join(commandsOf(got), "|") != "third|fourth" {
		t.Errorf("the last two are %v, want third|fourth", commandsOf(got))
	}
}

// A session left open overnight files its next command under the new day.
//
// Without this the sharding would fail exactly the sessions it is for: a
// terminal open for a week would put the week in the shard for the Monday it
// was started on, and `rm -rf index/2026/08` would take September with it.
func TestASessionPastMidnightRollsOverToTheNextShard(t *testing.T) {
	s, dir := newStore(t)
	mustAppend(t, s, Record{ID: "A", Command: "before", Start: day(2026, time.March, 1, 23)})
	mustAppend(t, s, Record{ID: "B", Command: "after", Start: day(2026, time.March, 2, 0)})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"index/2026/03/01.jsonl": "before",
		"index/2026/03/02.jsonl": "after",
	} {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("no shard at %s: %v", name, err)
		}
		if !strings.Contains(string(b), want) || strings.Count(string(b), "\n") != 1 {
			t.Errorf("%s holds %q, want just the %q record", name, b, want)
		}
	}
}

// A store written before the sharding still reads, and reads as the oldest
// part of it: the flat file holds everything up to the upgrade and nothing
// after, since no version appends to it again.
func TestTheFlatIndexOfAnOlderStoreStillReads(t *testing.T) {
	dir := t.TempDir()
	old := fmt.Appendf(nil,
		"%s%s",
		`{"v":1,"id":"A","command":"long ago","status":0,"start":"2025-01-01T00:00:00Z"}`+"\n",
		`{"v":1,"id":"B","command":"less long ago","status":0,"start":"2025-06-01T00:00:00Z"}`+"\n")
	if err := os.WriteFile(filepath.Join(dir, IndexName), old, 0o600); err != nil {
		t.Fatal(err)
	}
	s := Open(dir, boundary.Boundary{}, "SESSION")
	t.Cleanup(func() { _ = s.Close() })
	mustAppend(t, s, Record{ID: "C", Command: "today", Start: day(2026, time.September, 14, 9)})

	got := s.Load(t.Context(), 100)
	want := []string{"long ago", "less long ago", "today"}
	if strings.Join(commandsOf(got), "|") != strings.Join(want, "|") {
		t.Errorf("read back %v, want %v — the flat index is the oldest part of the store", commandsOf(got), want)
	}
}

// Whatever else is in the store directory is left alone.
//
// A store is a directory a person is invited to take `rm` to, so it collects
// things: an editor's backup, a `.DS_Store`, a directory somebody made by
// hand. Descending into one would be a read nobody asked for, and — because
// the newest-first walk depends on the fixed-width numeric names sorting
// chronologically — a name that is not one of those has no place in the
// sequence at all.
func TestJunkInTheStoreIsNotReadAsAShard(t *testing.T) {
	s, dir := newStore(t)
	mustAppend(t, s, Record{ID: "A", Command: "real", Start: day(2026, time.September, 14, 9)})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	junk := map[string]string{
		"index/notayear/09/14.jsonl":    `{"v":1,"id":"Z","command":"junk","status":0}`,
		"index/2026/notamonth/14.jsonl": `{"v":1,"id":"Z","command":"junk","status":0}`,
		"index/2026/09/notaday.jsonl":   `{"v":1,"id":"Z","command":"junk","status":0}`,
		"index/2026/09/14.jsonl.bak":    `{"v":1,"id":"Z","command":"junk","status":0}`,
	}
	for name, body := range junk {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := Open(dir, boundary.Boundary{}, "READER").Load(t.Context(), 100)
	if len(got) != 1 || got[0].Command != "real" {
		t.Errorf("read back %v, want only the real record", commandsOf(got))
	}
}

// Naming a block reads the newest shard and stops.
//
// The bound is what `cmd/sh`'s blocksLimit has always claimed — that
// `-blocks-show 1` costs the same on a store somebody has been filling for a
// year as on one from this morning — and what the old reader did not have: it
// decoded every record in the store and then took the last n.
func TestABoundedAskOpensOnlyTheNewestShard(t *testing.T) {
	dir := t.TempDir()
	writer := Open(dir, boundary.Boundary{}, "WRITER")
	for d := 1; d <= 20; d++ {
		mustAppend(t, writer, Record{
			ID: "X", Command: fmt.Sprintf("day-%02d", d),
			Start: day(2026, time.April, d, 9),
		})
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	var opened []string
	sink := interp.SinkFunc(func(_ context.Context, e interp.Event) {
		if e.Action.Kind == interp.ActionOpen {
			opened = append(opened, e.Action.Path)
		}
	})
	got := Open(dir, boundary.Boundary{Events: sink}, "READER").Load(t.Context(), 1)
	if len(got) != 1 || got[0].Command != "day-20" {
		t.Fatalf("loaded %v, want the newest record", commandsOf(got))
	}
	if len(opened) != 1 {
		t.Errorf("asking for one record opened %d index files, want 1: %v", len(opened), opened)
	}
}

// countingReaderAt answers reads and remembers how many bytes it was asked
// for, which is the only way to state the bound as a number.
type countingReaderAt struct {
	data []byte
	read int64
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	c.read += int64(len(p))
	return bytes.NewReader(c.data).ReadAt(p, off)
}

// The read is bounded by what was asked for rather than by how big the shard
// is.
//
// This is the half of #2275 the layout alone does not fix: a shard is one
// day's records, and a busy day on a machine that runs commands in a loop is
// still a large file. `Store.Load` used to decode all of it into a slice and
// then keep the tail, which measured at 3.5 s and 1.1 GB of resident memory to
// print one line on a store of 1.3M records.
func TestTheTailReadIsBoundedByWhatWasAsked(t *testing.T) {
	var buf bytes.Buffer
	const records = 20000
	for i := range records {
		line, err := encode(Record{
			V: Version, ID: "X", Status: 0,
			Command: fmt.Sprintf("command number %d with enough text to be a realistic record", i),
			Cwd:     "/some/working/directory/deep/enough/to/be/realistic",
			Start:   day(2026, time.May, 1, 9),
		})
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(line)
	}
	if buf.Len() < 32*tailChunk {
		t.Fatalf("the fixture is %d bytes, too small to tell a bounded read from a whole one", buf.Len())
	}

	c := &countingReaderAt{data: buf.Bytes()}
	got := tailRecords(c, int64(len(c.data)), 1, nil)
	if len(got) != 1 || !strings.HasPrefix(got[0].Command, fmt.Sprintf("command number %d ", records-1)) {
		t.Fatalf("loaded %v, want the last record", commandsOf(got))
	}
	// One chunk is enough for one record; two is the allowance for a record
	// that straddles a chunk boundary. What matters is that it is a constant
	// and not a fraction of the file.
	if limit := int64(2 * tailChunk); c.read > limit {
		t.Errorf("reading one record of a %d-byte shard read %d bytes, want at most %d",
			len(c.data), c.read, limit)
	}
}

// A record that straddles a chunk boundary is rejoined rather than lost.
//
// The backwards walk reads fixed chunks from the end, so most reads begin in
// the middle of a line. Every record must still come back, in order, however
// the boundaries happen to fall — which is the property a reader that only
// ever looked at the last few records would never exercise.
func TestEveryRecordSurvivesTheChunkBoundaries(t *testing.T) {
	var buf bytes.Buffer
	const records = 4000
	for i := range records {
		line, err := encode(Record{
			V: Version, ID: "X", Status: 0,
			Command: fmt.Sprintf("line-%04d", i),
			Cwd:     strings.Repeat("d", i%97), // so lines land on every offset
			Start:   day(2026, time.June, 1, 9),
		})
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(line)
	}
	if buf.Len() < 4*tailChunk {
		t.Fatalf("the fixture is %d bytes, too small to cross several chunks", buf.Len())
	}
	got := tailRecords(bytes.NewReader(buf.Bytes()), int64(buf.Len()), records, nil)
	if len(got) != records {
		t.Fatalf("read %d records back, want %d", len(got), records)
	}
	// Newest first, which is the direction the walk runs in.
	for i, r := range got {
		if want := fmt.Sprintf("line-%04d", records-1-i); r.Command != want {
			t.Fatalf("record %d is %q, want %q", i, r.Command, want)
		}
	}
}

// A line too long to be a record is dropped rather than accumulated.
//
// The walk accumulates the tail of a line until it finds where the line
// starts, so a shard holding one enormous line is the input that could put the
// whole file in memory — the defect this change exists to remove, arriving by
// another door. The bound is the same one the old forward reader gave its
// scanner, so what counts as a record has not moved.
//
// The over-long line here is *valid JSON for a real record*, which is what
// makes this a test: a reader with no bound would decode it and hand it back.
func TestAnOverlongLineIsDroppedRatherThanAccumulated(t *testing.T) {
	huge, err := encode(Record{
		V: Version, ID: "H", Status: 0,
		Command: strings.Repeat("x", maxRecordLine+4*tailChunk),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Clear of the bound rather than on it. The walk carries at most one chunk
	// of a line past the bound before it notices, so a line only just over
	// would be rejoined and decoded — which is a fair reading of "at most this
	// much memory" and not the thing this test is about.
	if len(huge) <= maxRecordLine+tailChunk {
		t.Fatalf("the fixture line is %d bytes, want well past the %d-byte bound", len(huge), maxRecordLine)
	}
	first, err := encode(Record{V: Version, ID: "A", Command: "before", Status: 0})
	if err != nil {
		t.Fatal(err)
	}
	last, err := encode(Record{V: Version, ID: "B", Command: "after", Status: 0})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	buf.Write(first)
	buf.Write(huge)
	buf.Write(last)

	got := tailRecords(bytes.NewReader(buf.Bytes()), int64(buf.Len()), 10, nil)
	if strings.Join(commandsOf(got), "|") != "after|before" {
		t.Errorf("read back %d records %.20q, want the two records with the overlong one dropped",
			len(got), commandsOf(got))
	}
}

// commandsOf is the command of each record, for an assertion that reads.
func commandsOf(recs []Record) []string {
	out := make([]string, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.Command)
	}
	return out
}
