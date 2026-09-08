// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package repl

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blairham/sh/internal/blocks"
	"github.com/blairham/sh/interp"
)

// Which line discipline a descriptor handler runs in, asked of the terminal.
//
// The claim these hold is watchfd.go's last measurement: the shell being
// modeled never hands the line discipline back for a handler, and this editor
// was handing it back for four fifths of an idle prompt because the handler
// ran inside the window inLineDiscipline opens. A handler that prints nothing
// — which is the one that spins, and the one a plugin leaves armed — has
// nothing to hand it back for.
//
// Asked of the terminal rather than of the bytes, for the reason
// hooksterminal_test.go gives: the mode is the state the *next* thing typed
// will find, and what that costs does not show up in anything printed.

// spinning arms a descriptor that is readable for ever and reports what the
// session's terminal looked like while the handler was being offered it.
//
// cooked is how many samples found the terminal in its own line discipline,
// out of samples taken; calls is how many times the handler ran while the
// sampling was happening, which is what makes a count of zero evidence rather
// than a test that watched nothing.
func spinning(t *testing.T, s *session, p *descriptorProbe, d time.Duration) (cooked, samples, calls int) {
	t.Helper()
	before := len(p.called())
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		samples++
		// ECHO rather than ICANON, and they are the same question: makeRaw
		// clears both and restore puts both back, so one ioctl answers for
		// the pair — and echoes is the reading hooksterminal_test.go already
		// has. A second helper reading the second flag would be a second
		// thing to keep right about one fact.
		if echoes(t, s.tty) {
			cooked++
		}
	}
	return cooked, samples, len(p.called()) - before
}

// armedForever is a descriptor at the end of its input, which select reports
// as readable and always will — the shape a plugin leaves behind and the one
// the shell being modeled spins on.
func armedForever(t *testing.T) *os.File {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = read.Close() })
	if _, werr := write.WriteString("gone\n"); werr != nil {
		t.Fatal(werr)
	}
	_ = write.Close()
	return read
}

// TestAHandlerThatPrintsNothingNeverTakesTheTerminalBack is the fix for #1463,
// measured the way the bug was.
//
// Before it, this sampling found the terminal in its own line discipline for
// **52%** of the samples, because every one of tens of thousands of handler
// calls a second restored it and took it away again — and for **96%** in the
// session TestUnderABlockStore builds, where the restore also waits for the
// conduit before taking raw mode back. Both on Linux with the machine at a
// load average of 0.5, which is the direction that matters: a busy machine
// makes this look better than it is.
//
// A `^D` typed into one of those windows is stored by the line discipline as
// a NUL and the `\x04` never arrives — reproduced directly on Linux, where a
// `^C` in the same window becomes a signal and an ordinary character survives
// intact. That is #1448's corruption, and it is what this closes.
//
// **Two claims in one spin, because they are one measurement.** The handler
// writes nothing through the stream it is *given*, which is the fix; and what
// it does write goes through the stream a previous call was given, which is
// what a job started inside a handler carries away with it. Neither may hand
// the terminal back, and one sampling answers for both.
func TestAHandlerThatPrintsNothingNeverTakesTheTerminalBack(t *testing.T) {
	read := armedForever(t)

	// Nothing to drain and nothing to disarm: the worst-behaved handler there
	// is, and the one that spins.
	p := &descriptorProbe{}
	var runner *interp.Runner
	s := newSessionWith(t, func(cfg *Shell) {
		runner = cfg.Runner
		cfg.WatchedDescriptors = p.watched
		cfg.DescriptorReady = p.ready
	})
	// And what a job started inside a handler carries away with it. The first
	// call only takes the stream — it writes nothing, so the terminal is never
	// handed back for it — and every call after that writes through *that*
	// call's stream, which is the wrapper of a call that has long since ended.
	// A background job printing at a prompt is exactly that write, and it must
	// reach the stream and leave the terminal alone: an editor drawing a line
	// cannot have the discipline taken out from under it by a job.
	var stale io.Writer
	p.answer = func(in Line) (Line, bool) {
		if stale == nil {
			stale = runner.Stdout
			return in, false
		}
		_, _ = io.WriteString(stale, ".")
		return in, false
	}
	s.prompt++
	waitFor(t, s.screen, "[1]", "the first prompt")
	p.arm(int(read.Fd()))
	// A keystroke, so the editor goes round the loop once with the descriptor
	// armed and reaches the wait holding it.
	if _, err := s.control.WriteString("x"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, s.screen, "x", "the keystroke")
	waitUntil(t, "the handler", func() bool { return len(p.called()) > 0 })

	cooked, samples, calls := spinning(t, s, p, 300*time.Millisecond)
	if calls == 0 {
		t.Fatal("the handler was not being called while the terminal was sampled, " +
			"so nothing was measured")
	}
	if cooked != 0 {
		t.Errorf("the terminal was in its own line discipline for %d of %d samples "+
			"(%.1f%%) across %d handler calls, where it should never have left raw mode",
			cooked, samples, 100*float64(cooked)/float64(samples), calls)
	}

	p.disarm()
	settled(t, p)
	// And the session is still a session: the half-typed line is killed and a
	// command typed in its place runs.
	if _, err := s.control.WriteString("\x15echo still-here\n"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, s.ran, "still-here", "a command typed after the spin")
	s.end()

	// Asked after the session has ended, which is what makes it safe to ask:
	// the streams are the editor goroutine's to write and s.end waits for it.
	//
	// The stream is the session's own again. A wrapper left behind by a
	// handler that printed nothing would be there for the rest of the
	// session, and every child after it would be on a pipe — the whole reason
	// it goes on for one call and comes off again.
	if _, wrapped := runner.Stdout.(*lazyStream); wrapped {
		t.Error("the Runner's stream was left wrapped by a handler that printed nothing")
	}
}

