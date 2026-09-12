// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"io"
	"sync"

	"github.com/blairham/sh/internal/fdset"
	"github.com/blairham/sh/interp"
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
//
//     Re-measured 2026-09-08, and the reason is not the one that reading the
//     output had suggested: that shell does not hand the line discipline back
//     for a handler at all. Its editor's raw mode *keeps* OPOST and ONLCR —
//     asked from the far end of a pseudo-terminal, an idle prompt there is
//     `ECHOE ISIG IEXTEN`, `OPOST ONLCR`, `BRKINT INLCR ICRNL IXON`, with
//     only ICANON and ECHO dropped — so the kernel translates a handler's
//     newline with nothing restored first. Ours drops OPOST too, which is
//     what leaves this package no way to reach column 0 except to restore.
//     What that costs is two bullets down.
//   - **Nothing is redrawn afterwards.** A line half-typed when a handler
//     fired is left sitting behind the handler's output. That is why the
//     answer from the seam decides the redraw rather than the redraw being
//     unconditional the way runShellWidget's is: the shell's *plain* callback
//     draws nothing, and only the spelling that asks for the line back
//     redraws.
//   - **A descriptor at the end of its input is readable for ever, and the
//     shell being modeled spins on one.** Watching the read end of a pipe
//     whose writer had exited, zsh calls the handler and never removes it —
//     removing itself is the handler's job. So this loop must let a keystroke
//     through a descriptor that is always ready, which is what fdset.Wait
//     answering about the terminal separately is for. A shell doing that to
//     itself is the shell's business; a prompt that stopped taking input
//     would be ours.
//
//     *How fast* it spins was written here as "hundreds of times in two
//     seconds", and that is wrong by three orders of magnitude. Re-measured
//     2026-09-08 with a handler that only counts its calls, differencing a
//     two-second idle against a six-second one so that the typing either side
//     falls out: **250,000 to 270,000 calls a second**, and a whole core held
//     for as long as the descriptor is armed — 1.00 core sustained against
//     0.00 with nothing armed. The old figure came from a handler that
//     *printed* into a pseudo-terminal nobody was reading: in that
//     arrangement the shell spends 0.00s of CPU in two seconds, blocked on
//     the write, and what was being counted was the pipe filling rather than
//     the loop running. The instrument, not the shell.
//
//   - **What is ours is the line discipline underneath that spin, not the
//     spin.** Sampling the terminal's attributes from the far end of the
//     pseudo-terminal while a watcher of that shape is armed: that shell is
//     in raw mode 100% of the time — ICANON set in 0 of 6.9 million samples —
//     and this editor is in the terminal's *own* discipline 80% of an idle
//     prompt, because the handler runs inside the window inLineDiscipline
//     opens and the offers come tens of thousands of times a second. With
//     nothing armed both are 0%, so the watcher is the whole of it. That is
//     what makes a control key typed at such a prompt a coin toss rather than
//     a narrow race, and it is what lazyDiscipline below closes: the terminal
//     is handed back at a handler's *first byte* and not before, so a handler
//     that prints nothing — which is the one that spins — never opens the
//     window at all. Measured the same way afterwards, ICANON is set in 0 of
//     the samples taken while such a watcher is armed, which is what that
//     shell reads.
//
//     **Not a rate**, and that is the other half of the answer. A rate would
//     have been a divergence: the shell being modeled offers the descriptor
//     four times as often as this loop does and holds the core while it is
//     armed, so there is no slower loop to converge on. What was ours was
//     never the frequency; it was the flap underneath it.
//
//     **Nor its raw mode**, which was the other way to close it and is a
//     bigger change than it looks. Keeping OPOST and ONLCR would end the flap
//     for every handler at once, but that shell's zle mode also keeps ISIG,
//     and ISIG turns the `^C` this editor reads as a byte into a signal —
//     which is behavior this package measures and pins elsewhere. The narrow
//     fix is the one that touches only the descriptor path.

