// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"io"
	"strings"
	"testing"
)

// readsInChunks hands the editor one read at a time, and records what had been
// drawn when the editor came back for the next one — which is what the screen
// shows while the shell waits for a key.
type readsInChunks struct {
	reads []string
	out   *strings.Builder
	seen  []string
}

func (c *readsInChunks) Read(p []byte) (int, error) {
	c.seen = append(c.seen, c.out.String())
	if len(c.reads) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.reads[0])
	c.reads = c.reads[1:]
	return n, nil
}

// Typed text and a key that draws nothing, arriving in one read, are drawn
// before the editor waits for the next key.
//
// Until #5881 the text was not: a character typed with more input in hand
// puts its draw off, and only a later character or the end of the line
// flushed it, so a read ending on a key that draws nothing left `abc` in the
// line and off the screen until the next keystroke. Measured 2026-10-04
// through a pseudo-terminal, zsh 5.9.2 shows `abc` at once for each of the
// keys below written in one write with it.
func TestTextTypedBeforeAKeyThatDrawsNothingIsDrawnBeforeTheWait(t *testing.T) {
	for _, key := range []string{
		"\x1b[200~\x1b[201~", // an empty paste
		"\x1bz",              // an escape pair nothing binds here
		"\x1b[15~",           // F5, unbound
	} {
		var out strings.Builder
		in := &readsInChunks{reads: []string{"abc" + key, "\r"}, out: &out}
		e := &editor{in: in, out: &out}
		line, err := e.readLine(drawPrompt("$ "))
		if err != nil || line != "abc" {
			t.Fatalf("after %q the line is %q, %v; want abc", key, line, err)
		}
		// seen[0] is before anything was read and seen[1] is the wait for the
		// carriage return.
		if len(in.seen) < 2 || !strings.Contains(in.seen[1], "abc") {
			t.Errorf("after %q, abc was not drawn before the next key was waited on; drawn: %q", key, in.seen)
		}
	}
}
