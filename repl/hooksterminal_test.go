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
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/internal/blocks"
	"github.com/blairham/sh/internal/pty"
)

// A hook prints with the terminal in its own line discipline.
//
// Raw mode has OPOST off, so a newline written under it is a line feed and
// nothing else and the next thing drawn starts wherever the last one ended.
// The editor holds the terminal in raw mode for the whole of the time between
// two commands — which is exactly when the prompt hook runs — and a hook is a
// shell function whose output goes to the Runner's streams, which nothing in
// this package translates. So the restore is what stands between a one-line
// `precmd` and a prompt twenty columns in.
//
// Asked of the terminal rather than of the bytes, because the bytes only show
// it once something has printed two lines: this is the state the *next* thing
// to print will find, whatever that turns out to be.
func TestWorkHandedTheTerminalBackFindsItsOwnLineDiscipline(t *testing.T) {
	control, terminal, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	t.Cleanup(func() {
		_ = terminal.Close()
		_ = control.Close()
	})
	state, err := makeRaw(terminal)
	if err != nil {
		t.Fatalf("raw mode: %v", err)
	}

	s := Shell{In: terminal, Err: io.Discard}
	var during bool
	s.inLineDiscipline(state, func() { during = echoes(t, terminal) })

	if !during {
		t.Error("the work ran with the terminal still in raw mode")
	}
	if echoes(t, terminal) {
		t.Error("raw mode was not put back afterwards")
	}
}

// echoes reports whether the terminal is in its own line discipline, which
// raw mode turns off and restoring turns back on.
func echoes(t *testing.T, f *os.File) bool {
	t.Helper()
	var termios syscall.Termios
	if err := ioctl(int(f.Fd()), tcGets, &termios); err != nil {
		t.Fatalf("reading the terminal: %v", err)
	}
	return termios.Lflag&syscall.ECHO != 0
}

// And nothing is still on its way to the terminal when raw mode comes back.
//
// The restore above is only half of what the helper owes. A command's output
// does not always go straight to the terminal: under a block store it goes
// through a pseudo-terminal whose own output discipline is off, and a
// goroutine copies that to the real one. So the command returning and its last
// bytes arriving are two events, and raw mode landing between them sends the
// tail out with the newline translation already gone — a bare line feed, and
// the next prompt drawn wherever the output ended.
//
// **The window is forced and the invariant is asked directly.** The conduit's
// destination is held shut until the test lets go, so the copy provably has
// not happened when the work returns; and rather than looking for bare line
// feeds in what came out, the destination records the terminal's *output
// discipline at the moment of each write*. Every byte the shell hands the
// terminal while it holds it has to be written with the translation on, and
// that is the whole assertion.
//
// Counting bare line feeds was the first version of this and it hoped: with
// a payload large enough to fill the inner terminal the work blocks until the
// copy has caught up, so there is nothing left in flight and the test passes
// for a shell with no wait at all. The payload here is deliberately small
// enough that the work cannot block, and what is measured is the mode and not
// the bytes.
func TestNothingIsStillOnItsWayWhenRawModeComesBack(t *testing.T) {
	control, terminal, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	t.Cleanup(func() {
		_ = terminal.Close()
		_ = control.Close()
	})
	// Somebody reading the far end, so the terminal's own buffer cannot fill
	// and block the writes being measured.
	go func() {
		_, _ = io.Copy(io.Discard, control)
	}()

	state, err := makeRaw(terminal)
	if err != nil {
		t.Fatalf("raw mode: %v", err)
	}
	gate := make(chan struct{})
	held := &modeRecordingWriter{w: terminal, fd: int(terminal.Fd()), gate: gate}
	captured := blocks.NewCapture(1 << 16)
	conduit, err := newPtyConduit(terminal, held, captured.Stream)
	if err != nil {
		t.Skipf("no conduit: %v", err)
	}
	t.Cleanup(conduit.close)

	s := Shell{In: terminal, Err: io.Discard, capture: &outputCapture{cap: captured, conduit: conduit}}
	// Let go well after the work has returned. A shell that does not wait is
	// already back in raw mode by then, and one that waits is still in its own
	// line discipline — which is the difference this measures.
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(gate)
	}()
	s.inLineDiscipline(state, func() {
		// What a command prints, through the stream a command is handed.
		// Small enough that the inner terminal takes all of it without the
		// copy having to keep up, so the work cannot block here — see the
		// comment above for why that matters.
		for i := 0; i < 20; i++ {
			if _, err := fmt.Fprintf(conduit.Stream(), "line-%03d\n", i); err != nil {
				t.Errorf("writing as a command would: %v", err)
				return
			}
		}
	})

	writes, translated, text := held.seen()
	if writes == 0 {
		// Not only a guard against measuring nothing — this *is* the wait.
		// The destination is still shut at this point, so a shell that came
		// back to raw mode without waiting has returned from the work with
		// none of the output delivered, which is the failure itself.
		t.Fatal("the work returned with none of the output delivered, " +
			"so raw mode came back with the output still on its way")
	}
	if translated != writes {
		t.Errorf("%d of %d writes reached the terminal with the newline translation off, "+
			"so raw mode came back before the output had arrived", writes-translated, writes)
	}
	// And it was the output, rather than some byte of the conduit's own.
	if want := "line-019"; !strings.Contains(text, want) {
		t.Errorf("the terminal never saw %q; what arrived was %q", want, text)
	}
	// The capture still holds it, which is the other half of the fix: waiting
	// by emptying the capture would lose every command's body from the store.
	if held, _, _ := captured.Take(); !strings.Contains(held, "line-019") {
		t.Errorf("the capture was emptied by the wait; it holds %q", held)
	}
}

// modeRecordingWriter is the fake terminal the invariant is asked of: it holds
// everything written until the gate is opened, and records for each write
// whether the real terminal's output discipline was on at that moment.
//
// The mode is read from the terminal itself rather than tracked here, so the
// test cannot pass by believing its own bookkeeping.
type modeRecordingWriter struct {
	w    io.Writer
	fd   int
	gate chan struct{}

	mu         sync.Mutex
	writes     int
	translated int
	text       strings.Builder
}

func (m *modeRecordingWriter) Write(p []byte) (int, error) {
	<-m.gate
	var termios syscall.Termios
	on := false
	if err := ioctl(m.fd, tcGets, &termios); err == nil {
		on = termios.Oflag&syscall.OPOST != 0
	}
	m.mu.Lock()
	m.writes++
	if on {
		m.translated++
	}
	m.text.Write(p)
	m.mu.Unlock()
	return m.w.Write(p)
}

// seen is how many writes reached the terminal, how many of those found the
// translation on, and what they carried.
func (m *modeRecordingWriter) seen() (writes, translated int, text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writes, m.translated, m.text.String()
}
