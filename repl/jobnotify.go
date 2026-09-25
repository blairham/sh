// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"sync"

	"github.com/blairham/sh/internal/fdset"
	"github.com/blairham/sh/interp"
)

// A finished job reported the moment it ends, rather than at the next prompt.
//
// This is the asynchronous half of the job notice, and it is the same shape as
// promptasync.go's — a pipe the session owns, poked from whatever goroutine
// noticed, waited on where the session is already idle with a descriptor in
// its hand. It is a second user of that mechanism rather than a second
// mechanism, and the type is shared for exactly the reason watchfd.go gives:
// an editor between keystrokes is the only place in a shell that is idle with
// a descriptor to wait on, and a Go channel cannot wake a select(2).
//
// # What is measured
//
// zsh's `NOTIFY`, on by default there and off in the rest of the panel:
// "report the status of background jobs immediately, rather than waiting until
// just before printing a prompt". Measured 2026-09-25 on a pseudo-terminal
// with `TERM=dumb`, `PS1`/`PS2` exported empty and zsh 5.9.2 started `-fiV
// +Z`, `sleep 0.4 &` and no signal anywhere:
//
//	                         notify on             notify off
//	nothing else running     at once               after the next command
//	a foreground command     at once, ahead of     after that command's
//	  still running            its output            output
//	`wait` for the job       before `wait` returns after it returns
//
// **The option is the noun.** The right-hand column is byte-for-byte what this
// shell did for all three rows before #4524, so the option was the only
// variable; and the left-hand column's second row is what says the rule is not
// "when the shell is next idle" — the notice lands in the middle of `sleep
// 1.5; print FGDONE`, ahead of `FGDONE`.
//
// # What this delivers, and what it does not
//
// The first row and the third are what a session waiting for a line can
// observe, and they are what this serves: the wake is looked at exactly where
// watchfd.go's is, in the editor's own read. The second row needs the notice
// written while a *foreground command* holds the terminal, which is a moment
// this shell has no seam at — interp is running the command and the front end
// is inside a wait — so there it still arrives when that command is done.
// That is a narrower lag than the one #4524 is about and it is stated rather
// than hidden; #4531 is where it is written down.
//
// # Where the line is drawn on the screen
//
// Two answers, and both are measured rather than guessed.
//
// With the line editor **off** — `+Z`, which is how zsh's own `W02jobs.ztst`
// drives the shell — the notice is written where the cursor is and nothing is
// redrawn. Measured with a two-row `PS1`: real zsh writes `PR1`, `zz> ` and
// then `[1]  + done       sleep 0.5` straight after the prompt, on the prompt's
// own row, and draws no new prompt afterwards.
//
// With the editor **on**, the notice goes on a row of its own and the prompt
// and the line being typed are drawn again under it. Measured the same day
// with `PS1=$'ROW1\nzn> '` and `print MID` typed but not submitted: a newline,
// then the notice, then both rows of the prompt with `print MID` back on the
// second one. That redraw is the part a shell gets wrong by doing nothing —
// a notice written into the middle of a half-typed line leaves the line
// looking like it was eaten.

// A jobWake is the descriptor a finished job pokes, opened the first time a
// session asks for it and not before.
//
// **Lazy, because the option can move under a session that is already
// running.** `unsetopt notify` in a startup file and `setopt notify` typed at
// the prompt afterwards both have to work, and a wake decided once at the
// start could not be armed later — so what is decided once is the *wiring*,
// and whether there is a descriptor in the wait is asked each time round it.
//
// The other half of that bargain is what watchfd.go promises: "with nothing to
// watch, a keystroke is read exactly the way it was read before this file
// existed, with no extra call in front of it". A session whose dialect holds
// the notice for the next prompt is answered here with a negative descriptor,
// the wait is handed an empty set, and it returns without a system call. What
// the four quiet dialects pay is one predicate per read.
//
// The mutex is for the poke, which arrives on whatever goroutine the job ended
// on; fd and close are the session's own. A poke that lands before anything
// has armed is dropped, which is correct rather than merely tolerable: nothing
// has armed because nothing is waiting, and the notice the job left behind is
// taken by the next prompt exactly as it always was.
type jobWake struct {
	r *interp.Runner

	mu sync.Mutex
	// signal is nil until a session has asked for the descriptor, and nil
	// again once it has been taken down.
	signal *promptSignal
	closed bool
}

// fd is the descriptor to wait on, or negative where this session is not
// reporting a job the moment it ends — which is four dialects of five, and is
// also a zsh whose `NOTIFY` is off right now.
//
// The pipe is opened here, on the session's own goroutine, so that the poke
// never has to.
func (j *jobWake) fd() int {
	if j == nil || !j.r.NotifiesAsAJobEnds() {
		return -1
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return -1
	}
	if j.signal == nil {
		if j.signal = newPromptSignal(); j.signal == nil {
			// No pipe to be had. A session that cannot have one reports a
			// finished job at the next prompt, which is every session this
			// package had before this file.
			j.closed = true
			return -1
		}
	}
	return j.signal.fd()
}

