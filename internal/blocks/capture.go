// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// DefaultMaxOutput is how much of one block's output is kept when nothing said.
//
// A number with no measurement behind it, and it is a variable so it can be
// argued with — see docs/design/blocks.md, which flags it. A megabyte holds a
// long build log's beginning and end, and bounds what a command that never
// stops printing can cost.
const DefaultMaxOutput = 1 << 20

// A Capture is the bounded copy a block keeps of what a command printed.
//
// One capture serves both of a command's streams, so what it holds is the two
// interleaved in the order they were written — which is what the person saw,
// and is the thing a block is for. Two separate copies would lose the
// interleaving, and a compiler printing progress on one stream and the error on
// the other is exactly the case where the interleaving is the information.
//
// The faithfulness claim is bounded and worth stating: writes through this are
// ordered by the mutex, and a child process's two streams race through
// os/exec's copy goroutines — but they race on a terminal too, so the copy is
// wrong in the places the screen was.
//
// # Bounded, which is not an aesthetic choice
//
// At most max bytes are kept: the first half and the last half, with the middle
// dropped and a marker saying how much went. Head-only loses the failure, which
// is at the end; tail-only loses what was invoked and what it decided, which is
// at the start; a build log is the worked example of both.
//
// The same bound is what makes this safe at a prompt. `cat /dev/urandom` costs
// a fixed megabyte however long it runs, where a capture that buffered a whole
// command's output would be a shell anyone can exhaust the memory of with a
// command that is behaving normally.
type Capture struct {
	mu sync.Mutex
	// half is how much is kept at each end. Held rather than derived so the
	// two ends cannot disagree about it.
	half int
	head []byte
	tail []byte
	// total is everything written since the last Take, which is what the
	// record reports — a consumer is told the true size rather than the kept
	// size, so it is never guessing about what it is looking at.
	total int64
}

// NewCapture keeps at most max bytes of each block's output.
//
// A max below two is raised to two, so that each end gets at least one byte and
// the halving cannot produce a capture that keeps nothing while claiming to.
func NewCapture(max int) *Capture {
	if max < 2 {
		max = 2
	}
	return &Capture{half: max / 2}
}

// Stream wraps one of a command's streams so that what goes to it is also kept.
//
// The wrapper is what costs a child its terminal, and that is the whole reason
// output capture is opt-in: interp hands a child r.Stdout directly, so an
// *os.File is inherited as a descriptor and anything else makes os/exec build a
// pipe. Downstream of that, isatty is false — `ls` stops colorizing, `git`
// stops paging, and a full-screen program is broken outright. See
// docs/design/blocks.md for why the honest fix is a pseudo-terminal and why
// that is a job-control change rather than an addition.
func (c *Capture) Stream(w io.Writer) io.Writer { return &stream{c: c, w: w} }

// stream is one stream's half of a capture.
type stream struct {
	c *Capture
	w io.Writer
}

// Write passes the bytes on and then keeps a copy.
//
// On first, deliberately. The terminal is what the person is looking at, and a
// capture that failed or blocked must not be able to come between them and
// their output; the copy is the addition and loses if anything has to.
func (s *stream) Write(p []byte) (int, error) {
	n, err := s.w.Write(p)
	if n > 0 {
		s.c.record(p[:n])
	}
	return n, err
}

// record adds bytes to the bounded copy.
func (c *Capture) record(p []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total += int64(len(p))
	if room := c.half - len(c.head); room > 0 {
		n := min(room, len(p))
		c.head = append(c.head, p[:n]...)
		p = p[n:]
	}
	if len(p) == 0 {
		return
	}
	c.tail = append(c.tail, p...)
	// Trimmed in blocks rather than byte by byte: the copy is amortized to one
	// pass per half-buffer of output, where trimming on every write would move
	// the whole tail for every line a command prints.
	if len(c.tail) > 2*c.half {
		c.tail = append(c.tail[:0], c.tail[len(c.tail)-c.half:]...)
	}
}

// Take is what the block that just closed printed, and resets for the next one.
//
// The reset is what makes a session-long capture work per block: a block gets
// everything written since the previous block was closed. Output from a
// background job that finished between two prompts therefore lands in whichever
// block is open when it arrives — which is exactly what a terminal does with
// it, and is why the capture is installed once for the session rather than
// swapped around each command. Swapping would leave `sleep 10 &` writing into a
// sink nobody reads.
func (c *Capture) Take() (text string, total int64, truncated bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	total = c.total
	kept := int64(len(c.head) + len(c.tail))
	truncated = total > kept
	var b strings.Builder
	b.Grow(int(kept) + 40)
	b.Write(c.head)
	if truncated {
		// Said rather than elided: output with a silent hole in it is output
		// somebody reads as corrupt and goes looking for the original of.
		fmt.Fprintf(&b, "\n[%d bytes omitted]\n", total-kept)
	}
	b.Write(c.tail)
	c.head, c.tail, c.total = c.head[:0], c.tail[:0], 0
	return b.String(), total, truncated
}
