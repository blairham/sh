// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package repl

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"unsafe"

	"github.com/blairham/sh/internal/pty"
)

// A ptyConduit is a pseudo-terminal between the shell's children and the
// terminal a person is looking at.
//
// It exists so that output can be *kept* without a child losing its terminal.
// interp hands a child `r.Stdout` directly: an *os.File is inherited as a
// descriptor, anything else makes os/exec build a pipe, and downstream of that
// `isatty` is false — `ls` stops colorizing, `git` stops paging, a full-screen
// program breaks. Putting a pseudo-terminal there means the writer is still a
// terminal file, so the child is on a terminal; the shell reads the other end,
// writes it where it was going, and keeps a copy on the way past.
//
// # Only the output half moves, which is the whole reason this is small
//
// docs/design/blocks.md predicted this as "a rewrite of the job-control path
// rather than an addition to it", because the shell would have to forward
// window size, terminal ownership and the stop signals to the inner terminal.
// Measured on 2026-09-06, two of those three do not arise: **standard input
// stays on the real terminal.** A child's fd 0 is the terminal the repl
// already hands to a foreground process group through /dev/tty, so terminal
// ownership, ^C and ^Z are exactly where they were and this change cannot move
// them. Only fd 1 and fd 2 are the inner terminal, and the only thing the
// inner one has to be told is how big it is.
//
// The probe, run under two pseudo-terminals with a child's stdin on one and
// its stdout on the other:
//
//	test -t 0 && echo IN_TTY   →  IN_TTY
//	test -t 1 && echo OUT_TTY  →  OUT_TTY
//
// Both, which is what a child on a terminal sees and what a captured child
// does not see today.
//
// # Two things the inner terminal must be told
//
// Its **output discipline is off** (OPOST), so it is a conduit rather than a
// second terminal doing the work again. A pty slave translates \n to \r\n by
// default; the real terminal then does it again to the \n that is left,
// producing \r\r\n. Measured — the first probe above came back
// "IN_TTY\r\nOUT_TTY\r\n" through a slave in its default discipline.
//
// Its **size is the real one's**, set when it is opened and again on every
// SIGWINCH. Without that a full-screen program asks fd 1 how wide the terminal
// is and is told 0 by 0, which is a worse answer than the wrong one.
type ptyConduit struct {
	master, slave *os.File

	// mark is this session's private end-of-block token, written into the
	// conduit after a command has finished so that the shell can wait for the
	// pump to reach it. See drain.
	mark []byte
	// reached is signaled by the pump each time it passes a mark.
	reached chan struct{}

	// winch is where SIGWINCH is delivered while this exists, and winchDone
	// is closed when the goroutine reading it has stopped.
	//
	// Two, because stopping the delivery is not the same as the reader having
	// finished: signal.Stop guarantees no *further* sends, and says nothing
	// about a resize already in progress. Closing the inner terminal while
	// that resize is holding its descriptor is a race the detector finds and
	// a descriptor a reused fd number turns into somebody else's ioctl.
	winch     chan os.Signal
	winchDone chan struct{}
	// real is the terminal whose size the inner one copies.
	real *os.File

	closeOnce sync.Once
	// done is closed when the pump has finished, so close can wait for it and
	// a caller is never told the conduit is shut while bytes are still being
	// written to the terminal behind it.
	done chan struct{}
}

// newPtyConduit puts a pseudo-terminal in front of out and starts copying.
//
// out is where the bytes were going and where they still go; keep is told
// every byte on the way past, and is what the block records. The order is
// deliberate and is the same one Capture.Stream already keeps: the terminal is
// what the person is looking at, and the copy is the addition.
func newPtyConduit(real *os.File, out io.Writer, keep func(io.Writer) io.Writer) (*ptyConduit, error) {
	master, slave, err := pty.Open()
	if err != nil {
		return nil, err
	}
	c := &ptyConduit{
		master: master, slave: slave, real: real,
		reached: make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	if err := rawOutput(slave); err != nil {
		_ = master.Close()
		_ = slave.Close()
		return nil, err
	}
	c.mark = newConduitMark()
	c.resize()
	c.watchResize()
	go c.pump(keep(out))
	return c, nil
}

// Stream is the file a child writes to, and it is a terminal.
//
// An *os.File and not a wrapper, which is the whole point: a wrapper is what
// costs the child its terminal, so the thing handed to the Runner has to be
// the real thing.
func (c *ptyConduit) Stream() *os.File { return c.slave }

// newConduitMark is a token no command's output will contain.
//
// Random per session rather than a fixed word: the pump strips this from the
// stream, and a fixed word would be stripped out of a command that printed it.
// Hex digits only, so nothing in it is a byte a line discipline rewrites.
//
// If the system will not give randomness the mark is the fixed fallback, and
// draining then risks swallowing a command that printed exactly it — which is
// a worse shell than one that cannot start, so it is not an error.
func newConduitMark() []byte {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return []byte("\x00sh-block-mark\x00")
	}
	out := make([]byte, 0, 2+hex.EncodedLen(len(b)))
	out = append(out, 0x00)
	out = append(out, []byte(hex.EncodeToString(b[:]))...)
	return append(out, 0x00)
}

