// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"fmt"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/event"
)

// What goes to a stream still reaches it. The capture is the addition, and a
// copy that came between somebody and their output would be the feature
// breaking the shell.
func TestCapturedOutputStillReachesTheStream(t *testing.T) {
	var out strings.Builder
	c := NewCapture(DefaultMaxOutput)
	w := c.Stream(&out)
	if _, err := fmt.Fprint(w, "hello"); err != nil {
		t.Fatal(err)
	}
	if out.String() != "hello" {
		t.Errorf("the stream got %q", out.String())
	}
	text, total, truncated := c.Take()
	if text != "hello" || total != 5 || truncated {
		t.Errorf("captured %q, %d, %v", text, total, truncated)
	}
}

// Both streams go to one capture, interleaved in the order they were written.
// That is what the person saw, and two files would lose it.
func TestBothStreamsAreOneBodyInWriteOrder(t *testing.T) {
	var out, err strings.Builder
	c := NewCapture(DefaultMaxOutput)
	o, e := c.Stream(&out), c.Stream(&err)
	for _, w := range []struct {
		to   interface{ Write([]byte) (int, error) }
		text string
	}{{o, "one "}, {e, "two "}, {o, "three"}} {
		if _, werr := w.to.Write([]byte(w.text)); werr != nil {
			t.Fatal(werr)
		}
	}
	text, _, _ := c.Take()
	if text != "one two three" {
		t.Errorf("captured %q, want the two streams in write order", text)
	}
	// And each stream still carries only its own.
	if out.String() != "one three" || err.String() != "two " {
		t.Errorf("streams got %q and %q", out.String(), err.String())
	}
}

// Over the cap, the head and the tail are kept and the middle is said to be
// gone. Head-only loses the failure and tail-only loses the invocation.
func TestOverTheCapTheHeadAndTailSurvive(t *testing.T) {
	c := NewCapture(20)
	var sent strings.Builder
	for i := range 100 {
		line := fmt.Sprintf("%d\n", i)
		sent.WriteString(line)
		if _, err := c.Stream(&strings.Builder{}).Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	text, total, truncated := c.Take()
	if !truncated {
		t.Fatal("output well over the cap was not marked truncated")
	}
	if total != int64(sent.Len()) {
		t.Errorf("total is %d, want the true %d — a consumer would be guessing",
			total, sent.Len())
	}
	if !strings.HasPrefix(text, "0\n") {
		t.Errorf("body is %q, want the beginning kept", text)
	}
	if !strings.HasSuffix(text, "99\n") {
		t.Errorf("body is %q, want the end kept", text)
	}
	if !strings.Contains(text, "bytes omitted") {
		t.Errorf("body is %q, want a marker saying the middle went", text)
	}
	// The kept text is bounded, which is what stops a command that never stops
	// printing from costing the shell its memory.
	if len(text) > 20+40 {
		t.Errorf("kept %d bytes for a cap of 20", len(text))
	}
}

// Under the cap nothing is dropped and nothing claims to have been.
func TestUnderTheCapNothingIsMarked(t *testing.T) {
	c := NewCapture(1000)
	if _, err := c.Stream(&strings.Builder{}).Write([]byte("short")); err != nil {
		t.Fatal(err)
	}
	text, total, truncated := c.Take()
	if text != "short" || total != 5 || truncated {
		t.Errorf("captured %q, %d, %v", text, total, truncated)
	}
}

// However much is printed, the capture holds a bounded amount. Without this a
// `cat /dev/urandom` at the prompt is a shell anybody can exhaust the memory
// of with a command that is behaving normally.
func TestMemoryIsBoundedByTheCap(t *testing.T) {
	c := NewCapture(64)
	chunk := strings.Repeat("x", 4096)
	sink := &strings.Builder{}
	w := c.Stream(sink)
	for range 500 {
		sink.Reset()
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
		if held := len(c.head) + len(c.tail); held > 4*64 {
			t.Fatalf("the capture holds %d bytes for a cap of 64", held)
		}
	}
	_, total, truncated := c.Take()
	if !truncated || total != int64(500*len(chunk)) {
		t.Errorf("total is %d, truncated %v", total, truncated)
	}
}

// Take is per block: what the next block gets is what was written after the
// last one closed, not the session so far.
func TestTakeResetsForTheNextBlock(t *testing.T) {
	c := NewCapture(DefaultMaxOutput)
	w := c.Stream(&strings.Builder{})
	if _, err := w.Write([]byte("first")); err != nil {
		t.Fatal(err)
	}
	if text, _, _ := c.Take(); text != "first" {
		t.Fatalf("the first block got %q", text)
	}
	if text, total, _ := c.Take(); text != "" || total != 0 {
		t.Fatalf("a block with no output got %q, %d", text, total)
	}
	if _, err := w.Write([]byte("second")); err != nil {
		t.Fatal(err)
	}
	if text, total, _ := c.Take(); text != "second" || total != 6 {
		t.Errorf("the second block got %q, %d — the first leaked into it", text, total)
	}
}

// A cap too small to halve still keeps something at each end rather than
// claiming to keep output and keeping none.
func TestATinyCapStillKeepsSomething(t *testing.T) {
	c := NewCapture(0)
	if _, err := c.Stream(&strings.Builder{}).Write([]byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	text, total, truncated := c.Take()
	if total != 6 || !truncated {
		t.Fatalf("captured %d, %v", total, truncated)
	}
	if !strings.HasPrefix(text, "a") || !strings.HasSuffix(text, "f") {
		t.Errorf("body is %q, want a byte from each end", text)
	}
}

// The store writes the body before the record that names it, so a record
// always points at a file that exists.
func TestRecordWritesTheBodyAndNamesIt(t *testing.T) {
	s, _ := newStore(t)
	err := s.Record(t.Context(),
		Record{ID: event.NewID(at(1000)), Command: "build", Start: at(1000)},
		Output{Text: "the log\n", Bytes: 900, Truncated: true})
	if err != nil {
		t.Fatal(err)
	}
	got := s.Load(t.Context(), 10)
	if len(got) != 1 {
		t.Fatalf("recorded %d blocks", len(got))
	}
	r := got[0]
	if r.Output == "" || r.OutputBytes != 900 || !r.Truncated || r.Streams != StreamsMerged {
		t.Fatalf("record is %+v, want it naming the body and its true size", r)
	}
	if body, ok := s.Body(t.Context(), r); !ok || body != "the log\n" {
		t.Errorf("body read back as %q, %v", body, ok)
	}
}

// A block whose command carries a credential leaves nothing at all — not even
// the body, which is the half worth having. Writing the output and then
// declining the record would leave it on disk with nothing pointing at it.
func TestACredentialLeavesNoBodyEither(t *testing.T) {
	s, dir := newStore(t)
	err := s.Record(t.Context(),
		Record{ID: event.NewID(at(1000)), Command: "export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE"},
		Output{Text: "some output", Bytes: 11})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Load(t.Context(), 10); len(got) != 0 {
		t.Errorf("recorded %+v", got)
	}
	if entries, rerr := readAll(dir); rerr != nil {
		t.Fatal(rerr)
	} else if len(entries) != 0 {
		t.Errorf("the store holds %v, want nothing", entries)
	}
}
