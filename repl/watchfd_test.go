// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"
)

// Waiting on something other than the terminal, through a real terminal.
//
// It has to be through one. The whole claim is about a *wait* — the editor
// sitting in a read with a descriptor in its other hand — and a reader-driven
// test cannot have one: a strings.Reader is always ready, so the wait never
// happens and a seam that is never called looks exactly like a seam that
// works. That is the rendered-surface trap this tree keeps finding, and here
// it would hide the whole feature.

// watcher is a descriptor armed for one of these tests, and what it recorded.
type descriptorProbe struct {
	mu     sync.Mutex
	fds    []int
	calls  []int
	line   Line
	answer func(Line) (Line, bool)

	// drain is read from when the callback fires, the way a real handler
	// reads what it was woken for. Without it a descriptor with something on
	// it stays readable and the callback is offered again immediately, which
	// is a spin the shell being modeled also has and which no test should
	// depend on the timing of.
	drain *os.File
}

func (p *descriptorProbe) watched() []int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]int(nil), p.fds...)
}

func (p *descriptorProbe) arm(fd int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fds = append(p.fds, fd)
}

func (p *descriptorProbe) disarm() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fds = nil
}

func (p *descriptorProbe) ready(_ context.Context, fd int, in Line) (Line, bool) {
	p.mu.Lock()
	p.calls = append(p.calls, fd)
	p.line = in
	answer, drain := p.answer, p.drain
	p.mu.Unlock()
	if drain != nil {
		var b [64]byte
		_, _ = drain.Read(b[:])
	}
	if answer == nil {
		return in, false
	}
	return answer(in)
}

func (p *descriptorProbe) called() []int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]int(nil), p.calls...)
}

func (p *descriptorProbe) sawLine() Line {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.line
}

// waitUntil is the wait these tests need: a condition rather than a mark on
// the screen, because what a descriptor callback does is not necessarily
// something drawn.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// probeSession is a terminal session with a descriptor probe wired into it.
func probeSession(t *testing.T, p *descriptorProbe) *session {
	t.Helper()
	return newSessionWith(t, func(s *Shell) {
		s.WatchedDescriptors = p.watched
		s.DescriptorReady = p.ready
	})
}

// TestADescriptorThatBecomesReadableCallsTheShell is the claim, at its
// smallest: with a pipe armed and the editor idle at a prompt, writing to the
// pipe reaches the front end — and it reaches it with the descriptor that woke.
func TestADescriptorThatBecomesReadableCallsTheShell(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = read.Close(); _ = write.Close() })

	p := &descriptorProbe{drain: read}
	s := probeSession(t, p)
	// Armed only once the session is up, so the arming cannot race the first
	// wait: an editor that had not started yet would be asked for nothing.
	s.prompt++
	waitFor(t, s.screen, "[1]", "the first prompt")
	fd := int(read.Fd())
	p.arm(fd)
	// A keystroke, so that the editor goes round the loop once with the
	// descriptor armed and reaches the wait holding it.
	if _, werr := s.control.WriteString("x"); werr != nil {
		t.Fatal(werr)
	}
	waitFor(t, s.screen, "x", "the keystroke")

	if _, werr := write.WriteString("wake\n"); werr != nil {
		t.Fatal(werr)
	}
	waitUntil(t, "the callback", func() bool { return len(p.called()) > 0 })
	if got := p.called()[0]; got != fd {
		t.Errorf("called with descriptor %d, want %d", got, fd)
	}
	// And it was handed the line as it stood, half-typed.
	if got := p.sawLine(); got.Buffer != "x" || got.Cursor != 1 {
		t.Errorf("line = %#v, want the half-typed x with the cursor after it", got)
	}
	p.disarm()
	// And the session is still a session: the half-typed line is killed and a
	// command typed in its place runs. Typed directly rather than through
	// typeLine, because no line has been accepted yet, so no second prompt
	// has been drawn to wait for.
	if _, werr := s.control.WriteString("\x15echo still-here\n"); werr != nil {
		t.Fatal(werr)
	}
	waitFor(t, s.ran, "still-here", "the command after the callback")
	s.end()
}

// TestAKeystrokeGetsPastAPermanentlyReadableDescriptor is the one that keeps a
// prompt usable, and it is not a hypothetical: measured, the shell being
// modeled calls a handler over and over on a pipe whose writer has gone and
// never removes it, so a session must be able to take input *through* that.
//
// An editor that served descriptors before looking at the terminal would sit
// in that loop for ever and never see a key.
func TestAKeystrokeGetsPastAPermanentlyReadableDescriptor(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = read.Close() })
	// Written and then closed: the read end is now at the end of its input,
	// which select reports as readable and always will.
	if _, werr := write.WriteString("gone\n"); werr != nil {
		t.Fatal(werr)
	}
	_ = write.Close()

	p := &descriptorProbe{}
	s := probeSession(t, p)
	s.prompt++
	waitFor(t, s.screen, "[1]", "the first prompt")
	p.arm(int(read.Fd()))
	// The callback never reads and never disarms, which is the worst-behaved
	// handler there is.
	if _, werr := s.control.WriteString("echo through\n"); werr != nil {
		t.Fatal(werr)
	}
	waitFor(t, s.ran, "through", "a command typed past a descriptor at end of input")
	if len(p.called()) == 0 {
		t.Error("the callback never ran at all, so this proved nothing about getting past it")
	}
	p.disarm()
	s.end()
}