// descriptorHandlers is how a session answers a descriptor that has become
// readable, with this session's context and terminal closed over.
//
// nil where the front end offered no way to answer one, which is what makes
// the editor's loop skip the waiting entirely — see serveDescriptors.
func (s Shell) descriptorHandlers(
	ctx context.Context, state *terminalState,
) func(int, Line, Actions) (Line, bool) {
	if s.DescriptorReady == nil {
		return nil
	}
	guard := s.guard()
	return func(fd int, in Line, ed Actions) (Line, bool) {
		var out Line
		var ok bool
		// In raw mode until it writes something — see runHandler, and the
		// last bullet of the file comment for what the eager restore this
		// replaced cost. Behind the guard a typed line, a hook and a key
		// bound to a shell action all run behind: the stronger version of
		// shellwidget.go's reason applies here, because this code runs
		// because a *descriptor* woke, at a moment nobody chose, so a panic
		// in it would end a session over something the person at the keyboard
		// did not do.
		s.runHandler(state, func() {
			if guard.Do(func() { out, ok = s.DescriptorReady(WithActions(ctx, ed), fd, in) }) {
				out, ok = Line{}, false
			}
		})
		if ok {
			// The line is coming back and the editor is about to draw it, so
			// whatever the handler printed has to have arrived first. Under a
			// block store that is not the same event as the handler
			// returning — see drained, and #1356, which is this ordering at
			// the boundary between a command and the next prompt. Only where
			// the line comes back, because that is the only answer this
			// editor draws anything for, and a handler that spins prints
			// nothing and asks for nothing.
			s.drained()
		}
		return out, ok
	}
}

// runHandler runs one descriptor handler with the terminal in whichever
// discipline that handler's output needs, and no other.
//
// Three answers rather than one, because "what does this handler's output
// need" has three of them and only the first is new:
//
//   - **Nothing, where something else is already translating.** Under a block
//     store the Runner's streams are the far end of a pseudo-terminal whose
//     pump asks the real terminal, per write, whether it is still adding the
//     carriage returns and adds them itself where it is not — see conduitOut,
//     which does that for a background job printing at a prompt and needs no
//     help from this. The handler runs in raw mode from beginning to end, and
//     a child it starts keeps a real terminal on its output, which is the
//     price lockWriter refuses to pay in interp for the same reason.
//   - **The restore, at the first byte and not before.** With the streams
//     going straight to the terminal, only the kernel can translate, so the
//     terminal has to be handed back — but not until there is something to
//     write. That is lazyDiscipline.
//   - **The restore, for the whole call**, where there are no streams to
//     watch. A Shell with no Runner cannot have a write noticed, and guessing
//     that a handler will print nothing is not something this can do on a
//     caller's behalf; it gets what every handler got before the flap was
//     measured.
//
// # What else a handler now finds, and why it is the right way round
//
// Output is what the restore was for, but the discipline is not only about
// output: in raw mode ISIG is off, so a `^C` typed while a handler is running
// is a byte the editor reads rather than a signal, and ICANON and ECHO are off,
// so a handler that reads the *terminal* is handed bytes as they are typed
// rather than a line. Both of those move toward the shell being modeled rather
// than away from it — measured, it never leaves raw mode for a handler at all,
// so what a handler finds there is exactly this.
//
// And the direction is what makes it a fix rather than a trade. The window
// this closes is one a person types into: the `^C` that used to become a
// signal, and the `^D` a line discipline stores as a NUL, were bytes meant for
// the editor that the handler's restore took away — four fifths of an idle
// prompt of them, which is #1448 and #1463.
func (s Shell) runHandler(state *terminalState, f func()) {
	switch {
	case state == nil, s.translatesOutput():
		f()
	case s.Runner == nil:
		s.inLineDiscipline(state, f)
	default:
		l := &lazyDiscipline{s: s, state: state, r: s.Runner}
		defer l.done()
		l.watch()
		f()
	}
}