// TestAHandlerThatPrintsIsGivenTheTerminalFirst is the other half, and it is
// what stops the fix above from being a regression: the restore is *late*
// rather than gone. A handler that writes something has to write it with the
// terminal translating newlines, or its output lands wherever the last thing
// drawn ended.
//
// The mode is read from the terminal at the moment of each write, by the
// destination itself, so this cannot pass by believing its own bookkeeping —
// the same instrument TestNothingIsStillOnItsWayWhenRawModeComesBack uses.
//
// **Both of the shell's streams, in sessions of their own.** A handler's
// diagnostic is exactly the thing that lands bare, and a single case that
// wrote to standard output first would answer for standard error without ever
// asking it: the terminal is already back by the time the second write
// happens. Only a handler that writes to one of them tells the two apart.
func TestAHandlerThatPrintsIsGivenTheTerminalFirst(t *testing.T) {
	for _, c := range []struct {
		name  string
		text  string
		which func(*interp.Runner) *io.Writer
	}{
		{"standard output", "from-the-handler", func(r *interp.Runner) *io.Writer { return &r.Stdout }},
		{"standard error", "the-handlers-complaint", func(r *interp.Runner) *io.Writer { return &r.Stderr }},
	} {
		t.Run(c.name, func(t *testing.T) {
			read, write, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = read.Close(); _ = write.Close() })
			if _, werr := write.WriteString("wake\n"); werr != nil {
				t.Fatal(werr)
			}

			// Open from the start: this instrument's gate is for a test that
			// has to hold the output back, and here the writes go straight
			// through.
			open := make(chan struct{})
			close(open)
			var recorded *modeRecordingWriter
			var runner *interp.Runner

			p := &descriptorProbe{}
			s := newSessionWith(t, func(cfg *Shell) {
				tty, ok := cfg.In.(*os.File)
				if !ok {
					t.Fatal("the session's input is not a terminal")
				}
				stream := c.which(cfg.Runner)
				recorded = &modeRecordingWriter{w: *stream, fd: int(tty.Fd()), gate: open}
				*stream = recorded
				runner = cfg.Runner
				cfg.WatchedDescriptors = p.watched
				cfg.DescriptorReady = p.ready
			})
			// Once, and then it takes itself off, which is what a real
			// handler does.
			var once sync.Once
			p.answer = func(in Line) (Line, bool) {
				once.Do(func() {
					// Through the stream as the Runner holds it *now*, which
					// is what a builtin does — r.stdout() is read per write.
					_, _ = fmt.Fprintln(*c.which(runner), c.text)
					p.disarm()
				})
				return in, false
			}

			s.prompt++
			waitFor(t, s.screen, "[1]", "the first prompt")
			p.arm(int(read.Fd()))
			if _, werr := s.control.WriteString("z"); werr != nil {
				t.Fatal(werr)
			}
			waitUntil(t, "the handler", func() bool { return len(p.called()) > 0 })
			waitUntil(t, "what the handler printed", func() bool {
				_, _, text := recorded.seen()
				return strings.Contains(text, c.text)
			})
			settled(t, p)

			writes, translated, text := recorded.seen()
			if writes == 0 {
				t.Fatal("nothing reached the terminal, so nothing was measured")
			}
			if translated != writes {
				t.Errorf("%d of %d writes left the handler with the newline translation off, "+
					"so the terminal was still in raw mode when it printed", writes-translated, writes)
			}
			if !strings.Contains(text, c.text) {
				t.Errorf("the stream carried %q rather than what the handler printed", text)
			}
			// And the terminal is back in raw mode for the next keystroke.
			if echoes(t, s.tty) {
				t.Error("the terminal was left in its own line discipline after the handler")
			}

			if _, werr := s.control.WriteString("\x15echo after-print\n"); werr != nil {
				t.Fatal(werr)
			}
			waitFor(t, s.ran, "after-print", "a command after the handler printed")
			s.end()

			// Asked after the session has ended, which is what makes it safe
			// to ask: the streams are the editor goroutine's to write, and
			// s.end waits for it. A wrapper left behind would be there for
			// the rest of the session, and every child after it would be on a
			// pipe rather than on a terminal.
			if _, wrapped := (*c.which(runner)).(*lazyStream); wrapped {
				t.Error("the Runner's stream was left wrapped after the handler returned")
			}
		})
	}
}