// TestACallbackThatRewritesTheLineIsDrawn is the other answer from the seam:
// true means the line came back changed, and the editor draws it.
func TestACallbackThatRewritesTheLineIsDrawn(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = read.Close(); _ = write.Close() })

	// Once, and only once: the pipe is never drained, so it stays readable
	// and the callback goes on being offered the line. A callback that
	// rewrote it every time would leave the editor redrawing for ever, which
	// is the shell's own business to avoid and not something to test through.
	var once sync.Once
	p := &descriptorProbe{drain: read, answer: func(in Line) (Line, bool) {
		out, changed := in, false
		once.Do(func() { out, changed = Line{Buffer: "echo rewritten", Cursor: 14}, true })
		return out, changed
	}}
	s := probeSession(t, p)
	s.prompt++
	waitFor(t, s.screen, "[1]", "the first prompt")
	p.arm(int(read.Fd()))
	if _, werr := s.control.WriteString("q"); werr != nil {
		t.Fatal(werr)
	}
	waitFor(t, s.screen, "q", "the keystroke")
	if _, werr := write.WriteString("wake\n"); werr != nil {
		t.Fatal(werr)
	}
	waitFor(t, s.screen, "echo rewritten", "the redrawn line")
	p.disarm()
	// And it is really the line, not just something printed: accepting it runs
	// the command the callback put there.
	if _, werr := s.control.WriteString("\n"); werr != nil {
		t.Fatal(werr)
	}
	waitFor(t, s.ran, "rewritten", "the command the callback left on the line")
	s.end()
}

// TestAHandlerThatDisarmsItselfIsCalledOnce is what a real handler does, and
// it is the reason the list is asked again on every round rather than once:
// the descriptor here stays readable for ever — nothing drains it — so a loop
// that had collected the list before it started would go on calling a callback
// that had already taken itself off.
func TestAHandlerThatDisarmsItselfIsCalledOnce(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = read.Close(); _ = write.Close() })
	if _, werr := write.WriteString("wake\n"); werr != nil {
		t.Fatal(werr)
	}

	p := &descriptorProbe{}
	p.answer = func(in Line) (Line, bool) {
		p.disarm()
		return in, false
	}
	s := probeSession(t, p)
	s.prompt++
	waitFor(t, s.screen, "[1]", "the first prompt")
	p.arm(int(read.Fd()))
	// A keystroke to send the editor round the loop with it armed.
	if _, werr := s.control.WriteString("z"); werr != nil {
		t.Fatal(werr)
	}
	waitUntil(t, "the callback", func() bool { return len(p.called()) > 0 })
	// The session still runs commands, which is what proves the loop left.
	if _, werr := s.control.WriteString("\x15echo after-disarm\n"); werr != nil {
		t.Fatal(werr)
	}
	waitFor(t, s.ran, "after-disarm", "a command after the callback disarmed itself")
	if got := p.called(); len(got) != 1 {
		t.Errorf("called %d times, want once — it took itself off on the first call", len(got))
	}
	s.end()
}

// TestOneDeadDescriptorDoesNotCostTheLiveOneItsTurn. Waiting is a question
// about a *set*, and the kernel refuses the whole call for one bad member. So
// a descriptor that was closed out from under a watcher — which the shell
// being modeled keeps listing and never fires — must be dropped before the
// question is asked, or a plugin's second watcher silently stops working
// because its first one's descriptor was closed.
func TestOneDeadDescriptorDoesNotCostTheLiveOneItsTurn(t *testing.T) {
	// The live pipe first and the dead one after it, so that closing the dead
	// one frees a number the live one is not sitting on: a test in which the
	// two are the same descriptor proves nothing.
	read, write, perr := os.Pipe()
	if perr != nil {
		t.Fatal(perr)
	}
	t.Cleanup(func() { _ = read.Close(); _ = write.Close() })
	dead, deadWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	deadFd := int(dead.Fd())
	_ = dead.Close()
	_ = deadWrite.Close()
	if live := int(read.Fd()); deadFd == live {
		t.Fatalf("the dead and the live descriptor are both %d", live)
	}

	p := &descriptorProbe{drain: read}
	s := probeSession(t, p)
	s.prompt++
	waitFor(t, s.screen, "[1]", "the first prompt")
	// The dead one first, so it is the one the kernel would complain about.
	p.arm(deadFd)
	live := int(read.Fd())
	p.arm(live)
	if _, werr := s.control.WriteString("y"); werr != nil {
		t.Fatal(werr)
	}
	waitFor(t, s.screen, "y", "the keystroke")
	if _, werr := write.WriteString("wake\n"); werr != nil {
		t.Fatal(werr)
	}
	waitUntil(t, "the live descriptor's callback", func() bool {
		for _, fd := range p.called() {
			if fd == live {
				return true
			}
		}
		return false
	})
	p.disarm()
	if _, werr := s.control.WriteString("\x15echo both-ok\n"); werr != nil {
		t.Fatal(werr)
	}
	waitFor(t, s.ran, "both-ok", "the command after the callback")
	s.end()
}

// TestASessionWithNothingArmedIsUnchanged. The property worth stating plainly,
// because it is what makes this safe to have at all: with no descriptor armed
// the editor never waits on anything but the terminal, and a session that has
// never armed one is asked no question it was not asked before.
func TestASessionWithNothingArmedIsUnchanged(t *testing.T) {
	p := &descriptorProbe{}
	s := probeSession(t, p)
	s.typeLine("echo untouched\n")
	waitFor(t, s.ran, "untouched", "the command's output")
	if got := p.called(); len(got) != 0 {
		t.Errorf("the callback ran %v times with nothing armed", got)
	}
	s.end()
}
