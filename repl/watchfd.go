// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"

	"github.com/blairham/sh/internal/fdset"
)

// Waiting on something other than the terminal.
//
// shellwidget.go describes the round trip this editor already had — the line
// goes out, something the shell owns runs, the line comes back — and ends by
// naming the seam it does *not* offer: an action that is started and finishes
// later, a callback on a descriptor, which "needs this loop to wait on more
// than the terminal, which is a change to how a key is read rather than an
// addition beside it". This file is that change, and it turned out to be
// smaller than the sentence promised, because the round trip was already the
// right shape. All that is new is *when* it happens.
//
// **What this package contributes is the waiting, and that is the whole of
// it.** A shell that can hand out a descriptor and be told when it is readable
// needs somewhere that is already idle with a descriptor in its hand, and an
// editor between keystrokes is the only place in a shell that has one: a
// script never waits, and a foreground command holds the terminal itself.
// What a descriptor *means*, which function is called for it, what it is
// called with and what the shell calls the command that armed it are the
// dialect's — see dialect/zsh/zlewatch.go, where all four have that shell's
// names on them and nothing here knows any of them.
//
// Measured 2026-09-07 against zsh 5.9.2 through a pseudo-terminal, and four
// facts out of it shaped this file rather than the dialect's:
//
//   - **The read loop is the only place it happens.** With a six-second
//     command running and the descriptor becoming readable three seconds in,
//     the handler ran *after* the command finished — bracketing markers
//     printed either side of the command came out before it. So a watcher is
//     looked at while the editor waits for a key and at no other time, which
//     is exactly the moment this file has to offer and nothing more.
//   - **A handler's output is written in the terminal's own line
//     discipline.** Two lines printed from one land at column 0 apiece,
//     carriage returns and all, which raw mode with OPOST off would not give.
//     So a handler runs where a hook and a scheduled command run, through
//     inLineDiscipline, and for the same reason.
//   - **Nothing is redrawn afterwards.** A line half-typed when a handler
//     fired is left sitting behind the handler's output. That is why the
//     answer from the seam decides the redraw rather than the redraw being
//     unconditional the way runShellWidget's is: the shell's *plain* callback
//     draws nothing, and only the spelling that asks for the line back
//     redraws.
//   - **A descriptor at the end of its input is readable for ever.** Watching
//     the read end of a pipe whose writer had exited, zsh called the handler
//     hundreds of times in two seconds and never removed it — removing itself
//     is the handler's job. So this loop must let a keystroke through a
//     descriptor that is always ready, which is what fdset.Wait answering
//     about the terminal separately is for. A shell doing that to itself is
//     the shell's business; a prompt that stopped taking input would be ours.

// descriptorHandlers is how a session answers a descriptor that has become
// readable, with this session's context and terminal closed over.
//
// nil where the front end offered no way to answer one, which is what makes
// the editor's loop skip the waiting entirely — see serveDescriptors.
func (s Shell) descriptorHandlers(ctx context.Context, state *terminalState) func(int, Line) (Line, bool) {
	if s.DescriptorReady == nil {
		return nil
	}
	guard := s.guard()
	return func(fd int, in Line) (Line, bool) {
		var out Line
		var ok bool
		// In the terminal's own line discipline, measured — and behind the
		// guard a typed line, a hook and a key bound to a shell action all
		// run behind. The stronger version of shellwidget.go's reason applies
		// here: this code runs because a *descriptor* woke, at a moment
		// nobody chose, so a panic in it would end a session over something
		// the person at the keyboard did not do.
		s.inLineDiscipline(state, func() {
			if guard.Do(func() { out, ok = s.DescriptorReady(ctx, fd, in) }) {
				out, ok = Line{}, false
			}
		})
		return out, ok
	}
}

// watchedDescriptors is which descriptors the shell wants waited on, asked
// fresh, or nil where the front end has none to offer.
//
// Asked fresh for the reason KeyBindings is: the table is state, and the
// command that changes it is one a startup file runs and one a handler runs —
// a handler that armed the next descriptor before returning is the ordinary
// case, not an unusual one, so a list collected once would be the list the
// session started with.
func (s Shell) watchedDescriptors() func() []int {
	if s.WatchedDescriptors == nil {
		return nil
	}
	return s.WatchedDescriptors
}

// serveDescriptors waits until the terminal has a byte, running the shell's
// handler for every watched descriptor that becomes readable first.
//
// It returns having done nothing at all in a session with no watcher armed,
// which is every session in three of the four dialects and every session in
// the fourth until a startup file arms one. That is the property worth stating
// plainly: with nothing to watch, a keystroke is read exactly the way it was
// read before this file existed, with no extra call in front of it.
func (e *editor) serveDescriptors(prompt drawnPrompt) {
	if e.watch == nil || e.descriptorReady == nil {
		return
	}
	for {
		fds := e.watch()
		if len(fds) == 0 {
			return
		}
		terminal := -1
		if e.inFd != nil {
			terminal = e.inFd()
		}
		ready, terminalReady, err := fdset.Wait(terminal, fds)
		if err != nil {
			// Nothing here can be waited on. The keystroke is read the way it
			// always was, and a watcher on this system is armed and never
			// fires — which the dialect that arms one has to say for itself,
			// because this package cannot tell a caller anything from inside
			// a read.
			return
		}
		if terminalReady || len(ready) == 0 {
			// A key is waiting, or nothing is. Either way the read comes
			// next: a descriptor that is ready as well will still be ready
			// after the keystroke has been dealt with, and letting the key
			// go first is what keeps a prompt usable when a descriptor is
			// permanently readable. See the file comment.
			return
		}
		for _, fd := range ready {
			e.serveDescriptor(fd, prompt)
		}
	}
}

// serveDescriptor runs the handler for one descriptor and draws whatever it
// left behind, which is usually nothing.
//
// The redraw is the shell's answer and not this editor's guess, which is where
// this differs from runShellWidget: measured, the plain spelling of a
// descriptor callback prints where the cursor was and leaves the line alone,
// and only the spelling that asked for the line back moves it. So false means
// *the line is untouched and the screen is the shell's problem* rather than
// meaning the shell declined.
func (e *editor) serveDescriptor(fd int, prompt drawnPrompt) {
	out, changed := e.descriptorReady(fd, Line{Buffer: string(e.line), Cursor: e.pos})
	if !changed {
		return
	}
	e.line = []rune(out.Buffer)
	e.pos = min(max(out.Cursor, 0), len(e.line))
	e.redraw(prompt)
}
