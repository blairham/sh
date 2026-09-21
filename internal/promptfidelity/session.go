// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package promptfidelity

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/blairham/sh/internal/cellgrid"
	"github.com/blairham/sh/internal/pty"
)

// A shell on a pseudo-terminal, for the two sources that need one.
//
// Small on purpose. It types lines and waits for the drawing to stop, which
// is the whole of what capturing a prompt needs — there is no mark to wait
// for, because a prompt is what a program draws when it has nothing left to
// do.

// ptySession is a program with a terminal on one end and this on the other.
type ptySession struct {
	cmd     *exec.Cmd
	control *os.File
	tty     *os.File

	mu    sync.Mutex
	bytes []byte
	// drawn is bumped every time something arrives, so a wait can ask
	// whether anything has happened since it last looked rather than
	// sleeping for a fixed time.
	drawn int
	done  chan struct{}
}

// startPty launches a program on a terminal of the pinned width.
//
// The width is set on the terminal itself rather than through `COLUMNS`,
// because that is where a shell reads it from and a prompt with a right
// side is a different prompt at a different width. `COLUMNS` goes in the
// environment too, for the sources that read it instead.
func startPty(program string, argv []string, ctx Context, env []string) (*ptySession, error) {
	control, tty, err := pty.Open()
	if err != nil {
		return nil, err
	}
	if ctx.Columns > 0 {
		// On the terminal end, which is the side the shell holds — see
		// internal/pty.SetSize, and the rows are the fixed 24 a comparison
		// of prompts has no use for beyond "more than the prompt".
		if err := pty.SetSize(tty, 24, ctx.Columns); err != nil {
			_ = control.Close()
			_ = tty.Close()
			return nil, err
		}
	}
	cmd := exec.Command(program, argv[1:]...)
	cmd.Args = argv
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	cmd.Dir = ctx.Dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	cmd.Env = append([]string{
		"TERM=xterm-256color",
		"LC_ALL=en_US.UTF-8",
		"PATH=" + os.Getenv("PATH"),
		"COLUMNS=" + strconv.Itoa(ctx.Columns),
	}, env...)
	if err := cmd.Start(); err != nil {
		_ = control.Close()
		_ = tty.Close()
		return nil, err
	}

	s := &ptySession{cmd: cmd, control: control, tty: tty, done: make(chan struct{})}
	go s.read()
	return s, nil
}

// read drains what the program draws.
func (s *ptySession) read() {
	defer close(s.done)
	buf := make([]byte, 8192)
	for {
		n, err := s.control.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.bytes = append(s.bytes, buf[:n]...)
			s.drawn++
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// line types one command and a return.
func (s *ptySession) line(text string) {
	_, _ = s.control.WriteString(text + "\r")
}

// settle waits until nothing has been drawn for settleFor, or the deadline
// passes.
//
// A quiet period rather than a mark, because a prompt has no end: it stops
// when the program has finished drawing it. A source still drawing after
// the deadline is a source whose row will differ, and differ visibly.
func (s *ptySession) settle(deadline time.Duration) error {
	end := time.Now().Add(deadline)
	last := -1
	quietSince := time.Now()
	for time.Now().Before(end) {
		s.mu.Lock()
		drawn, any := s.drawn, len(s.bytes) > 0
		s.mu.Unlock()
		if drawn != last {
			last, quietSince = drawn, time.Now()
		}
		if any && time.Since(quietSince) >= settleFor {
			return nil
		}
		select {
		case <-s.done:
			return errors.New("it ended before it drew a prompt")
		case <-time.After(5 * time.Millisecond):
		}
	}
	return errors.New("it never stopped drawing")
}

// await waits until a mark has been drawn.
//
// A mark and not a quiet period, for the one question quiet cannot answer:
// **is the program ready to be typed at?** A prompt program that is
// downloading a helper on its first run in a fresh home has quiet periods
// in the middle of the download, and the first run of this instrument
// settled inside one and compared a screen reading "fetching gitstatusd"
// against a prompt. Faithfully, and uselessly.
//
// The mark is never text that is typed. A terminal echoes keystrokes, so a
// wait on a mark the line contains is answered by the echo — the rule
// internal/smoke states — so the line is an expression and the mark is its
// answer.
func (s *ptySession) await(mark string, deadline time.Duration) error {
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		s.mu.Lock()
		found := bytes.Contains(s.bytes, []byte(mark))
		s.mu.Unlock()
		if found {
			return nil
		}
		select {
		case <-s.done:
			return errors.New("it ended before it was ready")
		case <-time.After(5 * time.Millisecond):
		}
	}
	return errors.New("it never became ready")
}

// ready types an expression and waits for its answer, which is the proof
// that the program is at a prompt and running what it is given.
func (s *ptySession) ready(deadline time.Duration) error {
	s.line("echo fidelity-$((6 * 7))-ok")
	return s.await("fidelity-42-ok", deadline)
}

// screen is what a terminal of this width would be showing.
func (s *ptySession) screen(cols int) *cellgrid.Grid {
	s.mu.Lock()
	defer s.mu.Unlock()
	grid := cellgrid.New(cols)
	_, _ = grid.Write(s.bytes)
	return grid
}

// close ends the program and takes the terminal away.
//
// The kill is by process *group*, because a source starts background jobs
// to pin a job count and a kill at the top would leave them: a harness that
// left a `sleep` per row behind would be the thing this repository's own
// rules were most recently tightened about.
func (s *ptySession) close() {
	if s.cmd.Process != nil {
		if pgid, err := syscall.Getpgid(s.cmd.Process.Pid); err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		} else {
			_ = s.cmd.Process.Kill()
		}
	}
	_ = s.control.Close()
	_ = s.tty.Close()
	_ = s.cmd.Wait()
	<-s.done
}