// drain blocks until everything written before it has reached the terminal.
//
// This is the one piece of synchronization the conduit needs, and it is an
// in-band mark rather than a wait for quiet — the same discipline the pty
// tests follow, and for the same reason: a wait for quiet passes early on a
// slow writer and hangs on a busy one, and neither failure is visible.
//
// A pseudo-terminal is a queue. Writing the mark to the slave after the
// command has exited puts it *behind* every byte that command wrote, so the
// pump seeing the mark is proof that it has copied all of them. Anything a
// background job writes afterwards lands in the next block, which is what a
// terminal does with it too.
//
// A conduit whose pump has stopped returns rather than waiting: the session is
// ending, and the last block is worth more than a prompt that never comes.
func (c *ptyConduit) drain() {
	if c == nil {
		return
	}
	if _, err := c.slave.Write(c.mark); err != nil {
		return
	}
	select {
	case <-c.reached:
	case <-c.done:
	}
}

// pump copies the inner terminal to the outer one, stripping the marks.
func (c *ptyConduit) pump(out io.Writer) {
	defer close(c.done)
	// A carry of one byte less than the mark, so a mark split across two reads
	// is still found. Without it the mark would be written to the terminal and
	// the drain would hang — and it would happen only under a read boundary
	// landing inside it, which is to say rarely and unreproducibly.
	carry := make([]byte, 0, len(c.mark))
	buf := make([]byte, 32*1024)
	for {
		n, err := c.master.Read(buf)
		if n > 0 {
			carry = c.forward(out, append(carry, buf[:n]...))
		}
		if err != nil {
			if len(carry) > 0 {
				_, _ = out.Write(carry)
			}
			return
		}
	}
}

// forward writes everything up to a possible partial mark and returns what it
// held back.
func (c *ptyConduit) forward(out io.Writer, b []byte) []byte {
	for {
		i := bytes.Index(b, c.mark)
		if i < 0 {
			break
		}
		if i > 0 {
			_, _ = out.Write(b[:i])
		}
		b = b[i+len(c.mark):]
		// Non-blocking: a drain that has already given up must not make the
		// pump wait for somebody to take the signal.
		select {
		case c.reached <- struct{}{}:
		default:
		}
	}
	// What is held back is only ever a prefix of the mark, so a command that
	// prints nothing else still reaches the terminal on the next write.
	keep := partialSuffix(b, c.mark)
	if keep > 0 {
		_, _ = out.Write(b[:len(b)-keep])
		return append([]byte(nil), b[len(b)-keep:]...)
	}
	_, _ = out.Write(b)
	return nil
}

// partialSuffix is how many trailing bytes of b could still become a mark.
//
// The longest is taken rather than the first found. With the mark newConduitMark
// builds, the two cannot differ, and that is a property of the mark rather than
// luck: two lengths both matching would need the mark to hold a NUL somewhere
// between its first byte and its last, and it holds hex digits there. A test
// asserts that shape, because it is what makes this scan unambiguous and a
// later change to the mark could take it away without anything else noticing.
//
// Written the general way anyway, so the scan stays correct for a mark that
// does not have it.
func partialSuffix(b, mark []byte) int {
	longest := min(len(mark)-1, len(b))
	for n := longest; n > 0; n-- {
		if bytes.Equal(b[len(b)-n:], mark[:n]) {
			return n
		}
	}
	return 0
}

// resize gives the inner terminal the outer one's size.
func (c *ptyConduit) resize() {
	rows, cols := terminalSize(c.real)
	if rows == 0 && cols == 0 {
		return
	}
	_ = pty.SetSize(c.slave, rows, cols)
}

// watchResize keeps the two the same size for as long as the conduit lives.
//
// A signal rather than asking afresh, which is the opposite of what
// terminalWidth does and is right for the opposite reason: the editor asks
// when it is about to draw, and there is nobody to ask on behalf of a child
// that is already running full-screen. The kernel tells the child by signaling
// it, and it will only do that if the size it is asking about has changed.
func (c *ptyConduit) watchResize() {
	c.winch = make(chan os.Signal, 1)
	c.winchDone = make(chan struct{})
	signal.Notify(c.winch, syscall.SIGWINCH)
	go func() {
		defer close(c.winchDone)
		for range c.winch {
			c.resize()
		}
	}()
}

// close shuts the conduit and waits for what is in it to reach the terminal.
func (c *ptyConduit) close() {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
		if c.winch != nil {
			// Stop the delivery, end the loop, and wait for it: the watcher
			// may be inside a resize right now, and that resize is holding
			// the descriptor closed two lines below.
			signal.Stop(c.winch)
			close(c.winch)
			<-c.winchDone
		}
		// The slave then: with the last writer gone the master reads end of
		// file, which is what stops the pump. Closing the master first would
		// stop it by error and lose whatever was still queued.
		_ = c.slave.Close()
		<-c.done
		_ = c.master.Close()
	})
}

// rawOutput turns off a terminal's output post-processing.
//
// Only the output flags: this terminal has no reader, so its input discipline
// is nobody's business, and touching more of it than the one thing that is
// wrong would be a second change hidden inside this one.
func rawOutput(f *os.File) error {
	fd := int(f.Fd())
	var t syscall.Termios
	if err := ioctl(fd, tcGets, &t); err != nil {
		return err
	}
	t.Oflag &^= syscall.OPOST
	return ioctl(fd, tcSets, &t)
}

// terminalSize is the terminal's rows and columns, or zeroes if it will not
// say.
func terminalSize(f *os.File) (rows, cols int) {
	if f == nil {
		return 0, 0
	}
	var ws winsize
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, f.Fd(),
		syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws)), 0, 0, 0)
	if errno != 0 {
		return 0, 0
	}
	return int(ws.rows), int(ws.cols)
}