// translatesOutput reports whether something other than the terminal's own
// line discipline is putting the carriage returns back into what the Runner
// writes.
//
// One question with one answer for the whole session: captureOutput points
// *both* of the Runner's streams at the conduit or neither of them, so there
// is no half of this to get right per stream.
func (s Shell) translatesOutput() bool {
	return s.capture != nil && s.capture.conduit != nil
}

// A lazyDiscipline is the terminal handed back at a handler's first byte.
//
// **The write is what is watched, because the write is what the restore is
// for.** Raw mode has OPOST off, so a newline written under it moves down
// without returning the carriage; a handler that writes nothing cannot be
// harmed by that, and the measurement in the file comment is what a handler
// that writes nothing was costing — four fifths of an idle prompt spent in
// the terminal's own line discipline, where a `^D` is stored by the line
// discipline as a NUL and a `^C` is a signal rather than a byte the editor
// reads.
//
// # Why the streams and not the terminal
//
// There is nowhere else to notice it. What a handler prints goes through the
// Runner's streams, which this package already owns the wiring of —
// captureOutput points them at a conduit at the start of a session — and the
// kernel offers no way to ask a terminal whether anything has been written to
// it. So the streams are wrapped for the length of one call and the wrapper is
// what notices.
//
// # One call, and the cost of that
//
// interp hands a child `r.Stdout` directly: an *os.File is inherited as a
// descriptor and anything else makes os/exec build a pipe, so a wrapped
// stream costs a child its terminal — `test -t 1` says no, `ls` stops
// colorizing. That price is written down twice already, in lockWriter and in
// blocks.CaptureMode, and neither pays it for a whole session. Nor does this:
// the wrapper goes on for one handler call and comes off at the end of it.
//
// **Which means an external command run by a handler is on a pipe for that
// call**, and that is the whole of what this costs. It is stated rather than
// hidden, and it is narrower than it reads: the *default* session takes the
// first of runHandler's three answers and is never wrapped at all, so this is
// the price of a session that turned the block store off. The command's
// output still reaches the terminal with the translation on, because the copy
// os/exec makes goes through this wrapper and hands the terminal back on its
// way past.
//
// **Taking the wrapper off at the first byte instead would be a data race**,
// which is why it is not done, and the reason is worth writing down because
// the shape of it is invisible at the call site. The first byte does not
// always arrive on the editor's goroutine: os/exec copies a child's output on
// one of its own, so a write that came from a child would be putting the
// Runner's stream back from a goroutine the Runner does not belong to, while
// the editor is reading it. The Runner's streams are the editor goroutine's
// to write — captureOutput writes them once, at the start — and this keeps
// that true: watch and done are both on it.
//
// # A handler may outlive its own streams
//
// A job the handler starts with `&` carries the wrapper away in its clone of
// the Runner and goes on writing through it after the call has ended, on its
// own goroutine. So the wrapper expires rather than merely being taken off,
// and a write that arrives afterwards is passed through untouched: a
// background job must not be able to take the terminal away from an editor
// that is drawing a line. The mutex is for that goroutine and for the child's
// copier — the handler itself runs on the editor's.
type lazyDiscipline struct {
	s     Shell
	state *terminalState
	r     *interp.Runner

	// out and err are the Runner's streams as they were, and what the
	// wrappers write through.
	out, err io.Writer

	mu sync.Mutex
	// handed is whether the terminal has been given back, so it is given back
	// once and taken back once.
	handed bool
	// expired is whether this call has ended, after which a write is somebody
	// else's — see the last section of the type comment.
	expired bool
}

// watch puts the wrappers on.
//
// A nil stream is left nil: nil means *empty* to interp — a writer that
// discards, and /dev/null to a child — so nothing written to one can reach a
// terminal and there is nothing there to notice.
func (l *lazyDiscipline) watch() {
	l.out, l.err = l.r.Stdout, l.r.Stderr
	if l.out != nil {
		l.r.Stdout = &lazyStream{l: l, w: l.out}
	}
	if l.err != nil {
		l.r.Stderr = &lazyStream{l: l, w: l.err}
	}
}

