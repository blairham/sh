// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"slices"
	"sync"
	"time"
)

// A placeholder that leads a body's process group (procanchor.go) is a
// process, and starting one is a fork and an exec from a shell whose heap is
// as large as the configuration it has read. On the configuration this was
// measured on that is ~2 ms, and it lands on the one path where a person is
// waiting for it: zsh-autosuggestions starts a `<(…)` worker at every keystroke
// whose first line is `echo $sysparams[pid]`, and the shell's next statement
// is a `read` of that line. So the whole start was between a key and its
// redraw (#6420).
//
// # One started ahead
//
// The body does not need *a new process*. It needs a group nobody else holds,
// led by a placeholder that ends when the body's family does — and a
// placeholder started a moment earlier is exactly that, because it has been
// doing nothing and nothing has been given its number. So once a body has
// asked, one more is started in the background a little later (spareDelay
// says why later), and the next body to ask takes it rather than starting its
// own. The fork still happens, once per
// asking body as before; it is moved off the path between the question and
// the answer.
//
// What a script observes is unchanged: a real process number, the leader of a
// group containing nothing of the shell's, signaled and waited for exactly as
// before. The pid zsh would report is its fork's, which is also fresh and also
// leads nothing else.
//
// # What it costs
//
// Nothing for a shell whose bodies never ask: the first spare is started only
// after the first body has asked and been answered the old way. After that,
// one idle placeholder — reading a pipe and holding no terminal, no
// descriptors and no environment — for as long as the shell runs. It goes
// when the shell does, by the same end-of-file that ends every placeholder:
// the shell's CleanUp closes the pipe, and a shell that died without one
// closes it by dying.

// heldGroup is one started placeholder: the group it leads, the pipe end whose
// closing ends it, and a channel closed once it has been waited for.
type heldGroup struct {
	pgid   int
	stdin  *os.File
	exited chan struct{}
}

// startHeldGroup starts a placeholder and the goroutine that waits for it,
// because nothing else will: it is not a job, not `$!`, and not a command any
// script named. The wait is also what tells the exec path the group has gone.
func startHeldGroup(argv []string) (*heldGroup, error) {
	cmd, stdin, err := startProcAnchor(argv)
	if err != nil {
		return nil, err
	}
	h := &heldGroup{pgid: cmd.Process.Pid, stdin: stdin, exited: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(h.exited)
	}()
	return h, nil
}

// hasExited reports that the placeholder has been waited for.
func (h *heldGroup) hasExited() bool { return closedChan(h.exited) }

// release delivers the end-of-file the placeholder is reading for.
func (h *heldGroup) release() {
	if h != nil && h.stdin != nil {
		_ = h.stdin.Close()
	}
}

// anchorSpares keeps one placeholder started ahead of the body that will take
// it. Shared down the clone chain, like inventedJobs: every body of one shell
// takes from, and refills, the same one.
type anchorSpares struct {
	mu sync.Mutex
	// argv is the command the kept spare was started with. A spare for a
	// different command is not handed out: an embedder that changed
	// ProcessAnchor asked for a different placeholder.
	argv []string
	// next delivers the kept spare once its start has finished, and nil
	// where that start failed. Nil while nothing is kept.
	next chan *heldGroup
	// done is set by CleanUp. A body still running after its shell has ended
	// is answered the old way and does not leave a spare behind it.
	done bool
	// timer starts the next spare once the shell has been left alone for
	// spareDelay. See refillLater.
	timer *time.Timer
}

// spareDelay is how long after a body has taken a placeholder the next one is
// started.
//
// Not at once, and that was measured rather than guessed. A fork copies this
// process's address space with its map locked, and Go holds its fork lock for
// the whole of it — so a start running beside the shell is not off the path at
// all: every other thread that faults a page or makes a pipe waits for it. On
// the configuration #6420 measured, refilling at once took the `read` of the
// pid from ~2.0 ms to 0.07 ms and gave most of it back elsewhere — the
// substitution's own pipe +0.3 ms, the statement after it +0.3 ms, and the
// syntax highlighter that runs next +0.8 ms. A keystroke's work is over within
// a few milliseconds of the key, and a person types several tens of
// milliseconds apart, so a start this far behind the take lands in time
// nobody is waiting through.
const spareDelay = 30 * time.Millisecond

// take answers a body's first question with a started placeholder: the kept
// one if there is one, a new one otherwise. Either way it then starts the
// next one in the background.
func (s *anchorSpares) take(argv []string) (*heldGroup, error) {
	if s == nil {
		return startHeldGroup(argv)
	}
	s.mu.Lock()
	next, same := s.next, slices.Equal(s.argv, argv)
	s.next = nil
	s.mu.Unlock()
	var h *heldGroup
	if next != nil {
		h = <-next
		if h != nil && (!same || h.hasExited()) {
			h.release()
			h = nil
		}
	}
	if h == nil {
		// Started here, on the asking body's time, and *before* the
		// refill below: two starts at once would serialize on the fork
		// lock and the body would wait for both.
		var err error
		if h, err = startHeldGroup(argv); err != nil {
			return nil, err
		}
	}
	s.refillLater(argv)
	return h, nil
}

// refillLater arranges for the next spare to be started after spareDelay, and
// pushes that back if a body asks again first: a shell asking in a tight loop
// is answered the old way, one start per body, rather than racing a background
// start for the fork lock.
func (s *anchorSpares) refillLater(argv []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(spareDelay, func() { s.refill(argv) })
}

// refill starts the next spare on a goroutine of its own.
func (s *anchorSpares) refill(argv []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done || s.next != nil {
		return
	}
	ch := make(chan *heldGroup, 1)
	s.argv, s.next = argv, ch
	go func() {
		h, err := startHeldGroup(argv)
		if err != nil {
			h = nil
		}
		ch <- h
	}()
}

// stop ends the kept spare and keeps no more. Waits for a start in flight, so
// that the placeholder it makes is ended rather than orphaned onto nobody.
func (s *anchorSpares) stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	next := s.next
	s.next, s.done = nil, true
	if s.timer != nil {
		s.timer.Stop()
	}
	s.mu.Unlock()
	if next != nil {
		(<-next).release()
	}
}