// TestUnderABlockStoreAHandlerKeepsBothTheStreamAndRawMode is the default
// session, which is the one the measurement in #1463 was taken in.
//
// With a block store the Runner's streams are the far end of a
// pseudo-terminal, and its pump asks the real terminal per write whether it is
// still adding the carriage returns and adds them itself where it is not — see
// conduitOut, which does that for a background job printing at a prompt. So
// there is nothing for a handler to hand the terminal back *for*, even when it
// prints: the terminal stays raw for the whole call, and the stream stays the
// *os.File that a child started from the handler would inherit a terminal on.
func TestUnderABlockStoreAHandlerKeepsBothTheStreamAndRawMode(t *testing.T) {
	read := armedForever(t)
	dir := t.TempDir()

	var runner *interp.Runner
	p := &descriptorProbe{}
	var mu sync.Mutex
	var printed sync.Once
	streams := map[bool]int{}
	s := newSessionWith(t, func(cfg *Shell) {
		tty, ok := cfg.In.(*os.File)
		if !ok {
			t.Fatal("the session's input is not a terminal")
		}
		// Both of the Runner's streams on one terminal, which is what a
		// session at a prompt has and what captureOutput puts a conduit in
		// front of. The store is a scratch directory of this test's own, and
		// HISTFILE with it: an empty HISTFILE turns the store off, so a
		// session that records nothing would get no conduit either.
		cfg.Runner.Stdout, cfg.Runner.Stderr = tty, tty
		cfg.Runner.Vars["HISTFILE"] = dir + "/history"
		cfg.Runner.Vars[blocks.DirVar] = dir
		runner = cfg.Runner
		cfg.WatchedDescriptors = p.watched
		cfg.DescriptorReady = p.ready
	})
	// Somebody reading the far end, so what the conduit forwards cannot fill
	// the terminal's buffer and block the session.
	fromTerminal := &syncBuffer{}
	go func() { _, _ = io.Copy(fromTerminal, s.control) }()

	p.answer = func(in Line) (Line, bool) {
		// What a child started here would be handed. An *os.File is inherited
		// as a descriptor; anything else makes os/exec build a pipe, and
		// `isatty` is false downstream of it.
		_, isFile := runner.Stdout.(*os.File)
		mu.Lock()
		streams[isFile]++
		mu.Unlock()
		printed.Do(func() { _, _ = fmt.Fprintln(runner.Stdout, "from-the-handler") })
		return in, false
	}

	s.prompt++
	waitFor(t, s.screen, "[1]", "the first prompt")
	p.arm(int(read.Fd()))
	if _, err := s.control.WriteString("x"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the handler", func() bool { return len(p.called()) > 0 })
	waitFor(t, fromTerminal, "from-the-handler", "what the handler printed")

	cooked, samples, calls := spinning(t, s, p, 300*time.Millisecond)
	if calls == 0 {
		t.Fatal("the handler was not being called while the terminal was sampled, " +
			"so nothing was measured")
	}
	if cooked != 0 {
		t.Errorf("the terminal was in its own line discipline for %d of %d samples "+
			"across %d handler calls, where the conduit makes the restore unnecessary",
			cooked, samples, calls)
	}

	p.disarm()
	settled(t, p)
	mu.Lock()
	kept, lost := streams[true], streams[false]
	mu.Unlock()
	if lost != 0 {
		t.Errorf("%d of %d calls found the stream wrapped, so a child started there "+
			"would have been on a pipe rather than on a terminal", lost, kept+lost)
	}
	// And it reached the terminal whole: the conduit's own newlines are bare,
	// and the return in front of this one is the pump reading the mode and
	// putting it back — #1436, at this seam.
	if got := fromTerminal.String(); !strings.Contains(got, "from-the-handler\r\n") {
		t.Errorf("the terminal saw %q, without the carriage return the conduit owes it", got)
	}

	if _, err := s.control.WriteString("\x15echo after-blocks\n"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, fromTerminal, "after-blocks", "a command after the handler")
	s.end()
}

// TestAHandlerThatRedirectsTheShellKeepsItsRedirection. `exec >somewhere` in a
// handler replaces the Runner's stream, and the wrapper this call put there is
// gone by the time the call ends. Putting back what was there before would
// undo a redirection the shell asked for, in a handler nobody was watching —
// so the streams are restored only where they are still this call's wrappers.
func TestAHandlerThatRedirectsTheShellKeepsItsRedirection(t *testing.T) {
	read := armedForever(t)
	elsewhere := &syncBuffer{}

	p := &descriptorProbe{}
	var runner *interp.Runner
	s := newSessionWith(t, func(cfg *Shell) {
		runner = cfg.Runner
		cfg.WatchedDescriptors = p.watched
		cfg.DescriptorReady = p.ready
	})
	var once sync.Once
	p.answer = func(in Line) (Line, bool) {
		once.Do(func() {
			runner.Stdout = elsewhere
			p.disarm()
		})
		return in, false
	}

	s.prompt++
	waitFor(t, s.screen, "[1]", "the first prompt")
	p.arm(int(read.Fd()))
	if _, err := s.control.WriteString("x"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the handler", func() bool { return len(p.called()) > 0 })
	settled(t, p)
	// And the redirection is what the shell writes through afterwards, which
	// is the claim as a person would see it.
	if _, err := s.control.WriteString("\x15echo redirected\n"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, elsewhere, "redirected", "a command after the handler redirected the shell")
	s.end()

	if runner.Stdout != io.Writer(elsewhere) {
		t.Errorf("the Runner's stream is %T, want the one the handler redirected it to", runner.Stdout)
	}
}
