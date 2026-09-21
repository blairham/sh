// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"sync"
)

// The asynchronous half of the prompt seam: a prompt drawn again because
// something it draws has *arrived*, rather than because somebody typed.
//
// # What this is not
//
// It is not a deadline on the synchronous seam, and promptprovider.go's
// refusal of one still stands unweakened. What is refused there is a fake
// deadline on a call the loop is waiting for — "a deadline reports something
// other than what happened, and abandoning the work leaks the goroutine doing
// it". Nothing here is waited for. A segment that has no answer yet draws
// what it has, which is usually nothing, and the prompt goes up without it;
// whoever is computing the answer owns its goroutine and its lifetime and is
// never abandoned, because it was never asked a question.
//
// So this adds a second kind beside the first rather than softening it. The
// two are told apart by which side speaks: a provider is *asked*, and a
// publisher *tells*.
//
// # One contract, two users
//
// docs/spec/prompt-theme.md settles this shape for a plugin segment — "it
// does not answer a request while the shell waits, it publishes, and the
// prompt draws with what it has and is redrawn when something new arrives" —
// and repository status needs exactly the same thing. Building one mechanism
// for the two is the point: a bespoke timeout for the prompt and a second for
// the plugin boundary would be two ways of being late.
//
// # Why a descriptor and not a channel
//
// Because of where the session waits. An editor between keystrokes is
// blocked in select(2) on the terminal and on whatever the shell armed — see
// watchfd.go — and a Go channel cannot wake that. A pipe can, so the wake is
// a pipe this package owns for the length of a session and hands a *function*
// out for. Nothing outside repl learns that a descriptor is involved, which
// is what keeps the theme engine free of the editor.

// PromptPublisher is a theme whose answer can change while a line is being
// typed.
//
// PublishTo is called once at the start of an interactive session with the
// function to call when what the theme would draw has changed, and once at
// the end with nil. Between those two calls the function is safe to call from
// any goroutine, as often as wanted: publishing twice before the prompt is
// redrawn costs one redraw, not two.
//
// Calling it is a statement that a *redraw would differ*, not a request for
// one. The editor decides — a prompt that renders the same is not written to
// the screen at all, which is what makes a publisher that cannot tell whether
// anything changed a cheap thing to be rather than a flickering one.
type PromptPublisher interface {
	PublishTo(publish func())
}

// promptSignal is the pipe a publisher's wake arrives on.
//
// At most one byte is ever in it. A publisher that fires a thousand times
// while a command runs must not fill a pipe nobody is reading, and the
// editor has nothing to do with the thousand that it would not do with the
// one: the answer is re-rendered from scratch either way.
type promptSignal struct {
	r, w *os.File

	mu sync.Mutex
	// pending is whether the byte is in the pipe and unread. It is what
	// bounds the pipe to one byte and what keeps drain from reading a pipe
	// with nothing in it.
	pending bool
	closed  bool
}

// newPromptSignal opens the wake, or answers nil where the system will not
// give a pipe.
//
// Nil is not an error worth reporting: a session that cannot have one is a
// session whose prompt is drawn once and not again, which is every session
// this package had before this file. The publisher is simply never wired.
func newPromptSignal() *promptSignal {
	r, w, err := os.Pipe()
	if err != nil {
		return nil
	}
	return &promptSignal{r: r, w: w}
}

// fd is the descriptor the editor waits on, or negative once it is closed.
func (p *promptSignal) fd() int {
	if p == nil {
		return -1
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return -1
	}
	return int(p.r.Fd())
}

// publish wakes the editor, from whatever goroutine computed the answer.
//
// It writes nothing when a wake is already waiting, which is the whole of
// what keeps a chatty publisher from filling the pipe — and it costs the
// publisher nothing to be chatty, because the editor re-renders the prompt
// rather than applying whatever was published.
func (p *promptSignal) publish() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.pending {
		return
	}
	p.pending = true
	if _, err := p.w.Write([]byte{0}); err != nil {
		// The far end has gone, which means the session has. Nothing is
		// waiting to be told and there is nobody to tell.
		p.pending = false
	}
}

// drain takes the wake off the pipe, on the editor's goroutine.
//
// Only where a wake is actually pending. select(2) reports a descriptor
// readable for reasons of its own — an end of file among them — and a read of
// a pipe with nothing in it would block the editor, which is the one thing a
// wake must never do.
func (p *promptSignal) drain() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.pending || p.closed {
		return
	}
	var buf [1]byte
	_, _ = p.r.Read(buf[:])
	p.pending = false
}

// close ends the session's wake. Publishing afterwards does nothing, which is
// what makes a publisher that outlives its session harmless rather than a
// write to a closed descriptor.
func (p *promptSignal) close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	_ = p.w.Close()
	_ = p.r.Close()
}

// publishing wires this session's theme to a wake, and answers how to take it
// down again.
//
// Both halves here rather than at the two call sites, because a publisher
// left pointing at a closed pipe is the shape of bug that only shows up in
// the second session a process runs — which for a shell is never, and for the
// library this package is meant to be is the ordinary case.
func (s Shell) publishing() (*promptSignal, func()) {
	publisher, ok := s.Theme.(PromptPublisher)
	if !ok {
		return nil, func() {}
	}
	signal := newPromptSignal()
	if signal == nil {
		return nil, func() {}
	}
	publisher.PublishTo(signal.publish)
	return signal, func() {
		publisher.PublishTo(nil)
		signal.close()
	}
}