// hand gives the terminal back, once, before the byte that asked for it is
// written.
//
// It can be called from a goroutine that is not the editor's — a child's
// output is copied on one of os/exec's — which is what the lock is for and
// what keeps the Runner's streams out of it; see the type comment.
func (l *lazyDiscipline) hand() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.handed || l.expired {
		return
	}
	l.handed = true
	if err := l.state.restore(); err != nil {
		l.s.errf("%v\n", err)
	}
}

// done ends the call: the wrappers come off, and raw mode comes back if the
// handler took the terminal.
//
// **Nothing is waited for first, and that is a statement rather than an
// omission.** inLineDiscipline waits for the conduit before it takes raw mode
// back, because a command's last bytes may still be in flight when the command
// returns and raw mode landing between the two sends the tail out with the
// newline translation already gone. Here there is nothing in flight to wait
// for: the only thing that stands between a handler and the terminal is that
// conduit, and a session that has one never reaches this branch at all — it
// takes the first of runHandler's three, where the terminal is never handed
// back. A drain here would be a call that cannot do anything, which is worse
// than none, because it reads as a wait something is relying on.
func (l *lazyDiscipline) done() {
	l.mu.Lock()
	l.expired = true
	handed := l.handed
	l.unwrap()
	l.mu.Unlock()
	if !handed {
		return
	}
	if _, err := makeRaw(l.s.inFile()); err != nil {
		l.s.errf("%v\n", err)
	}
}

// unwrap puts the Runner's streams back, and only where a wrapper is still
// what is there.
//
// A handler that redirected the shell's own output — `exec >somewhere` — has
// already replaced them, and putting back what was there before would undo
// that on the way out of a handler nobody was watching. Asked of the stream
// rather than remembered, because the shell's answer is the one in the
// Runner and this call's bookkeeping cannot have heard about it.
//
// A wrapper found here is always this call's: one goes on per call, comes off
// at the end of the same call, and a handler cannot be running inside a
// handler.
func (l *lazyDiscipline) unwrap() {
	if _, ok := l.r.Stdout.(*lazyStream); ok {
		l.r.Stdout = l.out
	}
	if _, ok := l.r.Stderr.(*lazyStream); ok {
		l.r.Stderr = l.err
	}
}

// A lazyStream is one of the Runner's streams while a handler is running.
type lazyStream struct {
	l *lazyDiscipline
	w io.Writer
}

func (w *lazyStream) Write(p []byte) (int, error) {
	w.l.hand()
	return w.w.Write(p)
}

// Unwrap is the stream underneath, so that this wrapper does not hide what it
// is over — [interp.WriterUnder], and the reason is #2069. A background job
// started inside a handler asks the shell's stream whether it is already
// guarded, and the answer is about the chain: without this the question stops
// here, a second guard goes on over the first, and the next write takes one
// mutex twice on one goroutine.
func (w *lazyStream) Unwrap() io.Writer { return w.w }

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
	if e.inputPending() {
		// Input this editor has already taken off the terminal is input
		// waiting, and the wait below cannot see it: fdset.Wait asks the
		// *kernel* whether the terminal has a byte, and the bytes of a paste
		// are in e.held by then. Without this a descriptor that is always
		// readable — the case this file exists for — is served for ever while
		// a whole line sits unread in hand, which is the very "never sees a
		// key" failure the descriptor is meant to be gotten past.
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
	out, changed := e.descriptorReady(fd, e.give(), editorActions{e: e, prompt: prompt})
	if !changed {
		return
	}
	e.line = []rune(out.Buffer)
	e.pos = min(max(out.Cursor, 0), len(e.line))
	e.redraw(prompt)
}
