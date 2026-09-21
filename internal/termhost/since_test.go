// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package termhost_test

import (
	"context"
	"io"
	"testing"

	"github.com/blairham/sh/internal/termhost"
)

// Since is what a streaming front end reads, and it is the one piece of this
// machinery neither protocol had before. Its whole job is to answer "what is
// new" against an offset that only ever grows, which is what a progress value
// has to be — and the retained buffer's length is not that, because a limit
// makes it shrink.

// writing stands a terminal up whose command is a function the test drives.
// Nothing is exec'd: the question is about the buffer, and a real program
// would put a process's scheduling between the test and its assertion.
func writing(t *testing.T, limit *int) (*termhost.Terminal, func(string)) {
	t.Helper()
	writes := make(chan string)
	written := make(chan struct{})
	h := &termhost.Host{
		Interpret: func(_ context.Context, _ termhost.Command, out io.Writer) int {
			for s := range writes {
				_, _ = io.WriteString(out, s)
				written <- struct{}{}
			}
			return 0
		},
	}
	id, err := h.Create(t.Context(), termhost.Request{Command: "driven", OutputByteLimit: limit})
	if err != nil {
		t.Fatal(err)
	}
	term, ok := h.Lookup(id)
	if !ok {
		t.Fatalf("the host lost %s the moment it made it", id)
	}
	t.Cleanup(func() { close(writes); h.Close() })
	// The write is handed over and waited for, so that every assertion below
	// is about the buffer rather than about when a goroutine got to run.
	return term, func(s string) {
		t.Helper()
		writes <- s
		<-written
	}
}

func TestSinceAnswersOnlyWhatIsNew(t *testing.T) {
	t.Parallel()
	term, write := writing(t, nil)

	write("first")
	text, at := term.Since(0)
	if text != "first" || at != 5 {
		t.Fatalf("Since(0) is %q at %d, want %q at 5", text, at, "first")
	}
	write("second")
	text, at = term.Since(at)
	if text != "second" || at != 11 {
		t.Errorf("Since(5) is %q at %d, want %q at 11: a reader was handed what it already had", text, at, "second")
	}
	// And nothing at all when nothing was written, which is what keeps a
	// progress value increasing: a caller with nothing new has no honest
	// number to send.
	text, again := term.Since(at)
	if text != "" || again != at {
		t.Errorf("Since with nothing new is %q at %d, want empty at %d", text, again, at)
	}
}

func TestSinceKeepsCountingPastWhatTruncationDropped(t *testing.T) {
	t.Parallel()
	four := 4
	term, write := writing(t, &four)

	write("aaaa")
	_, at := term.Since(0)
	if at != 4 {
		t.Fatalf("the offset is %d after four bytes, want 4", at)
	}
	write("bbbbbb")
	text, now := term.Since(at)
	if now != 10 {
		t.Errorf("the offset is %d after ten bytes, want 10.\n"+
			"\tIt counts everything ever written, not what is retained: a limit makes\n"+
			"\tthe buffer shrink, and a progress value that went backwards would read\n"+
			"\tto a client as a stall.", now)
	}
	// What is left of the unseen bytes is what the buffer still holds. The
	// gap is not reported here — Snapshot's Truncated is where that is said,
	// and saying it twice in two vocabularies is how the two disagree.
	if text != "bbbb" {
		t.Errorf("Since returned %q, want the four bytes still retained", text)
	}
	if !term.Snapshot().Truncated {
		t.Error("bytes were dropped and Truncated is false")
	}
}

func TestSinceNeverCutsACharacterInHalf(t *testing.T) {
	t.Parallel()
	// Two three-byte characters and a limit that lands inside the first.
	four := 4
	term, write := writing(t, &four)

	write("日本")
	text, _ := term.Since(0)
	for i, r := range text {
		if r == '�' {
			t.Fatalf("Since returned %q, whose byte %d is half a character.\n"+
				"\tA JSON string holding one arrives as mojibake, which is the failure\n"+
				"\tthe trim rule exists to prevent.", text, i)
		}
	}
	if text != "本" {
		t.Errorf("Since returned %q, want the one whole character that fits", text)
	}
}