// publish is what interp calls, from the goroutine the job ended on.
func (j *jobWake) publish() {
	j.mu.Lock()
	signal := j.signal
	j.mu.Unlock()
	signal.publish()
}

// drain takes the wake back off, on the session's goroutine.
func (j *jobWake) drain() {
	j.mu.Lock()
	signal := j.signal
	j.mu.Unlock()
	signal.drain()
}

// close ends the wake. Poking afterwards does nothing, which is what makes a
// job that outlives its session harmless rather than a write to a closed
// descriptor.
func (j *jobWake) close() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.closed = true
	j.signal.close()
	j.signal = nil
}

// jobNotifying wires this session's runner to a wake, and answers how to take
// the wiring down again.
//
// Nothing at all for a session with nobody to tell about a job, which is every
// route but the interactive one. Everything else is decided per read — see
// jobWake.
//
// Both halves here rather than at the call site, for the reason
// Shell.publishing gives: a Runner left pointing at a closed pipe is a bug
// that only appears in the second session a process runs, which for a library
// is the ordinary case.
func (s Shell) jobNotifying() (*jobWake, func()) {
	if s.Runner == nil || !s.Runner.JobControl {
		return nil, func() {}
	}
	wake := &jobWake{r: s.Runner}
	s.Runner.JobEnded = wake.publish
	return wake, func() {
		s.Runner.JobEnded = nil
		wake.close()
	}
}

// reportingFinishedJobs is what the editor calls when that wake fires: the
// same notice the next prompt would have written, written now.
//
// It answers whether anything was written, because the caller draws around it
// and must not draw around nothing. A wake and an empty report is the ordinary
// race and not an error — the job ended just as the prompt was being drawn,
// the prompt's own call to reportFinishedJobs took the line, and the byte in
// the pipe arrives at an editor with nothing left to say.
func (s Shell) reportingFinishedJobs() func() bool {
	return func() bool {
		if s.Runner == nil {
			return false
		}
		lines := s.Runner.FinishedJobNotices()
		for _, line := range lines {
			s.errf("%s\n", line)
		}
		return len(lines) > 0
	}
}

// reportJobs writes the notices and puts the prompt and the line being typed
// back underneath them.
//
// The editor's answer, and the shape is measured — see the file comment. The
// newline first, because the cursor is sitting wherever the typed line ended
// and a notice started there would run into it; the prompt's leading rows
// afterwards, because the redraw below rewrites only the row the line is on,
// which is the rule readLine is built on and the reason reprompt.go has to go
// up past them too.
func (e *editor) reportJobs(prompt drawnPrompt) {
	if e.jobNotices == nil {
		return
	}
	e.write(e.newline())
	if !e.jobNotices() {
		// Nothing to say after all, and nothing has been drawn over: the
		// newline above is the one byte this costs, and it is the same byte a
		// job's own output would have cost. Redrawing here would put a second
		// copy of the prompt on the screen for no reason.
		return
	}
	e.write(prompt.lead)
	// The line goes back under the new prompt from scratch. The incremental
	// repaint cannot account for a screen it did not write and does not try
	// to — the same handover reprompt makes, and for the same reason.
	e.row = 0
	e.redraw(prompt)
}

// awaitFinishedJobs waits until the terminal has a byte, writing a finished
// job's notice whenever one arrives first.
//
// serveDescriptors' loop with the drawing taken out, which is what a session
// whose **line editor is off** needs. There is nothing to draw again: the
// terminal is gathering the line itself and echoing it, so this shell has put
// nothing on the screen that a notice could land in the middle of. Measured on
// zsh 5.9.2 under `-fiV +Z` with `PS1=$'PR1\nzz> '`, the notice is written
// straight after the prompt, on the prompt's own row, and no new prompt
// follows it.
//
// It returns the moment a key is waiting, and that is the whole of its
// contract: the read after it is the read it always was. Two things make it
// return without waiting at all — input this editor already holds, which
// fdset.Wait cannot see because the bytes are past the kernel, and an input
// that is not a descriptor, where the poll below would spin rather than block.
func (e *editor) awaitFinishedJobs() {
	if e.jobWake == nil || e.jobNotices == nil || e.inFd == nil {
		return
	}
	if e.inputPending() {
		return
	}
	for {
		fd, terminal := e.jobWake(), e.inFd()
		if fd < 0 || terminal < 0 {
			return
		}
		ready, terminalReady, err := fdset.Wait(terminal, []int{fd})
		if err != nil || terminalReady || len(ready) == 0 {
			// A key is waiting, nothing is, or this system has no descriptor
			// set to ask. All three are the same answer here: stop waiting
			// and let the read happen.
			return
		}
		e.jobWoke()
		e.jobNotices()
	}
}
