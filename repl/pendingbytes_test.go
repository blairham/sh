// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"testing"
)

// pendingBytes is zsh's `$PENDING`: the typed bytes no key has read yet,
// wherever they are — taken off the terminal into held, or still on the
// descriptor — and never what an action pushed back, which zsh counts
// separately as `$KEYS_QUEUED_COUNT`. Measured 2026-10-15 on zsh 5.9.2
// through a pseudo-terminal, `abcdef` typed in one write: the six keys read
// 5, 4, 3, 2, 1, 0 (#6363).
func TestPendingBytesCountsTypedInputWhereverItIs(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = r.Close(), w.Close() })
	e := &editor{inFd: func() int { return int(r.Fd()) }}

	if got := e.pendingBytes(); got != 0 {
		t.Errorf("nothing typed: %d pending, want 0", got)
	}
	// In hand: the rest of one read, after the key being handled.
	copy(e.held[:], "abcdef")
	e.heldLen, e.heldPos = 6, 1
	if got := e.pendingBytes(); got != 5 {
		t.Errorf("five bytes left in hand: %d pending, want 5", got)
	}
	// On the descriptor as well: arrived after the read, and still counted.
	if _, err := w.Write([]byte("gh")); err != nil {
		t.Fatal(err)
	}
	if got := e.pendingBytes(); got != 7 {
		t.Errorf("five in hand and two on the descriptor: %d pending, want 7", got)
	}
	// What an action pushed back is not typed input.
	e.pushed = []byte("xyz")
	if got := e.pendingBytes(); got != 7 {
		t.Errorf("with three bytes pushed back: %d pending, want 7", got)
	}
}
